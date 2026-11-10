# Chuleta · Peticiones de plataforma en la consola

Las peticiones que más llegan a un equipo de plataforma, paso a paso **en la
consola de Google Cloud**, con el comando `gcloud` equivalente por si lo
necesitas. Se practican en la Semana 10 de la ruta Zentra
(`content/labs/zentra/p01…p10`).

## Antes de tocar nada

1. **Mira el proyecto** en el selector de arriba. DEV, AU y PRO se parecen
   mucho: el error más caro es hacer bien el cambio en el entorno equivocado.
2. **Proyecto por número.** Los errores de las APIs y muchos nombres de recursos
   usan el **número** del proyecto (12 dígitos), no su ID. En el selector puedes
   buscar por número. En la terminal:
   `gcloud projects list --filter=projectNumber=NÚMERO`.
3. **PRO va con su cambio (CHG)** aprobado. Si el ticket solo pide DEV y AU, PRO
   no se toca.
4. **Haz solo lo pedido, pero revisa lo que el ticket no ve.** Por ejemplo, un
   acceso de un externo que también está en PRO.

| Necesitas | En la consola | En la terminal |
|---|---|---|
| Cambiar de proyecto | Selector de proyectos › buscar por nombre, ID o número | `gcloud config set project ID` |
| Saber el número | Selector (columna Número) o Panel de control | `gcloud projects describe ID --format='value(projectNumber)'` |
| Carpeta y etiquetas | IAM y administración › Configuración / Etiquetas | `gcloud projects describe ID` |

## reCAPTCHA: añadir un dominio a una clave

**Seguridad › reCAPTCHA › Claves** › la clave › **Añadir un dominio** (o
**Editar clave**).

- Solo el nombre del dominio: sin `https://` ni ruta. Incluye sus subdominios.
- Al editar, la lista de dominios es **la lista entera**: si borras uno, la
  clave deja de funcionar en ese dominio.
- Comprueba que es la clave del entorno pedido (el ID de la clave empieza por
  `6L…`; cada entorno tiene la suya).

```sh
gcloud recaptcha keys describe ID_CLAVE
gcloud recaptcha keys update ID_CLAVE --web --domains=dom1,dom2,nuevo   # la lista completa
```

## Buckets en DEV, AU y PRO con el estándar

**Cloud Storage › Buckets › Crear**, una vez por entorno:

- Nombre con el patrón del banco (por ejemplo `gcs-<app>-<entorno>-<número>`).
  El número es el **de ese proyecto**: cópialo del selector, no de otro entorno.
- Ubicación en la región permitida (las políticas de organización rechazan otras).
- Acceso uniforme a nivel de bucket y prevención de acceso público.
- Cifrado con **clave gestionada por el cliente (Cloud KMS)** si el estándar lo
  pide: la clave en la misma ubicación y el agente de Cloud Storage con permiso.
- Etiquetas (`app`, `env`…).

```sh
gcloud storage buckets create gs://NOMBRE --location=REGIÓN --uniform-bucket-level-access \
  --public-access-prevention --default-encryption-key=projects/P/locations/R/keyRings/A/cryptoKeys/C \
  --labels=app=APP,env=ENT
```

Si te equivocas de número en el nombre, el nombre no se puede cambiar: se crea
el bueno y se borra el malo (si está vacío).

## Accesos

**IAM y administración › IAM** › **Conceder acceso** (o el lápiz de una fila).

- Siempre a **grupos**, no a personas.
- El rol mínimo: lectura de objetos es `roles/storage.objectViewer` en el
  bucket, no `editor` en el proyecto.
- Para quitar un acceso: filtra por el correo y quita **cada** rol, en **cada**
  entorno.

```sh
gcloud storage buckets add-iam-policy-binding gs://B --member=group:G --role=roles/storage.objectViewer
gcloud projects get-iam-policy ID --flatten=bindings[].members --filter=bindings.members:CORREO
gcloud projects remove-iam-policy-binding ID --member=user:CORREO --role=ROL
```

## Cuenta de servicio para desplegar desde GitHub

**IAM y administración › Cuentas de servicio › Crear**, con los roles justos
(por ejemplo `roles/run.developer`) y **sin claves JSON**: GitHub entra con
Workload Identity Federation (`roles/iam.workloadIdentityUser` para el
`principalSet` del repositorio).

## APIs

**APIs y servicios › Biblioteca** › la API › **Habilitar**. Se hace por
proyecto: en cada entorno que pida el ticket.

```sh
gcloud services enable secretmanager.googleapis.com --project=ID
```

## Etiquetas de proyecto (identificador de aplicación)

**IAM y administración › Etiquetas** › **Añadir etiqueta**.

- Claves y valores en **minúsculas** (`ASES-0420` → `ases-0420`).
- Añadir no borra las que ya hay.

```sh
gcloud projects update ID --update-labels=app-id=VALOR,capa=frontend
```

## Al cerrar el ticket

Di **qué** has hecho, **dónde** (proyecto e ID o número) y lo que has visto que
el ticket no pedía. Si algo queda para PRO, dilo y con qué cambio.
