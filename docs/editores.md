# Conectar VS Code, Antigravity u otra terminal con el simulador

`cm` (en `cmd/cm`, incluido en el instalador y en el ZIP portable) une tu
editor con el Google Cloud simulado de Cloud Mastery. Los comandos se
ejecutan en el simulador, **nunca en Google Cloud real**, sobre el mismo
estado que ves en la consola web.

## Puesta en marcha

1. Abre Cloud Mastery (app de escritorio o portable: `http://127.0.0.1:8765`;
   en otro servidor, `-server URL` o la variable `CM_SERVER`).
2. `cm login` — pide correo y contraseña de tu cuenta de Cloud Mastery. La
   sesión se guarda en tu carpeta de configuración, legible solo por ti
   (`cm.json`; caduca a las 12 h: vuelve a hacer `cm login`).
3. `cm vscode <carpeta del proyecto>` — escribe en `.vscode/`:
   - `settings.json`: perfiles de terminal **Cloud Mastery** (una Cloud
     Shell interactiva) y **Cloud Mastery (gcloud)** (tu shell de siempre con
     `gcloud`, `kubectl`, `gsutil`, `bq`, `gam` y `terraform` redirigidos al
     simulador, solo en esa terminal);
   - `mcp.json`: el servidor MCP `cloudmastery` para el agente de VS Code.
   Si ya tenías esos archivos, se fusionan; si tienen comentarios no se tocan
   y se muestra lo que hay que pegar.
4. **Antigravity**: está hecho sobre VS Code, así que usa los mismos perfiles
   de terminal. Para su agente: panel del agente › `…` › MCP Servers › Manage
   MCP Servers › View raw config, y añade lo que imprime `cm vscode`:

   ```json
   { "mcpServers": { "cloudmastery": { "command": "<ruta a cm>", "args": ["mcp"] } } }
   ```

## Comandos

| Comando | Qué hace |
|---|---|
| `cm sandbox [-world zentra]` / `-resume` | Abre una consola libre (o continúa la guardada). `cm worlds` lista los mundos. |
| `cm attach <sesión o URL del laboratorio>` | Trabaja sobre un laboratorio que tienes abierto en el navegador. |
| `cm shell` | Cloud Shell interactiva en la terminal (`exit` para salir). |
| `cm run -- <comando>` | Un comando suelto; devuelve su código de salida. |
| `cm push <archivo\|carpeta> [ruta]` / `cm pull [carpeta]` | Copia archivos a / desde el home de Cloud Shell (p. ej. Terraform editado en el editor). |
| `cm save` | Guarda el mundo de la consola libre. |
| `cm shims [carpeta]` | Crea los atajos `gcloud`, `kubectl`… para añadirlos al `PATH` de una terminal. |
| `cm status`, `cm logout` | Estado y cierre de sesión. |

## Herramientas MCP (para agentes)

`run_command`, `start_free_console`, `list_worlds`, `session_info`,
`save_world`, `write_file`, `list_files`. El agente y tú comparten la
sesión: lo que haga aparece en tu consola web y en tu terminal.

## Límites

- Si no hay sesión abierta, `cm run`, `cm shell` y el agente abren una
  consola libre «Proyecto nuevo».
- Los programas interactivos de pantalla completa (editores, `top`) no
  funcionan en `cm shell`; sí `gcloud compute ssh`, `psql`, etc., que
  cambian el prompt como en la web.
- Los archivos que se suben deben ser de texto y de menos de 400 KB.
