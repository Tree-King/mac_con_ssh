//go:build gui

package gui

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"ssh-tunnel/internal/config"
	"ssh-tunnel/internal/runner"
	"ssh-tunnel/internal/sshclient"
	"ssh-tunnel/internal/totp"
)

// Run starts a cross-platform native desktop control panel for ssh-tunnel.
func Run(configPath string) error {
	if configPath == "" {
		configPath = config.DefaultPath()
	}
	gui := &controlPanel{configPath: configPath, statuses: make(map[string]*serverRuntime)}
	gui.app = app.NewWithID("com.ssh-tunnel.control-panel")
	gui.window = gui.app.NewWindow("ssh-tunnel 配置与状态")
	gui.window.Resize(fyne.NewSize(1120, 760))
	gui.build()
	gui.loadConfig()
	gui.window.ShowAndRun()
	gui.stopAll()
	return nil
}

type controlPanel struct {
	app        fyne.App
	window     fyne.Window
	configPath string
	cfg        *config.Config

	serverList *widget.List
	editor     *widget.Entry
	status     *widget.RichText
	path       *widget.Entry
	selected   string

	mu       sync.Mutex
	statuses map[string]*serverRuntime
}

type serverRuntime struct {
	cancel  context.CancelFunc
	running bool
	last    string
}

func (p *controlPanel) build() {
	p.path = widget.NewEntry()
	p.path.SetText(p.configPath)
	p.path.OnSubmitted = func(path string) { p.configPath = strings.TrimSpace(path); p.loadConfig() }
	p.editor = widget.NewMultiLineEntry()
	p.editor.Wrapping = fyne.TextWrapOff
	p.status = widget.NewRichTextFromMarkdown("选择服务器后可查看状态。")
	p.status.Wrapping = fyne.TextWrapWord

	p.serverList = widget.NewList(
		func() int { return len(p.serverNames()) },
		func() fyne.CanvasObject { return widget.NewLabel("server") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			names := p.serverNames()
			if id >= 0 && id < len(names) {
				obj.(*widget.Label).SetText(p.listLabel(names[id]))
			}
		},
	)
	p.serverList.OnSelected = func(id widget.ListItemID) {
		names := p.serverNames()
		if id >= 0 && id < len(names) {
			p.selected = names[id]
			p.refreshStatus()
		}
	}

	toolbar := container.NewHBox(
		widget.NewButton("重新载入", p.loadConfig),
		widget.NewButton("保存", p.saveConfig),
		widget.NewButton("检查所选", p.checkSelected),
		widget.NewButton("启动所选", p.startSelected),
		widget.NewButton("停止所选", p.stopSelected),
	)
	left := container.NewBorder(widget.NewLabel("连接"), nil, nil, nil, p.serverList)
	rightTop := container.NewBorder(container.NewVBox(widget.NewLabel("配置文件"), p.path, toolbar), nil, nil, nil, p.editor)
	right := container.NewVSplit(rightTop, container.NewBorder(widget.NewLabel("连接状态 / 备选状态"), nil, nil, nil, p.status))
	right.SetOffset(0.62)
	split := container.NewHSplit(left, right)
	split.SetOffset(0.24)
	p.window.SetContent(split)
}

func (p *controlPanel) loadConfig() {
	path := strings.TrimSpace(p.path.Text)
	if path == "" {
		path = config.DefaultPath()
		p.path.SetText(path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		p.showError("读取配置失败", err)
		return
	}
	p.editor.SetText(string(data))
	cfg, warnings, err := config.Load(path)
	if err != nil {
		p.cfg = nil
		p.status.ParseMarkdown("❌ 配置无效：" + err.Error())
		p.serverList.Refresh()
		return
	}
	p.configPath = path
	p.cfg = cfg
	p.serverList.Refresh()
	if len(warnings) > 0 {
		p.status.ParseMarkdown("⚠️ " + strings.Join(warnings, "\n\n⚠️ "))
	} else {
		p.status.ParseMarkdown("✅ 配置已载入：" + path)
	}
}

func (p *controlPanel) saveConfig() {
	path := strings.TrimSpace(p.path.Text)
	if path == "" {
		p.showError("保存失败", fmt.Errorf("配置路径为空"))
		return
	}
	if err := os.WriteFile(path, []byte(p.editor.Text), 0600); err != nil {
		p.showError("保存失败", err)
		return
	}
	p.loadConfig()
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
		return
	}
	name := p.selected
	go func() {
		var b strings.Builder
		fmt.Fprintf(&b, "## %s\n\n", name)
		if err := s.Validate(); err != nil {
			fmt.Fprintf(&b, "❌ 配置校验失败：%v\n", err)
			p.setStatus(b.String())
			return
		}
		for _, f := range s.Forwards {
			p.checkForward(&b, s, f)
		}
		if s.Direct {
			fmt.Fprintf(&b, "\n✅ 直连 TCP 转发配置有效。\n")
		} else {
			p.checkSSH(&b, name, s)
		}
		p.setStatus(b.String())
	}()
}

