#!/bin/sh
# Abre Cloud Mastery en Linux. Ciérralo con Ctrl+C en esta terminal.
cd "$(dirname "$0")" || exit 1
case "$(uname -m)" in
  aarch64|arm64) BIN="bin/linux-arm64/gcplab" ;;
  *)             BIN="bin/linux-x64/gcplab" ;;
esac
chmod +x "$BIN" 2>/dev/null
exec "./$BIN" -desktop
