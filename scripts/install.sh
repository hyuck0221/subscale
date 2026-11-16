#!/bin/sh
# Install a published binary. No Go toolchain or repository checkout is required.
set -eu
REPO=hyuck0221/subscale
VERSION=${SUBSCALE_VERSION:-latest}
DEST=${SUBSCALE_INSTALL_DIR:-$HOME/.local/bin}
case "$(uname -s)" in
  Darwin) PLATFORM=darwin ;;
  Linux) PLATFORM=linux ;;
  *) echo 'Supported platforms: macOS and Linux.' >&2; exit 1 ;;
esac
case "$(uname -m)" in
  arm64|aarch64) ARCH=arm64 ;;
  x86_64|amd64) ARCH=amd64 ;;
  *) echo 'Supported architectures: ARM64 and AMD64.' >&2; exit 1 ;;
esac
if [ "$VERSION" = latest ]; then
  BASE="https://github.com/$REPO/releases/latest/download"
else
  case "$VERSION" in v[0-9]* ) ;; *) echo 'SUBSCALE_VERSION must be latest or a v-prefixed release tag.' >&2; exit 1 ;; esac
  BASE="https://github.com/$REPO/releases/download/$VERSION"
fi
command -v curl >/dev/null 2>&1 || { echo 'curl is required.' >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  HASH=sha256sum
elif command -v shasum >/dev/null 2>&1; then
  HASH=shasum
else
  echo 'sha256sum or shasum is required to verify the download.' >&2; exit 1
fi
ARCHIVE="subscale_${PLATFORM}_${ARCH}.tar.gz"
TMP=$(mktemp -d)
STAGED=''
trap 'rm -rf "$TMP"; if [ -n "$STAGED" ]; then rm -f "$STAGED"; fi' EXIT
trap 'exit 1' HUP INT TERM
printf 'Downloading subscale (%s/%s)…\n' "$PLATFORM" "$ARCH"
curl -fL --retry 3 -sS "$BASE/$ARCHIVE" -o "$TMP/$ARCHIVE"
curl -fL --retry 3 -sS "$BASE/checksums.txt" -o "$TMP/checksums.txt"
EXPECTED=$(awk -v file="$ARCHIVE" '$2 == file {print $1}' "$TMP/checksums.txt")
[ "${#EXPECTED}" -eq 64 ] || { echo 'Release checksum is missing or invalid.' >&2; exit 1; }
if [ "$HASH" = sha256sum ]; then
  ACTUAL=$(sha256sum "$TMP/$ARCHIVE" | awk '{print $1}')
else
  ACTUAL=$(shasum -a 256 "$TMP/$ARCHIVE" | awk '{print $1}')
fi
[ "$ACTUAL" = "$EXPECTED" ] || { echo 'Checksum verification failed.' >&2; exit 1; }
tar -xzf "$TMP/$ARCHIVE" -C "$TMP" subscale
mkdir -p "$DEST"
STAGED=$(mktemp "$DEST/.subscale.XXXXXX")
install -m 755 "$TMP/subscale" "$STAGED"
mv -f "$STAGED" "$DEST/subscale"
STAGED=''
printf '\nInstalled %s/subscale\n' "$DEST"
case ":$PATH:" in *":$DEST:"*) ;; *) printf 'Add to your shell configuration: export PATH="%s:$PATH"\n' "$DEST";; esac
if [ "$PLATFORM" = darwin ] && [ "${SUBSCALE_AUTOSTART:-1}" != 0 ]; then
  if ! "$DEST/subscale" service install; then
    printf '\nInstalled, but login startup could not be enabled. Retry: "%s/subscale" service install\n' "$DEST" >&2
  fi
fi
printf '\nNext:\n  subscale work login\n  subscale work add server.example.ts.net\n'
if [ "$PLATFORM" = linux ]; then
  printf '  subscale service install\n'
fi
