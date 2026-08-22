//go:build gui

package gui

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"ssh-tunnel/internal/config"
	"ssh-tunnel/internal/forward"
	"ssh-tunnel/internal/runner"
	"ssh-tunnel/internal/sshclient"
	"ssh-tunnel/internal/totp"
)

// Run starts a cross-platform non-web control panel for ssh-tunnel.
func Run(configPath string) error {
	oldLogOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(oldLogOutput)

	if configPath == "" {
		configPath = config.DefaultPath()
	}
	panel := &controlPanel{
		app:         tview.NewApplication(),
		configPath:  configPath,
		statuses:    make(map[string]*serverRuntime),
		diagnostics: make(map[string]serverDiagnostics),
	}
	panel.build()
	panel.loadConfig()
	panel.startTicker()
	if err := panel.app.Run(); err != nil {
		panel.stopAll()
		return err
	}
	panel.stopAll()
	return nil
}

type controlPanel struct {
	app        *tview.Application
	configPath string
	cfg        *config.Config

	servers    *tview.List
	editor     *tview.TextArea
	status     *tview.TextView
	path       *tview.InputField
	selected   string
	configOpen bool

	mu          sync.Mutex
	statuses    map[string]*serverRuntime
	diagnostics map[string]serverDiagnostics
	checkSeq    int
}

type serverDiagnostics struct {
	checking bool
	updated  time.Time
	summary  string
	details  string
}

type serverRuntime struct {
	cancel       context.CancelFunc
	running      bool
	last         string
	stats        *forward.Stats
	lastUpload   uint64
	lastDownload uint64
	lastSample   time.Time
	upSpeed      float64
	downSpeed    float64
}

func (p *controlPanel) build() {
	p.path = tview.NewInputField().SetLabel("Config: ").SetText(p.configPath).SetFieldWidth(0)
	p.editor = tview.NewTextArea().SetWrap(false)
	p.editor.SetTitle(" 配置保险箱 ").SetBorder(true).SetBorderColor(tcell.ColorDarkCyan).SetTitleColor(tcell.ColorLightCyan)
	p.editor.SetText("配置包含密码、TOTP 种子等敏感信息，默认不显示。按 o 打开，编辑后 Ctrl+S 保存；按 h 关闭并清空显示。", false)
	p.status = tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	p.status.SetTitle(" 运行雷达 / 自动体检 ").SetBorder(true).SetBorderColor(tcell.ColorDarkSlateBlue).SetTitleColor(tcell.ColorAqua)
	p.servers = tview.NewList().ShowSecondaryText(false)
	p.servers.SetTitle(" 隧道编队 ").SetBorder(true).SetBorderColor(tcell.ColorDarkCyan).SetTitleColor(tcell.ColorLightCyan)

	p.path.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			p.configPath = strings.TrimSpace(p.path.GetText())
			p.loadConfig()
		}
	})
	p.servers.SetSelectedFunc(func(index int, _ string, _ string, _ rune) {
		names := p.serverNames()
		if index >= 0 && index < len(names) {
			p.selected = names[index]
			p.refreshDashboard()
		}
	})

	hero := tview.NewTextView().SetDynamicColors(true).SetText("[::b][aqua]SSH Tunnel Control[-]  [gray]所有连接状态集中显示，延迟自动刷新[-]")
	help := tview.NewTextView().SetDynamicColors(true).SetText("[aqua]s[-] 启动  [aqua]x[-] 停止  [aqua]c[-] 立即体检  [aqua]o/h[-] 打开/隐藏配置  [aqua]Ctrl+S/R[-] 保存/重载  [aqua]q[-] 退出")
	left := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(p.path, 1, 0, false).AddItem(p.servers, 0, 1, true)
	right := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(hero, 1, 0, false).AddItem(p.status, 0, 4, false).AddItem(p.editor, 0, 2, false).AddItem(help, 1, 0, false)
	root := tview.NewFlex().AddItem(left, 36, 0, true).AddItem(right, 0, 1, false)

	p.app.SetRoot(root, true).EnableMouse(true).SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyCtrlS:
			p.saveConfig()
			return nil
		case event.Key() == tcell.KeyCtrlR:
			p.loadConfig()
			return nil
		case event.Rune() == 'o':
			p.openConfig()
			return nil
		case event.Rune() == 'h':
			p.closeConfig()
			return nil
		case event.Rune() == 'c':
			p.checkAll()
			return nil
		case event.Rune() == 's':
			p.startSelected()
			return nil
		case event.Rune() == 'x':
			p.stopSelected()
			return nil
		case event.Rune() == 'q':
			p.app.Stop()
			return nil
		}
		return event
	})
}

