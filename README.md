# ssh-tunnel

`ssh-tunnel` is a cross-platform Go CLI that creates SSH local port forwards without calling the system `ssh` binary. It uses `golang.org/x/crypto/ssh` and can answer keyboard-interactive TOTP prompts from a configured Google Authenticator-compatible seed.

## Install and build

```bash
go build -o ssh-tunnel ./cmd/ssh-tunnel
```

The resulting binary can be copied to Linux, macOS, or Windows hosts built for the matching `GOOS`/`GOARCH`.


## Release builds

Use `make release VERSION=v0.1.0` to create Linux, macOS, and Windows archives with SHA-256 checksums. See [`docs/release.md`](docs/release.md) for local and GitHub release steps.

## Configuration

Default configuration path:

```text
~/.ssh-tunnel/config.yaml
```

Use `--config /path/to/config.yaml` to override it. Keep the file private; on Unix-like systems `0600` is recommended and broader permissions produce a warning.

See [`config.example.yaml`](config.example.yaml) for a complete example. Supported v1 authentication modes are `password_totp` and `key_totp`:

If the same server has multiple addresses, configure them with `hosts`.
Each entry can be either an IP address or a DNS name. When multiple candidates
are present, `ssh-tunnel` probes the SSH TCP port for each address and
automatically connects to the reachable address with the lowest latency, which
is useful for choosing the best China Unicom route:

```yaml
host: "server-cu-a.example.com"
hosts:
  - "server-cu-a.example.com"
  - "server-cu-b.example.com"
  - "203.0.113.10"
```

Local forwards can also balance one local port across multiple ordinary TCP
`host:port` targets. For every new local connection, `ssh-tunnel` dials each
direct target from the local machine and uses the reachable target with the
lowest connection latency. Set `direct: true` for this mode; it does not
connect to SSH and does not require SSH auth fields:

```yaml
servers:
  direct-db:
    direct: true
    forwards:
      - name: "local-db"
        local_host: "127.0.0.1"
        local_port: 3307
        direct_targets:
          - host: "db-a.internal.example.com"
            port: 3306
          - host: "db-b.internal.example.com"
            port: 3306
          - host: "10.0.0.10"
            port: 3306
```

```yaml
auth:
  type: "password_totp"
  password: "your-password"
  totp_seed: "BASE32_TOTP_SECRET"
```

For servers that use public-key authentication before the TOTP prompt, use `key_totp`:

```yaml
auth:
  type: "key_totp"
  key_path: "/home/you/.ssh/id_ed25519"
  # key_passphrase is optional and only needed for encrypted private keys.
  key_passphrase: "optional-private-key-passphrase"
  totp_seed: "BASE32_TOTP_SECRET"
```

Passwords, seeds, and generated TOTP codes are never written to logs.

## Usage

Start a tunnel:

```bash
ssh-tunnel run my-server --config ./config.yaml
```

Check config, local port availability, TOTP seed validity, and SSH TCP reachability:

```bash
ssh-tunnel check my-server --config ./config.yaml
```

Generate the current TOTP code for local debugging:

```bash
ssh-tunnel totp my-server --config ./config.yaml
```

Show version:

```bash
ssh-tunnel version
```

## Troubleshooting

- SSH connection failure: verify `host`, `port`, firewall rules, and that the server is reachable.
- Password failure: verify `username` and `auth.password`.
- Key authentication failure: verify `username`, `auth.key_path`, private key permissions, and `auth.key_passphrase` for encrypted keys.
- If `key_totp` is rejected with an error that only mentions `password_totp`, rebuild or replace the executable; `ssh-tunnel version` should list both `password_totp` and `key_totp` in supported auth modes.
- TOTP failure: verify `auth.totp_seed`, local time, server time, and PAM/Google Authenticator settings.
- Local port in use: change `local_port` or stop the process using it.
- Remote port unreachable: verify `remote_host` and `remote_port` from the SSH server's network namespace.
- Reconnect does not happen: confirm `reconnect.enabled: true`; `max_retries: 0` means retry forever.
- Unsafe config permissions: run `chmod 600 ~/.ssh-tunnel/config.yaml` on Unix-like systems.

## Local container testing

A full PAM TOTP OpenSSH fixture is environment-specific, but the recommended flow is:

1. Start a Linux container with OpenSSH Server and `libpam-google-authenticator` installed.
2. Create a test user and enable password authentication plus keyboard-interactive/PAM authentication in `sshd_config`.
3. Run `google-authenticator` for the test user and copy the Base32 secret into `totp_seed`.
4. Start a simple service in the container, for example `python3 -m http.server 8000`.
5. Map the container SSH port to the host, configure `host`, `port`, and a forward from local `8080` to remote `127.0.0.1:8000`.
6. Run `ssh-tunnel run test --config ./config.yaml`.
7. Verify forwarding with `curl http://127.0.0.1:8080`.
8. Restart the container SSH service and confirm logs show reconnect and the forwarding works again.
