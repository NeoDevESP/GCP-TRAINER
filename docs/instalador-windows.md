# Instalador de Windows (MSI)

Cloud Mastery completo en tu PC, sin conexión a la nube: simulador, terminal,
los 54 laboratorios, incidentes generados, la empresa Nebula, la web en
español e inglés y tu progreso. No necesitas instalar Go, Node ni ninguna base
de datos.

## Descargar

- **Última versión publicada**: pestaña *Releases* del repositorio
  (`CloudMastery-X.Y.Z-x64.msi`).
- **Versión de cada cambio**: pestaña *Actions → windows-installer*, abre la
  ejecución más reciente y descarga el artefacto `CloudMastery-…-x64-msi`
  (viene dentro de un .zip).

Requisitos: Windows 10 u 11 de 64 bits y un navegador moderno (Edge, Chrome o
Firefox).

## Instalar

1. Haz doble clic en el `.msi`.
2. Windows SmartScreen puede avisar de que es un *editor desconocido*: el
   instalador no está firmado con un certificado de pago. Pulsa **Más
   información → Ejecutar de todas formas**.
3. Acepta el aviso de permisos de administrador (se instala en
   `C:\Program Files\Cloud Mastery`).

Se crean los accesos directos **Cloud Mastery** en el menú Inicio y en el
escritorio. El instalador **no abre el programa al terminar**: ábrelo tú desde
uno de esos accesos directos.

## Usar

1. Abre **Cloud Mastery** desde el menú Inicio o el escritorio.
2. Se abre una ventana negra con la dirección y el navegador en
   **http://127.0.0.1:8765**. Crea tu cuenta la primera vez (es local: no sale
   de tu PC).
3. **Deja la ventana negra abierta** mientras practicas. Ciérrala para salir.

Si vuelves a abrir el acceso directo con la plataforma ya en marcha, solo se
abre el navegador.

La plataforma solo escucha en `127.0.0.1`: nadie más en tu red puede
conectarse, y el Firewall de Windows no pide permiso.

## Tus datos

Se guardan en `%LOCALAPPDATA%\CloudMastery` (pega esa ruta en el Explorador de
archivos):

| Archivo | Qué es |
|---|---|
| `gcplab.json` | Tu cuenta, progreso, laboratorios en curso y empresa |
| `secret.key` | Clave que mantiene tu sesión abierta entre reinicios |
| `gcplab.log` | Registro técnico de la última ejecución (útil si algo falla) |

Para hacer una copia de seguridad, copia esa carpeta. Actualizar o desinstalar
Cloud Mastery **no borra** tus datos.

## Actualizar

Descarga el `.msi` nuevo e instálalo encima: sustituye la versión anterior y
conserva tus datos. Cierra antes la ventana de Cloud Mastery si está abierta.

## Desinstalar

*Configuración → Aplicaciones → Aplicaciones instaladas → Cloud Mastery →
Desinstalar*. Si además quieres borrar tu progreso, borra la carpeta
`%LOCALAPPDATA%\CloudMastery`.

## Si no se abre

1. Ábrelo desde PowerShell para ver el mensaje (pulsa Inicio, escribe
   *PowerShell* y pega):

   ```powershell
   & "C:\Program Files\Cloud Mastery\gcplab.exe" -desktop
   ```

   - Si dice *Cloud Mastery está en marcha*, abre **http://127.0.0.1:8765**
     en el navegador y deja PowerShell abierto.
   - Si dice que **no se encuentra** `gcplab.exe`, el antivirus lo ha puesto en
     cuarentena: *Seguridad de Windows → Protección contra virus y amenazas →
     Historial de protección*, elige Cloud Mastery y pulsa **Acciones →
     Restaurar** (o *Permitir en el dispositivo*). Después reinstala el MSI.
   - Si muestra otro error, cópialo junto con el final del registro:

     ```powershell
     Get-Content "$env:LOCALAPPDATA\CloudMastery\gcplab.log" -Tail 20
     ```

2. Comprueba que tu Windows es 10 u 11 de 64 bits (*Configuración → Sistema →
   Información*). Windows 7 y 8.1 no son compatibles.

## Problemas frecuentes

| Síntoma | Solución |
|---|---|
| SmartScreen bloquea el instalador | *Más información → Ejecutar de todas formas* (instalador sin firma de pago). |
| La ventana se cierra enseguida | Ábrela otra vez; si falla al arrancar, la ventana muestra el motivo y espera a que pulses Intro. Revisa también `%LOCALAPPDATA%\CloudMastery\gcplab.log`. |
| El navegador no se abre | Abre tú **http://127.0.0.1:8765**. |
| Otro programa usa el puerto 8765 | Cloud Mastery prueba automáticamente los puertos 8766 a 8774; la ventana muestra la dirección que ha usado. |
| El antivirus avisa o borra `gcplab.exe` | Es un falso positivo habitual con programas nuevos sin firma. Restáuralo desde el *Historial de protección* (ver arriba). El código fuente está en este repositorio. |
| No pasa nada al terminar de instalar | Es normal: abre **Cloud Mastery** desde el menú Inicio o el escritorio. |

## Para desarrolladores: construir el MSI

Desde Linux o macOS, con Go, Node y msitools (`sudo apt-get install wixl msitools` o
`brew install msitools`):

```sh
./deploy/windows/build-msi.sh              # → dist/CloudMastery-1.0.N-x64.msi
VERSION=1.2.3 ./deploy/windows/build-msi.sh
```

El MSI se define en `deploy/windows/cloudmastery.wxs`. `UpgradeCode` no debe
cambiar nunca: es lo que permite que una versión nueva sustituya a la
anterior. Para publicar una *Release*, crea una etiqueta `vX.Y.Z`; el flujo
`windows-installer` construye el MSI y lo adjunta.

El modo escritorio también funciona en Linux y macOS: `gcplab -desktop`.
