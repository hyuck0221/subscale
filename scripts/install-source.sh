#!/bin/sh
set -eu
# Build locally; authentication is performed later with `subscale NAME login`.
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
DEST="${SUBSCALE_INSTALL_DIR:-$HOME/.local/bin}"
command -v go >/dev/null 2>&1 || { echo 'Go is required to install from source.' >&2; exit 1; }
case "$(uname -s)" in Darwin|Linux) ;; *) echo 'Supported platforms: macOS and Linux.' >&2; exit 1;; esac
mkdir -p "$DEST"
cd "$ROOT"
go build -trimpath -o "$DEST/subscale" ./cmd/subscale
printf '\nInstalled %s/subscale\n' "$DEST"
case ":$PATH:" in *":$DEST:"*) ;; *) printf 'Add this directory to PATH: %s\n' "$DEST";; esac
printf '\nNext:\n  subscale service install\n  subscale work login\n  subscale work add server.example.ts.net\n'
