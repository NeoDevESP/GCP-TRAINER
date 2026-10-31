# Versión portátil (ZIP)

Además del instalador de Windows ([instalador-windows.md](instalador-windows.md)) hay una versión portátil:
un ZIP que se descomprime y se abre, sin instalar nada ni permisos de administrador.

- **Windows x64:** doble clic en `CloudMastery.exe`.
- **macOS** (Apple Silicon e Intel): clic derecho en `Abrir en Mac.command` › Abrir.
- **Linux** (x64 y ARM64): `./"Abrir en Linux.sh"`.

El archivo `portable.txt` activa el modo portátil: los datos (cuenta, progreso, registro) se guardan en la
carpeta `datos` junto al programa, así que la carpeta se puede copiar a otro equipo o llevar en un USB. Sin
ese archivo, o si la carpeta no admite escritura, se usa el perfil del usuario como con el instalador.
Instrucciones para el usuario final: [`deploy/portable/LEEME.txt`](../deploy/portable/LEEME.txt).

## Cómo se genera

```bash
./deploy/portable/build-zip.sh                 # → dist/CloudMastery-1.0.N-portable.zip
VERSION=1.2.3 SKIP_WEB=1 ./deploy/portable/build-zip.sh
```

Requiere Go, Node (para la web) y `zip`. Compila para Windows x64 (con icono y datos de versión, sin recortar
símbolos para no despertar a los antivirus), macOS arm64/amd64 y Linux amd64/arm64.
Los binarios no están firmados con un certificado: Windows (SmartScreen) y macOS (Gatekeeper) avisan la
primera vez; el LEEME explica cómo continuar.
