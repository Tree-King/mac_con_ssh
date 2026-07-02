#!/usr/bin/env sh
set -eu

APP_NAME=${APP_NAME:-ssh-tunnel}
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
GOOS_TARGET=${GOOS_TARGET:-$(go env GOOS)}
GOARCH_TARGET=${GOARCH_TARGET:-$(go env GOARCH)}
OUT_DIR=${OUT_DIR:-dist}

mkdir -p "$OUT_DIR"
EXT=""
if [ "$GOOS_TARGET" = "windows" ]; then
  EXT=".exe"
fi

OUTPUT="$OUT_DIR/${APP_NAME}_${VERSION}_${GOOS_TARGET}_${GOARCH_TARGET}${EXT}"
GOTOOLCHAIN=local GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" CGO_ENABLED=0 go build \
  -trimpath \
  -ldflags "-s -w -X main.version=$VERSION" \
  -o "$OUTPUT" \
  ./cmd/ssh-tunnel

echo "$OUTPUT"
