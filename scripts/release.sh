#!/usr/bin/env sh
set -eu

APP_NAME=${APP_NAME:-ssh-tunnel}
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
DIST_DIR=${DIST_DIR:-dist}
RELEASE_SPECS=${RELEASE_SPECS:-"linux/amd64// linux/arm64// darwin/amd64// darwin/arm64// windows/amd64// windows/arm64// linux/amd64/gui/gui linux/arm64/gui/gui darwin/amd64/gui/gui darwin/arm64/gui/gui windows/amd64/gui/gui windows/arm64/gui/gui"}

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

for spec in $RELEASE_SPECS; do
  os=$(printf '%s' "$spec" | cut -d/ -f1)
  arch=$(printf '%s' "$spec" | cut -d/ -f2)
  tags=$(printf '%s' "$spec" | cut -d/ -f3)
  suffix=$(printf '%s' "$spec" | cut -d/ -f4)
  tmpdir=$(mktemp -d)
  bin="$APP_NAME"
  if [ "$os" = "windows" ]; then
    bin="$APP_NAME.exe"
  fi

  build_args=""
  if [ -n "$tags" ]; then
    build_args="-tags $tags"
  fi

  # shellcheck disable=SC2086
  GOTOOLCHAIN=local GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build \
    $build_args \
    -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" \
    -o "$tmpdir/$bin" \
    ./cmd/ssh-tunnel

  cp README.md config.example.yaml "$tmpdir/"
  archive_base="${APP_NAME}_${VERSION}_${os}_${arch}"
  if [ -n "$suffix" ]; then
    archive_base="${archive_base}_${suffix}"
  fi
  if [ "$os" = "windows" ]; then
    (cd "$tmpdir" && zip -q -r "../${archive_base}.zip" .)
    mv "$tmpdir/../${archive_base}.zip" "$DIST_DIR/"
  else
    tar -C "$tmpdir" -czf "$DIST_DIR/${archive_base}.tar.gz" .
  fi
  rm -rf "$tmpdir"
done

(cd "$DIST_DIR" && sha256sum * > SHA256SUMS)
echo "Release artifacts written to $DIST_DIR"
