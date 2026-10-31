#!/bin/bash
# Abre Cloud Mastery en macOS (doble clic). Ciérralo cerrando esta ventana.
cd "$(dirname "$0")" || exit 1
case "$(uname -m)" in
  arm64) BIN="bin/mac-arm64/gcplab" ;;
  *)     BIN="bin/mac-intel/gcplab" ;;
esac
# macOS marca lo descargado como «en cuarentena»; esta carpeta ya la has abierto tú.
xattr -dr com.apple.quarantine . 2>/dev/null
chmod +x "$BIN" 2>/dev/null
exec "./$BIN" -desktop
