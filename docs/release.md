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

The release workflow runs on every push to `main`, including merged pull requests. It runs tests, runs `go vet`, creates a unique release tag in the form `main-<run-number>-<short-sha>`, builds all release artifacts with that exact tag embedded in `ssh-tunnel version`, and uploads the artifacts to a new GitHub Release marked as the latest release. The workflow does not delete or reuse older releases or tags.

GitHub release artifacts should therefore be named like `ssh-tunnel_main-123-51f7c44_linux_amd64.tar.gz`, not `ssh-tunnel_latest_linux_amd64.tar.gz`. If a downloaded binary still reports `latest`, it came from an older workflow run and should be replaced with an artifact whose archive name matches the release tag and commit shown in the release body.

You can also start the same workflow manually from the GitHub Actions `workflow_dispatch` button.
