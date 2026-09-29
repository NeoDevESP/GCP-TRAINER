#!/usr/bin/env bash
# Builds the Windows installer (MSI) of Cloud Mastery from Linux or macOS.
#
#   ./deploy/windows/build-msi.sh            # → dist/CloudMastery-<version>-x64.msi
#   VERSION=1.2.3 ./deploy/windows/build-msi.sh
#   SKIP_WEB=1 ./deploy/windows/build-msi.sh  # reuse an existing web/out build
#
# Requires Go, Node (for the web client) and msitools (wixl, wixl-heat):
#   sudo apt-get install wixl msitools   # Debian/Ubuntu
#   brew install msitools            # macOS
set -euo pipefail
cd "$(dirname "$0")/../.."

# MSI versions are major.minor.build (build < 65536).
VERSION="${VERSION:-1.0.$(git rev-list --count HEAD 2>/dev/null || echo 0)}"
WORK="build/msi"
STAGE="$WORK/stage"
OUT="dist/CloudMastery-${VERSION}-x64.msi"

for t in go wixl wixl-heat; do
  command -v "$t" >/dev/null || { echo "Falta $t (ver la cabecera de este script)" >&2; exit 2; }
done

echo "==> Validando el contenido"
go run ./cmd/labctl validate

if [[ -z "${SKIP_WEB:-}" ]]; then
  echo "==> Construyendo la web"
  (cd web && npm ci --no-audit --no-fund && npm run build)
fi
[[ -f web/out/index.html ]] || { echo "No hay web/out: construye la web o quita SKIP_WEB" >&2; exit 2; }

echo "==> Compilando gcplab.exe (Windows x64)"
rm -rf "$WORK" && mkdir -p "$STAGE" dist
# Icon, version details and manifest: an exe without them (and with stripped
# symbols) is what antivirus heuristics most often flag as suspicious.
SYSO=cmd/gcplab/rsrc_windows_amd64.syso
trap 'rm -f "$SYSO"' EXIT
go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64 --out cmd/gcplab/rsrc \
  --icon deploy/windows/icon.png --manifest cli \
  --product-name "Cloud Mastery" --file-description "Cloud Mastery - simulador de Google Cloud" \
  --product-version "$VERSION.0" --file-version "$VERSION.0" \
  --copyright "NeoDevESP" --original-filename gcplab.exe
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$STAGE/gcplab.exe" ./cmd/gcplab
rm -f "$SYSO"
cp -r content "$STAGE/content"
cp -r web/out "$STAGE/web"

echo "==> Generando la lista de archivos"
(cd "$STAGE" && find content -type f | LC_ALL=C sort |
  wixl-heat -p content/ --component-group ContentFiles --var var.ContentDir --directory-ref CONTENTDIR --win64) > "$WORK/content.wxs"
(cd "$STAGE" && find web -type f | LC_ALL=C sort |
  wixl-heat -p web/ --component-group WebFiles --var var.WebDir --directory-ref WEBDIR --win64) > "$WORK/web.wxs"

echo "==> Empaquetando $OUT"
wixl -a x64 -D Win64=yes -D "Version=$VERSION" -D "ExeDir=$STAGE" -D "ContentDir=$STAGE/content" -D "WebDir=$STAGE/web" \
  -o "$OUT" deploy/windows/cloudmastery.wxs "$WORK/content.wxs" "$WORK/web.wxs"

ls -lh "$OUT"
