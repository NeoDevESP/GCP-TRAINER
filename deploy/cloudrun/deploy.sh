#!/usr/bin/env bash
# Publica Cloud Mastery en Google Cloud Run con la base de datos en Supabase.
# Pensado para Cloud Shell (https://shell.cloud.google.com), sin instalar nada.
# Guía: docs/despliegue-cloud-run.md
#
#   ./deploy/cloudrun/deploy.sh PROJECT_ID [REGION]
#
# La primera vez pide la DATABASE_URL de Supabase (o léela de la variable de
# entorno DATABASE_URL). Volver a ejecutarlo publica una versión nueva; lo que
# ya existe no se toca.
set -euo pipefail

PROJECT="${1:-${PROJECT:-}}"
REGION="${2:-${REGION:-europe-west1}}"
SERVICE="cloud-mastery"
REPO="cloud-mastery"
BUILDER="cm-builder"
RUNTIME="cm-runtime"
BUCKET="${PROJECT}-cm-build"
SECRET_DB="cm-database-url"
SECRET_JWT="cm-jwt-secret"

if [[ -z "$PROJECT" ]]; then
  echo "Uso: $0 PROJECT_ID [REGION]" >&2
  exit 2
fi
cd "$(dirname "$0")/../.."   # raíz del repositorio

step() { printf '\n==> %s\n' "$*"; }
exists() { "$@" >/dev/null 2>&1; }
sa() { echo "$1@${PROJECT}.iam.gserviceaccount.com"; }

step "Proyecto ${PROJECT}, región ${REGION}"
gcloud config set project "$PROJECT" >/dev/null

step "Activando las API (Cloud Run, Cloud Build, Artifact Registry, Secret Manager)"
gcloud services enable run.googleapis.com cloudbuild.googleapis.com \
  artifactregistry.googleapis.com secretmanager.googleapis.com

step "Repositorio de imágenes ${REPO}"
exists gcloud artifacts repositories describe "$REPO" --location="$REGION" ||
  gcloud artifacts repositories create "$REPO" --repository-format=docker --location="$REGION" \
    --description="Imágenes de Cloud Mastery"

step "Cuentas de servicio (mínimo privilegio: una compila, otra ejecuta)"
exists gcloud iam service-accounts describe "$(sa $BUILDER)" ||
  gcloud iam service-accounts create "$BUILDER" --display-name="Cloud Mastery: compilación"
exists gcloud iam service-accounts describe "$(sa $RUNTIME)" ||
  gcloud iam service-accounts create "$RUNTIME" --display-name="Cloud Mastery: ejecución"

step "Secretos en Secret Manager"
if ! exists gcloud secrets describe "$SECRET_DB"; then
  if [[ -z "${DATABASE_URL:-}" ]]; then
    read -r -s -p "Pega la DATABASE_URL de Supabase (Session pooler) y pulsa Intro: " DATABASE_URL
    echo
  fi
  [[ "$DATABASE_URL" == postgres* ]] || { echo "La DATABASE_URL debe empezar por postgresql://" >&2; exit 2; }
  printf '%s' "$DATABASE_URL" | gcloud secrets create "$SECRET_DB" --replication-policy=automatic --data-file=-
elif [[ -n "${DATABASE_URL:-}" ]]; then
  printf '%s' "$DATABASE_URL" | gcloud secrets versions add "$SECRET_DB" --data-file=-
fi
# JWT_SECRET se genera una sola vez: si cambia, todo el mundo tendría que volver a entrar.
exists gcloud secrets describe "$SECRET_JWT" ||
  head -c 48 /dev/urandom | base64 | tr -d '\n' | gcloud secrets create "$SECRET_JWT" --replication-policy=automatic --data-file=-

step "Permisos"
for s in "$SECRET_DB" "$SECRET_JWT"; do
  gcloud secrets add-iam-policy-binding "$s" --member="serviceAccount:$(sa $RUNTIME)" \
    --role=roles/secretmanager.secretAccessor >/dev/null
done
gcloud artifacts repositories add-iam-policy-binding "$REPO" --location="$REGION" \
  --member="serviceAccount:$(sa $BUILDER)" --role=roles/artifactregistry.writer >/dev/null
gcloud projects add-iam-policy-binding "$PROJECT" --member="serviceAccount:$(sa $BUILDER)" \
  --role=roles/logging.logWriter --condition=None >/dev/null

step "Bucket del código fuente de las compilaciones (se borra solo a los 7 días)"
if ! exists gcloud storage buckets describe "gs://${BUCKET}"; then
  gcloud storage buckets create "gs://${BUCKET}" --location="$REGION" \
    --uniform-bucket-level-access --public-access-prevention
  printf '{"rule":[{"action":{"type":"Delete"},"condition":{"age":7}}]}' > /tmp/cm-lifecycle.json
  gcloud storage buckets update "gs://${BUCKET}" --lifecycle-file=/tmp/cm-lifecycle.json
fi
gcloud storage buckets add-iam-policy-binding "gs://${BUCKET}" \
  --member="serviceAccount:$(sa $BUILDER)" --role=roles/storage.objectViewer >/dev/null

TAG="$(date -u +%Y%m%d-%H%M%S)"
IMAGE="${REGION}-docker.pkg.dev/${PROJECT}/${REPO}/app:${TAG}"
step "Compilando la imagen en Cloud Build (5-10 minutos la primera vez)"
gcloud builds submit . --region="$REGION" --config=deploy/cloudrun/cloudbuild.yaml \
  --substitutions="_IMAGE=${IMAGE}" \
  --service-account="projects/${PROJECT}/serviceAccounts/$(sa $BUILDER)" \
  --gcs-source-staging-dir="gs://${BUCKET}/source"

step "Desplegando en Cloud Run"
# max-instances=1: los laboratorios en curso viven en memoria de una sola instancia
# (y se guardan en la base de datos); min-instances=0: sin uso no cuesta nada.
gcloud run deploy "$SERVICE" --image="$IMAGE" --region="$REGION" \
  --service-account="$(sa $RUNTIME)" --allow-unauthenticated \
  --port=8080 --cpu=1 --memory=512Mi --min-instances=0 --max-instances=1 \
  --set-secrets="DATABASE_URL=${SECRET_DB}:latest,JWT_SECRET=${SECRET_JWT}:latest" --quiet

URL="$(gcloud run services describe "$SERVICE" --region="$REGION" --format='value(status.url)')"
step "Listo: ${URL}"
curl -fsS "${URL}/api/health" && echo || echo "(el servicio aún está arrancando; prueba ${URL}/api/health en unos segundos)"