func (p *controlPanel) checkForward(b *strings.Builder, s config.Server, f config.Forward) {
	addr := net.JoinHostPort(f.LocalHost, fmt.Sprint(f.LocalPort))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(b, "- ❌ 本地监听 %s：%v\n", addr, err)
	} else {
		_ = ln.Close()
		fmt.Fprintf(b, "- ✅ 本地监听 %s 可用\n", addr)
	}
	if s.Direct {
		for _, t := range f.DirectCandidates() {
			probeTCP(b, "备选直连", net.JoinHostPort(t.Host, fmt.Sprint(t.Port)))
		}
	}
}

func (p *controlPanel) checkSSH(b *strings.Builder, name string, s config.Server) {
	if _, err := totp.Generate(s.Auth.TOTPSeed); err != nil {
		fmt.Fprintf(b, "\n❌ TOTP 种子无效：%v\n", err)
	} else {
		fmt.Fprintf(b, "\n✅ TOTP 种子格式有效\n")
	}
	selected, probes, err := sshclient.SelectBestEndpoint(s)
	for _, probe := range probes {
		if probe.Err != nil {
			fmt.Fprintf(b, "- ❌ SSH 备选 %s 不可达（%s）：%v\n", probe.Address, probe.Latency.Round(time.Millisecond), probe.Err)
		} else {
			fmt.Fprintf(b, "- ✅ SSH 备选 %s 可达（%s）\n", probe.Address, probe.Latency.Round(time.Millisecond))
		}
	}
	if err != nil {
		fmt.Fprintf(b, "\n❌ 无可用 SSH 备选：%v\n", err)
	} else {
		fmt.Fprintf(b, "\n✅ %s 当前最佳 SSH 主机：%s\n", name, selected)
	}
}

func probeTCP(b *strings.Builder, label, addr string) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	latency := time.Since(start)
	if err != nil {
		fmt.Fprintf(b, "- ❌ %s %s 不可达（%s）：%v\n", label, addr, latency.Round(time.Millisecond), err)
		return
	}
	_ = conn.Close()
	fmt.Fprintf(b, "- ✅ %s %s 可达（%s）\n", label, addr, latency.Round(time.Millisecond))
}

func (p *controlPanel) startSelected() {
	s, ok := p.selectedServer()
	if !ok {
		return
	}
	name := p.selected
	p.mu.Lock()
	if rt := p.statuses[name]; rt != nil && rt.running {
		p.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.statuses[name] = &serverRuntime{cancel: cancel, running: true, last: "启动中"}
	p.mu.Unlock()
	p.serverList.Refresh()
	p.refreshStatus()
	go func() {
		err := runner.Run(ctx, name, s)
		p.mu.Lock()
		rt := p.statuses[name]
		if rt != nil {
			rt.running = false
			rt.last = fmt.Sprint(err)
		}
		p.mu.Unlock()
		p.serverList.Refresh()
		p.refreshStatus()
	}()
}

func (p *controlPanel) stopSelected() {
	if p.selected != "" {
		p.stop(p.selected)
		p.serverList.Refresh()
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
	p.mu.Lock()
	rt := p.statuses[p.selected]
	p.mu.Unlock()
	state := "未运行"
	if rt != nil {
		if rt.running {
			state = "运行中"
		} else if rt.last != "" {
			state = rt.last
		}
	}
	p.status.ParseMarkdown(fmt.Sprintf("## %s\n\n状态：%s\n\n点击“检查所选”刷新备选地址和端口状态。", p.selected, state))
}
func (p *controlPanel) setStatus(markdown string) { p.status.ParseMarkdown(markdown) }
func (p *controlPanel) showError(title string, err error) {
	log.Print(err)
	dialog.ShowError(fmt.Errorf("%s: %w", title, err), p.window)
}
