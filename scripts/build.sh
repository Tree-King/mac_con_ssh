#!/usr/bin/env sh
set -eu

APP_NAME=${APP_NAME:-ssh-tunnel}
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
GOOS_TARGET=${GOOS_TARGET:-$(go env GOOS)}
GOARCH_TARGET=${GOARCH_TARGET:-$(go env GOARCH)}
OUT_DIR=${OUT_DIR:-dist}
BUILD_TAGS=${BUILD_TAGS:-}
OUTPUT_SUFFIX=${OUTPUT_SUFFIX:-}

mkdir -p "$OUT_DIR"
EXT=""
if [ "$GOOS_TARGET" = "windows" ]; then
  EXT=".exe"
fi

OUTPUT="$OUT_DIR/${APP_NAME}_${VERSION}_${GOOS_TARGET}_${GOARCH_TARGET}${OUTPUT_SUFFIX}${EXT}"
BUILD_ARGS=""
if [ -n "$BUILD_TAGS" ]; then
  BUILD_ARGS="-tags $BUILD_TAGS"
fi

# shellcheck disable=SC2086
GOTOOLCHAIN=local GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" CGO_ENABLED=0 go build \
  $BUILD_ARGS \
  -trimpath \
  -ldflags "-s -w -X main.version=$VERSION" \
  -o "$OUTPUT" \
  ./cmd/ssh-tunnel

echo "$OUTPUT"
