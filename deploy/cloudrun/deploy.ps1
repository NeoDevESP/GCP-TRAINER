<#
.SYNOPSIS
  Publica Cloud Mastery en Google Cloud Run con la base de datos en Supabase.

.DESCRIPTION
  Versión para Windows PowerShell 5.1 o PowerShell 7 del script deploy.sh.
  Guía: docs/despliegue-cloud-run.md

  Requisitos: Google Cloud CLI instalado y sesión iniciada (gcloud auth login).
  La primera vez pide la DATABASE_URL de Supabase. Volver a ejecutarlo publica
  una versión nueva; lo que ya existe no se toca.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File deploy\cloudrun\deploy.ps1 -ProjectId mi-proyecto
#>
param(
  [Parameter(Mandatory = $true)][string]$ProjectId,
  [string]$Region = "europe-west1",
  [string]$DatabaseUrl = $env:DATABASE_URL
)

$ErrorActionPreference = "Continue"   # los errores de gcloud se comprueban con $LASTEXITCODE
$Service = "cloud-mastery"; $Repo = "cloud-mastery"
$Builder = "cm-builder"; $Runtime = "cm-runtime"
$Bucket = "$ProjectId-cm-build"
$SecretDb = "cm-database-url"; $SecretJwt = "cm-jwt-secret"

function Step($msg) { Write-Host ""; Write-Host "==> $msg" -ForegroundColor Cyan }
function Sa($name) { return "$name@$ProjectId.iam.gserviceaccount.com" }

# Ejecuta gcloud y se detiene si falla.
function Invoke-Gcloud {
  & gcloud @args
  if ($LASTEXITCODE -ne 0) { throw "Ha fallado: gcloud $($args -join ' ')" }
}
# Devuelve $true si el recurso existe.
function Exists {
  & gcloud @args 2>$null | Out-Null
  return ($LASTEXITCODE -eq 0)
}
# Escribe un texto en un archivo temporal sin BOM ni salto de línea final
# (una tubería de PowerShell añadiría un salto de línea al secreto).
function TempFileWith($text) {
  $f = [System.IO.Path]::GetTempFileName()
  [System.IO.File]::WriteAllText($f, $text)
  return $f
}

if (-not (Get-Command gcloud -ErrorAction SilentlyContinue)) {
  Write-Error "No encuentro gcloud. Instálalo desde https://cloud.google.com/sdk/docs/install y ejecuta 'gcloud auth login'."
  exit 2
}
Set-Location (Resolve-Path (Join-Path $PSScriptRoot "../.."))   # raíz del repositorio

