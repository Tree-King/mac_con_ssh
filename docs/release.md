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

### Tag-based release

Tag a version and push it:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The release workflow runs tests, runs `go vet`, builds all release artifacts, uploads them as workflow artifacts, and uploads them to the GitHub Release for the tag.

### Manual Actions run

From the GitHub web UI, open **Actions → release → Run workflow**. A manual run always uploads downloadable files under the workflow run's **Artifacts** section.

Optional inputs:

- `version`: version text embedded in binary names. If omitted or left as `manual`, the workflow uses `manual-<run-number>-<short-sha>`.
- `create_github_release`: when checked, the workflow also creates or updates a GitHub Release using the resolved version as the tag name. Leave it unchecked if you only want workflow artifacts.
