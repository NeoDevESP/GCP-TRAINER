#!/usr/bin/env bash
# Builds the portable ZIP of Cloud Mastery: unzip and open, no installer and no
# administrator rights. One folder runs on Windows (x64), macOS (Apple Silicon
# and Intel) and Linux (x64/arm64); the learner's data stays in its "datos"
# folder, so it can live on a USB stick.
#
#   ./deploy/portable/build-zip.sh            # → dist/CloudMastery-<version>-portable.zip
#   SKIP_WEB=1 ./deploy/portable/build-zip.sh  # reuse an existing web/out build
set -euo pipefail
cd "$(dirname "$0")/../.."

VERSION="${VERSION:-1.0.$(git rev-list --count HEAD 2>/dev/null || echo 0)}"
NAME="CloudMastery-${VERSION}"
WORK="build/portable"
STAGE="$WORK/$NAME"
OUT="dist/$NAME-portable.zip"

for t in go zip; do
  command -v "$t" >/dev/null || { echo "Falta $t" >&2; exit 2; }
done

echo "==> Validando el contenido"
go run ./cmd/labctl validate

if [[ -z "${SKIP_WEB:-}" ]]; then
  echo "==> Construyendo la web"
  (cd web && npm ci --no-audit --no-fund && npm run build)
fi
[[ -f web/out/index.html ]] || { echo "No hay web/out: construye la web o quita SKIP_WEB" >&2; exit 2; }

rm -rf "$WORK" && mkdir -p "$STAGE/bin" dist

echo "==> Compilando para Windows x64"
SYSO=cmd/gcplab/rsrc_windows_amd64.syso
trap 'rm -f "$SYSO"' EXIT
go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64 --out cmd/gcplab/rsrc \
  --icon deploy/windows/icon.png --manifest cli \
  --product-name "Cloud Mastery" --file-description "Cloud Mastery - simulador de Google Cloud" \
  --product-version "$VERSION.0" --file-version "$VERSION.0" \
  --copyright "NeoDevESP" --original-filename CloudMastery.exe
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$STAGE/CloudMastery.exe" ./cmd/gcplab
rm -f "$SYSO"

for target in darwin/arm64:mac-arm64 darwin/amd64:mac-intel linux/amd64:linux-x64 linux/arm64:linux-arm64; do
  platform="${target%%:*}"; dir="${target##*:}"
  echo "==> Compilando para $dir"
  mkdir -p "$STAGE/bin/$dir"
  GOOS="${platform%/*}" GOARCH="${platform#*/}" CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$STAGE/bin/$dir/gcplab" ./cmd/gcplab
done

cp -r content "$STAGE/content"
cp -r web/out "$STAGE/web"
cp deploy/portable/LEEME.txt "$STAGE/LEEME.txt"
cp deploy/portable/portable.txt "$STAGE/portable.txt"
cp "deploy/portable/Abrir en Mac.command" "deploy/portable/Abrir en Linux.sh" "$STAGE/"
chmod +x "$STAGE/Abrir en Mac.command" "$STAGE/Abrir en Linux.sh" "$STAGE"/bin/*/gcplab

echo "==> Empaquetando $OUT"
rm -f "$OUT"
(cd "$WORK" && zip -qr -9 -X "../../$OUT" "$NAME")
ls -lh "$OUT"
