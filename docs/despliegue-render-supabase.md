# Publicar Cloud Mastery en Render + Supabase

Con esta guía tendrás la plataforma online, con una URL propia, para practicar
desde cualquier sitio. No hace falta instalar nada en tu PC.

- **Supabase** guarda los datos (usuarios, progreso, laboratorios en curso,
  empresa) en PostgreSQL.
- **Render** ejecuta la aplicación (web, API, laboratorios y evaluador) en un
  solo servicio Docker construido desde este repositorio, con el archivo
  [`render.yaml`](../render.yaml).

Tiempo aproximado: 15 minutos. Los dos servicios tienen plan gratuito.

## 1. Crear la base de datos en Supabase

1. Entra en <https://supabase.com> y crea una cuenta (puedes usar tu cuenta de
   GitHub).
2. **New project**:
   - *Name*: `cloud-mastery`
   - *Database Password*: pulsa **Generate a password** y **guárdala**; la
     necesitarás en el paso 3.
   - *Region*: **Central EU (Frankfurt)** (`eu-central-1`), la misma zona que
     usará Render.
3. Espera a que el proyecto termine de crearse (1–2 minutos).
4. Pulsa **Connect** (arriba) y, en *Connection string*, elige
   **Session pooler**. Copia la cadena. Tiene esta forma:

   ```
   postgresql://postgres.abcdefghijkl:[YOUR-PASSWORD]@aws-0-eu-central-1.pooler.supabase.com:5432/postgres
   ```

5. Sustituye `[YOUR-PASSWORD]` por la contraseña del paso 2 (sin corchetes) y
   añade `?sslmode=require` al final:

   ```
   postgresql://postgres.abcdefghijkl:MiContraseña@aws-0-eu-central-1.pooler.supabase.com:5432/postgres?sslmode=require
   ```

   Esa es tu `DATABASE_URL`.

Importante:

- Usa **Session pooler**, no *Direct connection*: la conexión directa de
  Supabase solo funciona por IPv6 y Render conecta por IPv4.
- Si la contraseña contiene caracteres como `@`, `:`, `/`, `?` o `#`,
  codifícalos (`@` → `%40`, `:` → `%3A`, `/` → `%2F`, `?` → `%3F`, `#` → `%23`)
  o genera otra contraseña solo con letras y números.
- No tienes que crear tablas: la aplicación crea la suya (`documents`) al
  arrancar y le activa *Row Level Security*, así que la API pública de Supabase
  no puede leerla. Supabase puede avisar de que la tabla no tiene políticas:
  es lo esperado.

## 2. Conectar el repositorio con Render

1. Entra en <https://render.com> y crea una cuenta con **GitHub**.
2. Si el repositorio es privado, autoriza a Render a verlo: *Account Settings
   → GitHub → Configure* y da acceso a `neodevesp/gcp-trainer`.

## 3. Crear el servicio desde el Blueprint

1. En Render: **New + → Blueprint**.
2. Elige el repositorio `neodevesp/gcp-trainer`. Render lee `render.yaml` y
   propone un servicio web llamado **cloud-mastery**.
3. Te pedirá el valor de **`DATABASE_URL`**: pega la cadena del paso 1.5.
   `JWT_SECRET` se genera solo y `PORT` ya viene configurado.
4. Pulsa **Apply**. La primera construcción tarda unos 5–10 minutos: instala
   la web, compila el servidor y valida los 54 laboratorios.
5. Cuando el estado sea **Live**, abre la URL que muestra Render (algo como
   `https://cloud-mastery.onrender.com`), crea tu cuenta y empieza por el
   catálogo.

`render.yaml` apunta a la rama `claude/bold-turing-acsitj`. Cuando la fusiones
con `main`, cambia `branch: main` en el archivo (o en *Settings → Branch* del
servicio). Cada `git push` a esa rama vuelve a desplegar automáticamente.

## 4. Comprobar que todo funciona

- `https://TU-SERVICIO.onrender.com/api/health` debe responder
  `{"labs":54,"ok":true,...}`.
- En Supabase, *Table Editor → documents* muestra filas cuando creas la cuenta
  y empiezas un laboratorio.
- Tu progreso sobrevive a los reinicios: los laboratorios en curso se guardan
  en la base de datos y se reanudan donde los dejaste.

## Planes y límites

| | Gratis | Siguiente paso |
|---|---|---|
| Render | Se duerme tras 15 min sin uso; el primer acceso tarda unos 30–60 s en despertarlo. 512 MB de RAM, suficiente para una persona. | *Starter* (~7 USD/mes): siempre despierto. Cámbialo en `render.yaml` (`plan: starter`) o en el panel. |
| Supabase | 500 MB de base de datos; el proyecto se pausa tras 7 días sin actividad (se reactiva desde el panel). | *Pro* si compartes la plataforma con más gente. |

Mientras el servicio duerme no se pierde nada: los datos están en Supabase.

## Problemas frecuentes

| Síntoma | Causa y solución |
|---|---|
| El despliegue falla con `store` o `failed to connect` | `DATABASE_URL` mal copiada: revisa la contraseña, usa *Session pooler* (`pooler.supabase.com`) y termina con `?sslmode=require`. Corrígela en *Environment* y pulsa *Manual Deploy*. |
| `password authentication failed` | La contraseña no es la del proyecto o tiene caracteres sin codificar. Puedes cambiarla en Supabase: *Project Settings → Database → Reset database password*. |
| `network is unreachable` o conexión a una dirección IPv6 | Estás usando *Direct connection*. Usa la de *Session pooler*. |
| Tras un despliegue hay que volver a iniciar sesión | Ha cambiado `JWT_SECRET`. No lo regeneres; si lo haces, basta con volver a entrar. |
| La web tarda en cargar la primera vez | El plan gratuito estaba dormido. Espera unos segundos y recarga. |

## Opcional

- **Dominio propio**: *Settings → Custom Domains* en Render.
- **Revisor de post-mortems con Claude**: añade `MENTOR_LLM=on` y
  `ANTHROPIC_API_KEY` en *Environment* (tiene coste por uso de la API).
- **Inicio de sesión con Google Workspace**: variables `OIDC_*` del
  [README](../README.md#configuración).