func (p *controlPanel) openConfig() {
	p.configOpen = true
	p.loadConfig()
}

func (p *controlPanel) closeConfig() {
	p.configOpen = false
	p.editor.SetText("配置包含密码、TOTP 种子等敏感信息，已隐藏。按 o 打开。", false)
	p.refreshDashboard()
}

func (p *controlPanel) startTicker() {
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			p.sampleRuntimeStats()
		}
	}()
}

func (p *controlPanel) sampleRuntimeStats() {
	now := time.Now()
	p.mu.Lock()
	for _, rt := range p.statuses {
		if rt == nil || rt.stats == nil {
			continue
		}
		up, down := rt.stats.Snapshot()
		if !rt.lastSample.IsZero() {
			elapsed := now.Sub(rt.lastSample).Seconds()
			if elapsed > 0 {
				rt.upSpeed = float64(up-rt.lastUpload) / elapsed
				rt.downSpeed = float64(down-rt.lastDownload) / elapsed
			}
		}
		rt.lastUpload, rt.lastDownload, rt.lastSample = up, down, now
	}
	p.mu.Unlock()
	p.app.QueueUpdateDraw(func() { p.refreshDashboard() })
}

func (p *controlPanel) loadConfig() {
	path := strings.TrimSpace(p.path.GetText())
	if path == "" {
		path = config.DefaultPath()
		p.path.SetText(path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		p.setStatus(fmt.Sprintf("[red]读取配置失败:[-] %v", err))
		return
	}
	if p.configOpen {
		p.editor.SetText(string(data), false)
	}
	cfg, warnings, err := config.Load(path)
	if err != nil {
		p.cfg = nil
		p.rebuildServerList()
		p.setStatus(fmt.Sprintf("[red]配置无效:[-] %v", err))
		return
	}
	p.configPath = path
	p.cfg = cfg
	p.rebuildServerList()
	if len(warnings) > 0 {
		p.setStatus("[yellow]" + strings.Join(warnings, "\n") + "[-]")
		p.checkAll()
		return
	}
	p.refreshDashboard()
	p.checkAll()
}

func (p *controlPanel) saveConfig() {
	path := strings.TrimSpace(p.path.GetText())
	if path == "" {
		p.setStatus("[red]保存失败:[-] 配置路径为空")
		return
	}
	if !p.configOpen {
		p.setStatus("[yellow]配置内容当前已隐藏；按 o 打开后再保存。[-]")
		return
	}
	if err := os.WriteFile(path, []byte(p.editor.GetText()), 0600); err != nil {
		p.setStatus(fmt.Sprintf("[red]保存失败:[-] %v", err))
		return
	}
	p.loadConfig()
}

func (p *controlPanel) rebuildServerList() {
	p.servers.Clear()
	names := p.serverNames()
	selectedExists := false
	for _, name := range names {
		if name == p.selected {
			selectedExists = true
		}
		p.servers.AddItem(p.listLabel(name), "", 0, nil)
	}
	if !selectedExists {
		p.selected = ""
		if len(names) > 0 {
			p.selected = names[0]
		}
	}
}

func (p *controlPanel) serverNames() []string {
	if p.cfg == nil {
		return nil
	}
	names := make([]string, 0, len(p.cfg.Servers))
	for name := range p.cfg.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (p *controlPanel) listLabel(name string) string {
	p.mu.Lock()
	rt := p.statuses[name]
	diag := p.diagnostics[name]
	p.mu.Unlock()
	if rt != nil && rt.running {
		return "[green]◆[-] " + name
	}
	if diag.checking {
		return "[yellow]◌[-] " + name
	}
	if strings.HasPrefix(diag.summary, "异常") {
		return "[red]●[-] " + name
	}
	if strings.HasPrefix(diag.summary, "健康") {
		return "[aqua]●[-] " + name
	}
	return "[gray]○[-] " + name
}

func (p *controlPanel) selectedServer() (config.Server, bool) {
	if p.cfg == nil || p.selected == "" {
		return config.Server{}, false
	}
	s, err := p.cfg.Server(p.selected)
	return s, err == nil
}

func (p *controlPanel) checkAll() {
	if p.cfg == nil {
		return
	}
	p.mu.Lock()
	p.checkSeq++
	seq := p.checkSeq
	p.diagnostics = make(map[string]serverDiagnostics, len(p.cfg.Servers))
	servers := make(map[string]config.Server, len(p.cfg.Servers))
	for name, srv := range p.cfg.Servers {
		servers[name] = srv
		p.diagnostics[name] = serverDiagnostics{checking: true, summary: "检查中"}
	}
	p.mu.Unlock()
	p.rebuildServerList()
	p.refreshDashboard()
	for name, srv := range servers {
		go p.checkServer(seq, name, srv)
	}
}

func (p *controlPanel) checkServer(seq int, name string, s config.Server) {
	summary, details := p.buildDiagnostics(name, s)
	p.mu.Lock()
	if seq != p.checkSeq {
		p.mu.Unlock()
		return
	}
	p.diagnostics[name] = serverDiagnostics{
		updated: time.Now(),
		summary: summary,
		details: details,
	}
	p.mu.Unlock()
	p.app.QueueUpdateDraw(func() {
		p.rebuildServerList()
		p.refreshDashboard()
	})
}

func (p *controlPanel) buildDiagnostics(name string, s config.Server) (string, string) {
	var b strings.Builder
	healthy := true
	fmt.Fprintf(&b, "[::b]%s[::-]\n", tview.Escape(name))
	if err := s.Validate(); err != nil {
		fmt.Fprintf(&b, "[red]配置校验失败:[-] %s\n", tview.Escape(err.Error()))
		return "异常：配置无效", b.String()
	}
	for _, f := range s.Forwards {
		if !p.checkForward(&b, name, s, f) {
			healthy = false
		}
	}
	if s.Direct {
		if healthy {
			fmt.Fprintf(&b, "[green]直连转发就绪[-]\n")
		}
		return healthSummary(healthy), b.String()
	}
	if !p.checkSSH(&b, name, s) {
		healthy = false
	}
	return healthSummary(healthy), b.String()
}

func (p *controlPanel) checkForward(b *strings.Builder, name string, s config.Server, f config.Forward) bool {
	healthy := true
	addr := net.JoinHostPort(f.LocalHost, fmt.Sprint(f.LocalPort))
	if p.isRunning(name) {
		fmt.Fprintf(b, "- [green]本地监听 %s 已由当前连接接管[-]\n", tview.Escape(addr))
	} else {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			fmt.Fprintf(b, "- [red]本地监听 %s 不可用:[-] %s\n", tview.Escape(addr), tview.Escape(err.Error()))
			healthy = false
		} else {
			_ = ln.Close()
			fmt.Fprintf(b, "- [green]本地监听 %s 可用[-]\n", tview.Escape(addr))
		}
	}
	if s.Direct {
		for _, t := range f.DirectCandidates() {
			if !probeTCP(b, "备选直连", net.JoinHostPort(t.Host, fmt.Sprint(t.Port))) {
				healthy = false
			}
		}
	}
	return healthy
}

func (p *controlPanel) isRunning(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	rt := p.statuses[name]
	return rt != nil && rt.running
}

func (p *controlPanel) checkSSH(b *strings.Builder, name string, s config.Server) bool {
	healthy := true
	if _, err := totp.Generate(s.Auth.TOTPSeed); err != nil {
		fmt.Fprintf(b, "\n[red]TOTP 种子无效:[-] %s\n", tview.Escape(err.Error()))
		healthy = false
	} else {
		fmt.Fprintf(b, "\n[green]TOTP 种子格式有效[-]\n")
	}
	selected, probes, err := sshclient.SelectBestEndpoint(s)
	for _, probe := range probes {
		if probe.Err != nil {
			fmt.Fprintf(b, "- [red]SSH 备选 %s 不可达[-]（%s）：%s\n", tview.Escape(probe.Address), probe.Latency.Round(time.Millisecond), tview.Escape(probe.Err.Error()))
		} else {
			fmt.Fprintf(b, "- [green]SSH 备选 %s 可达[-]（%s）\n", tview.Escape(probe.Address), probe.Latency.Round(time.Millisecond))
		}
	}
	if err != nil {
		fmt.Fprintf(b, "\n[red]无可用 SSH 备选:[-] %s\n", tview.Escape(err.Error()))
		healthy = false
	} else {
		fmt.Fprintf(b, "\n[green]%s 当前最佳 SSH 主机:[-] %s\n", tview.Escape(name), tview.Escape(selected))
	}
	return healthy
}

func probeTCP(b *strings.Builder, label, addr string) bool {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	latency := time.Since(start)
	if err != nil {
		fmt.Fprintf(b, "- [red]%s %s 不可达[-]（%s）：%s\n", tview.Escape(label), tview.Escape(addr), latency.Round(time.Millisecond), tview.Escape(err.Error()))
		return false
	}
	_ = conn.Close()
	fmt.Fprintf(b, "- [green]%s %s 可达[-]（%s）\n", tview.Escape(label), tview.Escape(addr), latency.Round(time.Millisecond))
	return true
}

func healthSummary(healthy bool) string {
	if healthy {
		return "健康"
	}
	return "异常：部分检查失败"
}

func (p *controlPanel) startSelected() {
	s, ok := p.selectedServer()
	if !ok {
		p.setStatus("[yellow]请先选择一个连接。[-]")
		return
	}
	name := p.selected
	p.mu.Lock()
	if rt := p.statuses[name]; rt != nil && rt.running {
		p.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.statuses[name] = &serverRuntime{cancel: cancel, running: true, last: "启动中", stats: &forward.Stats{}, lastSample: time.Now()}
	p.mu.Unlock()
	p.rebuildServerList()
	p.refreshDashboard()
	go func() {
		p.mu.Lock()
		stats := p.statuses[name].stats
		p.mu.Unlock()
		err := runner.RunWithStats(ctx, name, s, stats)
		p.mu.Lock()
		rt := p.statuses[name]
		if rt != nil {
			rt.running = false
			rt.last = fmt.Sprint(err)
		}
		p.mu.Unlock()
		p.app.QueueUpdateDraw(func() {
			p.rebuildServerList()
			p.refreshDashboard()
		})
	}()
}

func (p *controlPanel) stopSelected() {
	if p.selected != "" {
		p.stop(p.selected)
		p.rebuildServerList()
		p.refreshDashboard()
	}
}

func (p *controlPanel) stopAll() {
	for _, name := range p.serverNames() {
		p.stop(name)
	}
}

func (p *controlPanel) stop(name string) {
	p.mu.Lock()
	rt := p.statuses[name]
	if rt != nil && rt.cancel != nil {
		rt.cancel()
		rt.running = false
		rt.last = "已停止"
	}
	p.mu.Unlock()
}

func (p *controlPanel) refreshDashboard() {
	names := p.serverNames()
	if len(names) == 0 {
		p.setStatus("[yellow]未载入连接配置。[-]")
		return
	}
	if p.selected == "" {
		p.selected = names[0]
	}

	var running, checking, failed int
	type row struct {
		name       string
		state      string
		summary    string
		updated    time.Time
		up, down   uint64
		upSpeed    float64
		downSpeed  float64
		forwardNum int
	}
	rows := make([]row, 0, len(names))
	p.mu.Lock()
	for _, name := range names {
		state := "未运行"
		var up, down uint64
		var upSpeed, downSpeed float64
		if rt := p.statuses[name]; rt != nil {
			if rt.running {
				state = "运行中"
				running++
			} else if rt.last != "" {
				state = rt.last
			}
			if rt.stats != nil {
				up, down = rt.stats.Snapshot()
			}
			upSpeed, downSpeed = rt.upSpeed, rt.downSpeed
		}
		diag := p.diagnostics[name]
		if diag.checking {
			checking++
		}
		if strings.HasPrefix(diag.summary, "异常") {
			failed++
		}
		forwardNum := 0
		if p.cfg != nil {
			forwardNum = len(p.cfg.Servers[name].Forwards)
		}
		rows = append(rows, row{name: name, state: state, summary: diag.summary, updated: diag.updated, up: up, down: down, upSpeed: upSpeed, downSpeed: downSpeed, forwardNum: forwardNum})
	}
	selectedDiag := p.diagnostics[p.selected]
	p.mu.Unlock()

	var b strings.Builder
	fmt.Fprintf(&b, "[::b][aqua]舰桥总览[::-]  连接 %d  运行 %d  检查中 %d  异常 %d\n", len(names), running, checking, failed)
	fmt.Fprintf(&b, "[gray]配置：%s[-]\n\n", tview.Escape(p.configPath))
	b.WriteString("[::b]集中状态[::-]\n")
	for _, r := range rows {
		mark := "[gray]○[-]"
		if r.state == "运行中" {
			mark = "[green]◆[-]"
		} else if strings.HasPrefix(r.summary, "异常") {
			mark = "[red]●[-]"
		} else if r.summary == "健康" {
			mark = "[aqua]●[-]"
		} else if r.summary == "检查中" {
			mark = "[yellow]◌[-]"
		}
		summary := r.summary
		if summary == "" {
			summary = "等待自动体检"
		}
		updated := "未检查"
		if !r.updated.IsZero() {
			updated = r.updated.Format("15:04:05")
		}
		fmt.Fprintf(&b, "%s %-18s %-10s %-18s 转发:%-2d ↑%s/s ↓%s/s 合计:%s  %s\n",
			mark,
			tview.Escape(r.name),
			tview.Escape(r.state),
			tview.Escape(summary),
			r.forwardNum,
			formatBytes(uint64(r.upSpeed)),
			formatBytes(uint64(r.downSpeed)),
			formatBytes(r.up+r.down),
			updated,
		)
	}

	if s, ok := p.selectedServer(); ok {
		fmt.Fprintf(&b, "\n[::b][aqua]当前聚焦：%s[::-]\n", tview.Escape(p.selected))
		b.WriteString("转发拓扑：\n")
		for _, f := range s.Forwards {
			local := net.JoinHostPort(f.LocalHost, fmt.Sprint(f.LocalPort))
			fmt.Fprintf(&b, "- %s  [gray]%s[-] -> %s\n", tview.Escape(f.Name), tview.Escape(local), tview.Escape(forwardDestinationLabel(f)))
		}
		if selectedDiag.checking {
			b.WriteString("\n[yellow]正在自动体检当前连接...[-]\n")
		} else if selectedDiag.details != "" {
			b.WriteString("\n")
			b.WriteString(selectedDiag.details)
		}
	}
	b.WriteString("\n[gray]提示：体检会在载入/重载后自动执行；按 c 可立即刷新全部延迟。[-]")
	p.setStatus(b.String())
}

func forwardDestinationLabel(f config.Forward) string {
	if len(f.DirectTargets) > 0 {
		parts := make([]string, 0, len(f.DirectCandidates()))
		for _, t := range f.DirectCandidates() {
			parts = append(parts, net.JoinHostPort(t.Host, fmt.Sprint(t.Port)))
		}
		return strings.Join(parts, ", ")
	}
	return net.JoinHostPort(f.RemoteHost, fmt.Sprint(f.RemotePort))
}

func formatBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func (p *controlPanel) setStatus(text string) {
	p.status.Clear()
	p.status.SetText(text)
	p.status.ScrollToBeginning()
}

func (p *controlPanel) queueStatus(text string) {
	p.app.QueueUpdateDraw(func() { p.setStatus(text) })
}
