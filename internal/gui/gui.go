//go:build gui

package gui

import (
	"context"
	"fmt"
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
	if configPath == "" {
		configPath = config.DefaultPath()
	}
	panel := &controlPanel{
		app:        tview.NewApplication(),
		configPath: configPath,
		statuses:   make(map[string]*serverRuntime),
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

	mu       sync.Mutex
	statuses map[string]*serverRuntime
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
	p.editor.SetTitle(" YAML 配置（默认隐藏，按 o 打开 / h 关闭） ").SetBorder(true)
	p.editor.SetText("配置包含密码、TOTP 种子等敏感信息，默认不显示。按 o 打开，编辑后 Ctrl+S 保存；按 h 关闭并清空显示。", false)
	p.status = tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	p.status.SetTitle(" 状态 / 备选 ").SetBorder(true)
	p.servers = tview.NewList().ShowSecondaryText(false)
	p.servers.SetTitle(" 连接 ").SetBorder(true)

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
			p.refreshStatus()
		}
	})

	help := tview.NewTextView().SetDynamicColors(true).SetText("[green]o[-] 打开配置  [green]h[-] 关闭配置  [green]Ctrl+S[-] 保存  [green]Ctrl+R[-] 重载  [green]c[-] 检查  [green]s/x[-] 启停  [green]q[-] 退出")
	left := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(p.path, 1, 0, false).AddItem(p.servers, 0, 1, true)
	right := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(p.editor, 0, 3, false).AddItem(p.status, 0, 2, false).AddItem(help, 1, 0, false)
	root := tview.NewFlex().AddItem(left, 34, 0, true).AddItem(right, 0, 1, false)

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
			p.checkSelected()
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
	p.setStatus("[green]配置内容已关闭并从界面清空。[-]")
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
	p.app.QueueUpdateDraw(func() { p.refreshStatus() })
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
		return
	}
	p.setStatus("[green]配置已载入:[-] " + path)
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
	for _, name := range p.serverNames() {
		p.servers.AddItem(p.listLabel(name), "", 0, nil)
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
	p.mu.Unlock()
	if rt != nil && rt.running {
		return "● " + name
	}
	return "○ " + name
}

func (p *controlPanel) selectedServer() (config.Server, bool) {
	if p.cfg == nil || p.selected == "" {
		return config.Server{}, false
	}
	s, err := p.cfg.Server(p.selected)
	return s, err == nil
}

func (p *controlPanel) checkSelected() {
	s, ok := p.selectedServer()
	if !ok {
		p.setStatus("[yellow]请先选择一个连接。[-]")
		return
	}
	name := p.selected
	p.setStatus("[yellow]正在检查 " + name + "...[-]")
	go func() {
		var b strings.Builder
		fmt.Fprintf(&b, "[::b]%s[::-]\n\n", tview.Escape(name))
		if err := s.Validate(); err != nil {
			fmt.Fprintf(&b, "[red]配置校验失败:[-] %s\n", tview.Escape(err.Error()))
			p.queueStatus(b.String())
			return
		}
		for _, f := range s.Forwards {
			p.checkForward(&b, s, f)
		}
		if s.Direct {
			fmt.Fprintf(&b, "\n[green]直连 TCP 转发配置有效。[-]\n")
		} else {
			p.checkSSH(&b, name, s)
		}
		p.queueStatus(b.String())
	}()
}

func (p *controlPanel) checkForward(b *strings.Builder, s config.Server, f config.Forward) {
	addr := net.JoinHostPort(f.LocalHost, fmt.Sprint(f.LocalPort))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(b, "- [red]本地监听 %s 不可用:[-] %s\n", tview.Escape(addr), tview.Escape(err.Error()))
	} else {
		_ = ln.Close()
		fmt.Fprintf(b, "- [green]本地监听 %s 可用[-]\n", tview.Escape(addr))
	}
	if s.Direct {
		for _, t := range f.DirectCandidates() {
			probeTCP(b, "备选直连", net.JoinHostPort(t.Host, fmt.Sprint(t.Port)))
		}
	}
}

func (p *controlPanel) checkSSH(b *strings.Builder, name string, s config.Server) {
	if _, err := totp.Generate(s.Auth.TOTPSeed); err != nil {
		fmt.Fprintf(b, "\n[red]TOTP 种子无效:[-] %s\n", tview.Escape(err.Error()))
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
	} else {
		fmt.Fprintf(b, "\n[green]%s 当前最佳 SSH 主机:[-] %s\n", tview.Escape(name), tview.Escape(selected))
	}
}

func probeTCP(b *strings.Builder, label, addr string) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	latency := time.Since(start)
	if err != nil {
		fmt.Fprintf(b, "- [red]%s %s 不可达[-]（%s）：%s\n", tview.Escape(label), tview.Escape(addr), latency.Round(time.Millisecond), tview.Escape(err.Error()))
		return
	}
	_ = conn.Close()
	fmt.Fprintf(b, "- [green]%s %s 可达[-]（%s）\n", tview.Escape(label), tview.Escape(addr), latency.Round(time.Millisecond))
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
	p.refreshStatus()
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
			p.refreshStatus()
		})
	}()
}

func (p *controlPanel) stopSelected() {
	if p.selected != "" {
		p.stop(p.selected)
		p.rebuildServerList()
		p.refreshStatus()
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

func (p *controlPanel) refreshStatus() {
	if p.selected == "" {
		return
	}
	state := "未运行"
	var up, down uint64
	var upSpeed, downSpeed float64
	p.mu.Lock()
	if rt := p.statuses[p.selected]; rt != nil {
		if rt.running {
			state = "运行中"
		} else if rt.last != "" {
			state = rt.last
		}
		if rt.stats != nil {
			up, down = rt.stats.Snapshot()
		}
		upSpeed, downSpeed = rt.upSpeed, rt.downSpeed
	}
	p.mu.Unlock()
	var b strings.Builder
	fmt.Fprintf(&b, "[::b]%s[::-]\n\n状态：%s\n", tview.Escape(p.selected), tview.Escape(state))
	fmt.Fprintf(&b, "实时速度：↑ %s/s  ↓ %s/s\n", formatBytes(uint64(upSpeed)), formatBytes(uint64(downSpeed)))
	fmt.Fprintf(&b, "累计流量：↑ %s  ↓ %s  合计 %s\n\n", formatBytes(up), formatBytes(down), formatBytes(up+down))
	if s, ok := p.selectedServer(); ok {
		b.WriteString("转发列表：\n")
		for _, f := range s.Forwards {
			local := net.JoinHostPort(f.LocalHost, fmt.Sprint(f.LocalPort))
			fmt.Fprintf(&b, "- %s  %s -> %s\n", tview.Escape(f.Name), tview.Escape(local), tview.Escape(forwardDestinationLabel(f)))
			for _, target := range f.DirectCandidates() {
				fmt.Fprintf(&b, "  · 备选 %s  延迟：按 c 检查\n", tview.Escape(net.JoinHostPort(target.Host, fmt.Sprint(target.Port))))
			}
		}
		if !s.Direct {
			b.WriteString("\nSSH 备选：\n")
			for _, host := range s.HostCandidates() {
				fmt.Fprintf(&b, "- %s  延迟：按 c 检查\n", tview.Escape(net.JoinHostPort(host, fmt.Sprint(s.Port))))
			}
		}
	}
	b.WriteString("\n按 c 检查所有备选地址延迟；按 o 打开配置，按 h 关闭配置。")
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
	p.status.SetText(text)
}

func (p *controlPanel) queueStatus(text string) {
	p.app.QueueUpdateDraw(func() { p.setStatus(text) })
}
