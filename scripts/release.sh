#!/bin/sh
# Build the four release archives and their SHA-256 manifest.
set -eu
VERSION=${1:?usage: scripts/release.sh v0.1.0}
case "$VERSION" in v[0-9]* ) ;; *) echo 'Expected a v-prefixed version.' >&2; exit 1 ;; esac
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"
mkdir -p dist
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
trap 'exit 1' HUP INT TERM
for PLATFORM in darwin linux; do
  for ARCH in amd64 arm64; do
    printf 'Building %s/%s %s\n' "$PLATFORM" "$ARCH" "$VERSION"
    CGO_ENABLED=0 GOOS="$PLATFORM" GOARCH="$ARCH" go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$TMP/subscale" ./cmd/subscale
    tar -czf "dist/subscale_${PLATFORM}_${ARCH}.tar.gz" -C "$TMP" subscale -C "$ROOT" README.md
  done
done
python3 - <<'PY'
from pathlib import Path
import hashlib
files = sorted(Path('dist').glob('subscale_*.tar.gz'))
Path('dist/checksums.txt').write_text(''.join(f'{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n' for p in files))
PY