try {
  Step "Proyecto $ProjectId, región $Region"
  Invoke-Gcloud config set project $ProjectId

  Step "Activando las API (Cloud Run, Cloud Build, Artifact Registry, Secret Manager)"
  Invoke-Gcloud services enable run.googleapis.com cloudbuild.googleapis.com artifactregistry.googleapis.com secretmanager.googleapis.com

  Step "Repositorio de imágenes $Repo"
  if (-not (Exists artifacts repositories describe $Repo "--location=$Region")) {
    Invoke-Gcloud artifacts repositories create $Repo --repository-format=docker "--location=$Region" "--description=Imágenes de Cloud Mastery"
  }

  Step "Cuentas de servicio (mínimo privilegio: una compila, otra ejecuta)"
  if (-not (Exists iam service-accounts describe (Sa $Builder))) {
    Invoke-Gcloud iam service-accounts create $Builder "--display-name=Cloud Mastery: compilación"
  }
  if (-not (Exists iam service-accounts describe (Sa $Runtime))) {
    Invoke-Gcloud iam service-accounts create $Runtime "--display-name=Cloud Mastery: ejecución"
  }

  Step "Secretos en Secret Manager"
  $dbExists = Exists secrets describe $SecretDb
  if (-not $dbExists -and -not $DatabaseUrl) {
    $secure = Read-Host "Pega la DATABASE_URL de Supabase (Session pooler)" -AsSecureString
    $DatabaseUrl = [Runtime.InteropServices.Marshal]::PtrToStringBSTR([Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure))
  }
  if ($DatabaseUrl) {
    $DatabaseUrl = $DatabaseUrl.Trim()
    if (-not $DatabaseUrl.StartsWith("postgres")) { throw "La DATABASE_URL debe empezar por postgresql://" }
    $f = TempFileWith $DatabaseUrl
    try {
      if ($dbExists) { Invoke-Gcloud secrets versions add $SecretDb "--data-file=$f" }
      else { Invoke-Gcloud secrets create $SecretDb --replication-policy=automatic "--data-file=$f" }
    } finally { Remove-Item $f -Force }
  }
  # JWT_SECRET se genera una sola vez: si cambia, todo el mundo tendría que volver a entrar.
  if (-not (Exists secrets describe $SecretJwt)) {
    $bytes = New-Object byte[] 48
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
    $f = TempFileWith ([Convert]::ToBase64String($bytes))
    try { Invoke-Gcloud secrets create $SecretJwt --replication-policy=automatic "--data-file=$f" } finally { Remove-Item $f -Force }
  }

  Step "Permisos"
  foreach ($s in @($SecretDb, $SecretJwt)) {
    Invoke-Gcloud secrets add-iam-policy-binding $s "--member=serviceAccount:$(Sa $Runtime)" --role=roles/secretmanager.secretAccessor | Out-Null
  }
  Invoke-Gcloud artifacts repositories add-iam-policy-binding $Repo "--location=$Region" "--member=serviceAccount:$(Sa $Builder)" --role=roles/artifactregistry.writer | Out-Null
  Invoke-Gcloud projects add-iam-policy-binding $ProjectId "--member=serviceAccount:$(Sa $Builder)" --role=roles/logging.logWriter --condition=None | Out-Null

  Step "Bucket del código fuente de las compilaciones (se borra solo a los 7 días)"
  if (-not (Exists storage buckets describe "gs://$Bucket")) {
    Invoke-Gcloud storage buckets create "gs://$Bucket" "--location=$Region" --uniform-bucket-level-access --public-access-prevention
    $f = TempFileWith '{"rule":[{"action":{"type":"Delete"},"condition":{"age":7}}]}'
    try { Invoke-Gcloud storage buckets update "gs://$Bucket" "--lifecycle-file=$f" } finally { Remove-Item $f -Force }
  }
  Invoke-Gcloud storage buckets add-iam-policy-binding "gs://$Bucket" "--member=serviceAccount:$(Sa $Builder)" --role=roles/storage.objectViewer | Out-Null

  $tag = (Get-Date).ToUniversalTime().ToString("yyyyMMdd-HHmmss")
  $image = "$Region-docker.pkg.dev/$ProjectId/$Repo/app:$tag"
  Step "Compilando la imagen en Cloud Build (5-10 minutos la primera vez)"
  Invoke-Gcloud builds submit . "--region=$Region" --config=deploy/cloudrun/cloudbuild.yaml "--substitutions=_IMAGE=$image" `
    "--service-account=projects/$ProjectId/serviceAccounts/$(Sa $Builder)" "--gcs-source-staging-dir=gs://$Bucket/source"

  Step "Desplegando en Cloud Run"
  # max-instances=1: los laboratorios en curso viven en memoria de una sola instancia
  # (y se guardan en la base de datos); min-instances=0: sin uso no cuesta nada.
  Invoke-Gcloud run deploy $Service "--image=$image" "--region=$Region" "--service-account=$(Sa $Runtime)" --allow-unauthenticated `
    --port=8080 --cpu=1 --memory=512Mi --min-instances=0 --max-instances=1 `
    "--set-secrets=DATABASE_URL=${SecretDb}:latest,JWT_SECRET=${SecretJwt}:latest" --quiet

  $url = (& gcloud run services describe $Service "--region=$Region" "--format=value(status.url)").Trim()
  Step "Listo: $url"
  try { Invoke-RestMethod "$url/api/health" -TimeoutSec 60 | ConvertTo-Json -Compress | Write-Host }
  catch { Write-Host "(el servicio aún está arrancando; prueba $url/api/health en unos segundos)" }
}
catch {
  Write-Host ""
  Write-Host $_.Exception.Message -ForegroundColor Red
  Write-Host "Consulta 'Problemas frecuentes' en docs/despliegue-cloud-run.md" -ForegroundColor Yellow
  exit 1
}
