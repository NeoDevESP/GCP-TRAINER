# Publicar Cloud Mastery en Koyeb + Supabase

Alternativa a [Render](despliegue-render-supabase.md) y a
[Cloud Run](despliegue-cloud-run.md): Koyeb construye la imagen directamente
desde GitHub y tiene región en Frankfurt. Todo se hace desde su panel web, sin
instalar nada.

Los nombres de los botones pueden variar un poco si Koyeb cambia su panel; los
valores que hay que poner son siempre los de esta guía.

## 1. Base de datos en Supabase

Sigue el [paso 1 de la guía de Render](despliegue-render-supabase.md#1-crear-la-base-de-datos-en-supabase).
Necesitas la `DATABASE_URL` de tipo **Session pooler** terminada en
`?sslmode=require`.

## 2. Crear el servicio en Koyeb

1. Entra en <https://www.koyeb.com> y crea una cuenta con **GitHub**. Si el
   repositorio es privado, autoriza a la aplicación de Koyeb en GitHub a verlo.
2. **Create Web Service → GitHub** y elige `neodevesp/gcp-trainer`, rama
   `claude/bold-turing-acsitj` (o `main` cuando la fusiones).
3. **Builder**: elige **Dockerfile** y rellena:
   - *Dockerfile location*: `deploy/Dockerfile`
   - *Work directory* / contexto: la raíz del repositorio (déjalo vacío)
   - *Target*: `runtime` (si lo pregunta; es el último del archivo)
4. **Environment variables**:
   - `DATABASE_URL`: la de Supabase. Guárdala como **Secret** si Koyeb lo ofrece.
   - `JWT_SECRET`: una frase larga y aleatoria, por ejemplo la salida de
     `openssl rand -base64 48`. No la cambies después o habrá que volver a
     iniciar sesión.
   - `PORT`: `8080`
5. **Exposed ports**: `8080`, protocolo HTTP, ruta `/`.
6. **Health checks**: HTTP en el puerto `8080`, ruta `/api/health`.
7. **Instance**: la gratuita (o la más pequeña). **Region**: Frankfurt.
8. **Scaling**: 1 instancia como máximo. Los laboratorios en curso viven en la
   memoria de la instancia (y se guardan en Supabase).
9. **Deploy**. La primera construcción tarda unos 5–10 minutos. Cuando el
   servicio esté *Healthy*, abre la URL `https://...koyeb.app`, crea tu cuenta
   y empieza por el catálogo.

Cada `git push` a la rama elegida vuelve a desplegar automáticamente.

## Notas

- En el plan gratuito, Koyeb puede dormir el servicio tras un rato sin uso; el
  primer acceso lo despierta en unos segundos. No se pierde nada, porque los
  datos están en Supabase. Consulta sus condiciones actuales en la web.
- Si el despliegue falla al conectar con la base de datos, revisa la tabla de
  [problemas frecuentes](despliegue-render-supabase.md#problemas-frecuentes):
  las causas son las mismas.
