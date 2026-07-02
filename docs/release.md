# Build and release

## Local development build

Build the current platform binary with the Git version embedded:

```bash
make build
```

The command writes `./ssh-tunnel` or `./ssh-tunnel.exe` depending on the platform.

## Single target build

Use `scripts/build.sh` for one explicit target:

```bash
GOOS_TARGET=linux GOARCH_TARGET=amd64 VERSION=v0.1.0 ./scripts/build.sh
```

The output is written to `dist/` by default.

## Cross-platform release artifacts

Create Linux, macOS, and Windows archives plus checksums:

```bash
make release VERSION=v0.1.0
```

Artifacts are written to `dist/`:

- `ssh-tunnel_<version>_linux_amd64.tar.gz`
- `ssh-tunnel_<version>_linux_arm64.tar.gz`
- `ssh-tunnel_<version>_darwin_amd64.tar.gz`
- `ssh-tunnel_<version>_darwin_arm64.tar.gz`
- `ssh-tunnel_<version>_windows_amd64.zip`
- `ssh-tunnel_<version>_windows_arm64.zip`
- `SHA256SUMS`

Each archive includes the binary, `README.md`, and `config.example.yaml`.

## GitHub release

The release workflow runs on every push to `main`, including merged pull requests. It runs tests, runs `go vet`, builds all release artifacts, creates a unique release tag for that workflow run, and uploads the artifacts to a new GitHub Release marked as the latest release. The workflow does not delete or reuse older releases or tags.

You can also start the same workflow manually from the GitHub Actions `workflow_dispatch` button.
