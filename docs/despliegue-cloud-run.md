# Publicar Cloud Mastery en Google Cloud Run + Supabase

Con esta guía tendrás la plataforma online en **Google Cloud Run**, con los
datos en **Supabase**. Además de practicar con la plataforma, montarla ya es
práctica de la certificación ACE: Artifact Registry, Cloud Build, Cloud Run,
Secret Manager, cuentas de servicio con mínimo privilegio e IAM.

- **Cloud Run** ejecuta la aplicación (web, API, laboratorios y evaluador) en
  un único servicio. Sin uso baja a cero instancias y no cuesta nada; el primer
  acceso tarda unos segundos en arrancarla.
- **Supabase** guarda los usuarios, el progreso y los laboratorios en curso,
  así que no se pierde nada cuando Cloud Run apaga la instancia.

Tiempo aproximado: 20–30 minutos la primera vez. Después, publicar una versión
nueva es un solo comando.

## 1. Base de datos en Supabase

Sigue el [paso 1 de la guía de Render](despliegue-render-supabase.md#1-crear-la-base-de-datos-en-supabase):
es idéntico. Al final tendrás una `DATABASE_URL` de tipo **Session pooler** que
termina en `?sslmode=require`. Elige la región **Central EU (Frankfurt)**, cerca
de Cloud Run en `europe-west1`.

## 2. Proyecto de Google Cloud

1. Entra en <https://console.cloud.google.com> con tu cuenta de Google.
2. Crea un proyecto nuevo (*Seleccionar proyecto → Proyecto nuevo*), por
   ejemplo `cloud-mastery-tunombre`. Apunta su **ID** (aparece debajo del
   nombre; puede llevar números al final).
3. Vincula una **cuenta de facturación** al proyecto (*Facturación*). Cloud Run
   la exige aunque te quedes dentro de la capa gratuita. Las cuentas nuevas
   suelen incluir crédito de prueba.
4. Recomendado: crea un **presupuesto** con alertas (*Facturación →
   Presupuestos y alertas*), por ejemplo de 5 EUR al mes con avisos al 50, 90 y
   100 %. Recuerda que un presupuesto avisa pero no detiene el gasto (es una
   pregunta típica del examen).

## 3. Publicar

Elige una de las dos formas. El script es el mismo en ambas y puedes
ejecutarlo tantas veces como quieras: crea lo que falta y publica una versión
nueva.

### Opción A: desde el navegador con Cloud Shell (no instalas nada)

1. Abre <https://shell.cloud.google.com>.
2. Descarga el repositorio:

   ```sh
   git clone -b claude/bold-turing-acsitj https://github.com/neodevesp/gcp-trainer.git
   cd gcp-trainer
   ```

   Si el repositorio es privado, Git pedirá usuario y contraseña: escribe tu
   usuario de GitHub y, como contraseña, un *token* creado en GitHub → *Settings
   → Developer settings → Personal access tokens → Fine-grained tokens* con
   permiso de solo lectura (*Contents: Read-only*) sobre este repositorio.
3. Ejecuta el script con el ID de tu proyecto:

   ```sh
   ./deploy/cloudrun/deploy.sh TU-PROYECTO
   ```

   Pega la `DATABASE_URL` cuando la pida (no se ve al escribirla).

### Opción B: desde tu PC con Windows (PowerShell)

1. Instala Git y la CLI de Google Cloud, y cierra y vuelve a abrir PowerShell:

   ```powershell
   winget install --id Git.Git -e
   winget install --id Google.CloudSDK -e
   ```

2. Inicia sesión en Google Cloud (se abre el navegador):

   ```powershell
   gcloud auth login
   ```

3. Descarga el repositorio y ejecuta el script:

   ```powershell
   cd $HOME
   git clone -b claude/bold-turing-acsitj https://github.com/neodevesp/gcp-trainer.git
   cd gcp-trainer
   powershell -ExecutionPolicy Bypass -File deploy\cloudrun\deploy.ps1 -ProjectId TU-PROYECTO
   ```

   Pega la `DATABASE_URL` cuando la pida.

En los dos casos, al terminar verás `==> Listo: https://cloud-mastery-xxxx.europe-west1.run.app`.
Abre esa URL, crea tu cuenta y empieza por el catálogo. La primera publicación
tarda unos 5–10 minutos, porque Cloud Build construye la imagen y valida los
54 laboratorios.

## 4. Qué ha creado el script (y por qué)

| Recurso | Para qué | Concepto ACE |
|---|---|---|
| API de Cloud Run, Cloud Build, Artifact Registry y Secret Manager | Activar los servicios que se usan | Habilitar API por proyecto |
| Repositorio Docker `cloud-mastery` en Artifact Registry | Guardar las imágenes de la aplicación | Artifact Registry |
| Cuenta de servicio `cm-builder` | Identidad de las compilaciones: solo puede escribir imágenes en ese repositorio, escribir registros y leer el código subido | Mínimo privilegio, cuenta de servicio de Cloud Build propia |
| Cuenta de servicio `cm-runtime` | Identidad del servicio: solo puede leer sus dos secretos | Identidad de la carga de trabajo, sin roles básicos |
| Secretos `cm-database-url` y `cm-jwt-secret` en Secret Manager | La conexión a Supabase y la clave que firma las sesiones, fuera del código y de las variables en claro | Secret Manager montado como variable de entorno |
| Bucket `TU-PROYECTO-cm-build` | Código subido para compilar; se borra solo a los 7 días | Ciclo de vida de objetos, UBLA, prevención de acceso público |
| Servicio de Cloud Run `cloud-mastery` | La aplicación: 1 vCPU, 512 MiB, de 0 a 1 instancias, acceso público | Cloud Run, escalado a cero, `--allow-unauthenticated` |

El servicio se limita a una instancia (`--max-instances=1`) porque los
laboratorios en curso se ejecutan en la memoria de la instancia. También se
guardan en Supabase, así que se reanudan si la instancia se apaga.

## 5. Actualizar

Cada vez que quieras publicar los últimos cambios del repositorio:

```sh
git pull
./deploy/cloudrun/deploy.sh TU-PROYECTO        # o deploy.ps1 en Windows
```

Para cambiar la `DATABASE_URL`, ejecútalo con la variable definida:
`DATABASE_URL='postgresql://...' ./deploy/cloudrun/deploy.sh TU-PROYECTO`
(o `-DatabaseUrl '...'` en PowerShell). Se añade una versión nueva del
secreto. El `JWT_SECRET` no cambia nunca, así que las sesiones siguen abiertas.

## 6. Coste

Con uso personal deberías quedarte dentro de la capa gratuita:

- **Cloud Run** tiene una cuota gratuita mensual de peticiones, CPU y memoria,
  y sin uso no hay instancias.
- **Cloud Build** incluye minutos de compilación gratis al mes.
- **Artifact Registry** incluye una cantidad pequeña de almacenamiento
  gratuito. Cada publicación deja una imagen de unos 140 MB: borra las antiguas
  de vez en cuando (*Artifact Registry → cloud-mastery → app*).
- **Secret Manager** incluye unos pocos secretos y accesos gratis.

Las cuotas exactas cambian con el tiempo: consúltalas en la página de precios
de cada producto y deja activado el presupuesto del paso 2.

## 7. Problemas frecuentes

| Síntoma | Causa y solución |
|---|---|
| `Billing account ... not found` o `billing is disabled` | El proyecto no tiene facturación vinculada (paso 2.3). |
| `One or more users named in the policy do not belong to a permitted customer` al desplegar | Tu cuenta es de una organización (Google Workspace) con la política *Domain restricted sharing*, que prohíbe `allUsers`. Usa un proyecto de una cuenta personal (@gmail.com) o pide a quien administre la organización una excepción para este proyecto. |
| La compilación falla con `PERMISSION_DENIED` al escribir en Artifact Registry o en los registros | Los permisos recién concedidos pueden tardar un par de minutos en aplicarse. Vuelve a ejecutar el script. |
| El servicio arranca pero la aplicación falla con `store` o `failed to connect` | `DATABASE_URL` incorrecta. Revisa la guía de Supabase (Session pooler, contraseña, `?sslmode=require`) y vuelve a ejecutar el script con `DATABASE_URL` definida. Los registros están en *Cloud Run → cloud-mastery → Registros*. |
| `ExecutionPolicy` impide ejecutar el script en Windows | Ejecútalo como se indica, con `powershell -ExecutionPolicy Bypass -File ...`; solo afecta a esa ejecución. |
| La web tarda unos segundos la primera vez | Cloud Run estaba a cero instancias y arranca una. Es lo esperado. |

## 8. Borrarlo todo

Si ya no lo usas, borra el proyecto completo (*IAM y administración → Configuración
→ Cerrar*) y no quedará nada facturable. Los datos siguen en Supabase hasta que
borres también ese proyecto.
