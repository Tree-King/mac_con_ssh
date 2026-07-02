#!/usr/bin/env sh
set -eu

APP_NAME=${APP_NAME:-ssh-tunnel}
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
DIST_DIR=${DIST_DIR:-dist}
TARGETS=${TARGETS:-"linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"}

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

for target in $TARGETS; do
  os=${target%/*}
  arch=${target#*/}
  tmpdir=$(mktemp -d)
  bin="$APP_NAME"
  if [ "$os" = "windows" ]; then
    bin="$APP_NAME.exe"
  fi

  GOTOOLCHAIN=local GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X main.version=$VERSION" \
    -o "$tmpdir/$bin" \
    ./cmd/ssh-tunnel

  cp README.md config.example.yaml "$tmpdir/"
  archive_base="${APP_NAME}_${VERSION}_${os}_${arch}"
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
