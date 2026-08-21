# AGENTS.md

本文件为 Codex/自动化编码代理在本仓库工作时的项目说明。请优先遵循系统与开发者指令；本文件用于补充仓库本地约定。

## 项目概览

- 项目名：`ssh-tunnel`
- 语言：Go
- 入口：`cmd/ssh-tunnel/main.go`
- 主要功能：不依赖系统 `ssh` 命令，使用 `golang.org/x/crypto/ssh` 创建本地端口转发；支持密码+TOTP、公钥+TOTP、直连 TCP 转发、多候选地址探测与重连。
- GUI：通过 `gui` build tag 启用，基于 `tview/tcell` 的终端 GUI，代码位于 `internal/gui/`。

## 常用命令

- 普通构建：`make build`
- GUI 构建：`make build-gui`
- 测试：`go test ./...`
- GUI tag 测试：`go test -tags gui ./...`
- Vet：`make vet`
- 发布构建：`make release VERSION=v0.1.0`

## 目录说明

- `cmd/ssh-tunnel/`：CLI 命令解析与顶层命令实现。
- `internal/config/`：YAML 配置结构、默认值、校验、权限警告。
- `internal/forward/`：本地监听、TCP/SSH 转发、流量统计、直连目标选择。
- `internal/sshclient/`：SSH 连接、认证方式、候选 SSH endpoint 探测、keepalive。
- `internal/runner/`：运行与重连循环，串联 SSH client 和 forward manager。
- `internal/totp/`：TOTP 生成封装。
- `internal/secrets/`：缺失敏感信息时的终端安全输入。
- `internal/gui/`：GUI 实现；非 GUI 构建使用 `gui_stub.go`。
- `scripts/`：单目标与发布构建脚本。
- `test/sshd-totp/`：本地 OpenSSH + PAM TOTP 容器测试 fixture。

## 开发约定

- 修改 Go 代码后运行 `gofmt`。
- 优先保持默认构建不引入 GUI 依赖；GUI 相关代码必须受 `//go:build gui` 或 `//go:build !gui` 控制。
- 涉及 GUI 的行为变更，至少运行 `go test -tags gui ./...`。
- 涉及普通 CLI、配置、转发、SSH、TOTP 的变更，至少运行 `go test ./...`。
- 不要在日志中输出密码、TOTP seed、生成的 TOTP code、私钥 passphrase 等敏感信息。
- 配置文件示例和文档中只能使用测试 secret 或占位值。
- 避免破坏已有 CLI 参数兼容性；`--config`、`-config`、`--server`、`-server` 已支持多种位置和 `=` 写法。

## 关键行为

- 配置默认路径为 `~/.ssh-tunnel/config.yaml`。
- 支持认证类型由 `config.SupportedAuthTypes` 定义，目前为 `password_totp` 和 `key_totp`。
- `direct: true` 表示普通 TCP 直连转发，不需要 SSH host、username 或 auth 字段。
- SSH 模式下可配置 `host` 与 `hosts`，运行时会选择 TCP 可达且延迟最低的候选主机。
- direct target 模式下每个新连接会探测多个 `direct_targets`，选择可达且延迟最低的目标。
- `reconnect.max_retries: 0` 表示无限重试。

## 测试注意事项

- 单元测试不应依赖真实外部 SSH 服务；已有测试使用本地 listener 或模拟 SSH server。
- 容器集成测试参考 `docs/testing.md`，需要 Docker 环境手动运行。
- 端口相关测试应使用 `127.0.0.1:0` 动态分配，避免固定端口冲突。

## 当前上下文提示

- 如果继续处理 GUI 状态区域问题，重点查看 `internal/gui/gui.go` 中 `setStatus`、`refreshStatus`、`queueStatus`，以及后台日志是否直接写到终端导致界面残留。
