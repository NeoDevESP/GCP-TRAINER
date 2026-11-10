# Baseline: recursos de los proyectos de aplicación de Zentra en DEV, AU y PRO
# (los proyectos los crea la base «zentra-entornos»). Va después de
# «zentra-core» (directorio y grupos) y de «zentra-entornos».
#
# Normas del banco en todos los proyectos de aplicación: sin buckets públicos,
# sin claves de cuentas de servicio y datos solo en la UE.
for p in dev-zentra-altas au-zentra-altas zentra-altas dev-zentra-cartera au-zentra-cartera zentra-cartera dev-zentra-asesores au-zentra-asesores zentra-asesores dev-zentra-asesores-sp au-zentra-asesores-sp zentra-asesores-sp; do
  gcloud resource-manager org-policies enable-enforce storage.publicAccessPrevention --project=$p
  gcloud resource-manager org-policies enable-enforce iam.managed.disableServiceAccountKeyCreation --project=$p
  gcloud resource-manager org-policies allow gcp.resourceLocations in:eu-locations --project=$p
done
# Cifrado con clave propia (CMEK) para los buckets: una clave por proyecto,
# con permiso ya dado al agente de Cloud Storage de cada uno.
for p in dev-zentra-altas au-zentra-altas zentra-altas dev-zentra-cartera au-zentra-cartera zentra-cartera dev-zentra-asesores au-zentra-asesores zentra-asesores dev-zentra-asesores-sp au-zentra-asesores-sp zentra-asesores-sp; do
  gcloud kms keyrings create zentra-almacen --location={{.region}} --project=$p
  gcloud kms keys create gcs --keyring=zentra-almacen --location={{.region}} --purpose=encryption --project=$p
  gcloud kms keys add-iam-policy-binding gcs --keyring=zentra-almacen --location={{.region}} --project=$p --member=serviceAccount:$(gcloud storage service-agent --project=$p) --role=roles/cloudkms.cryptoKeyEncrypterDecrypter
done
# reCAPTCHA del formulario de alta de clientes: una clave por entorno.
gcloud recaptcha keys create --project=dev-zentra-altas --key-id=6LdZtR8qAAAAAJx3wPm7vK2nYq5cT9bH4sLfE1uA --display-name=altas-web-dev --web --integration-type=score --domains=altas.dev.zentra.dev,localhost
gcloud recaptcha keys create --project=au-zentra-altas --key-id=6LcQ2nWpAAAAAHk8Rt3vMx7bZq1sY6dJ9fGa4LeP --display-name=altas-web-au --web --integration-type=score --domains=altas.au.zentra.dev
gcloud recaptcha keys create --project=zentra-altas --key-id=6LfB7yKsAAAAAN5mQw2rT8vXc3hZp6jL1dEu9SgR --display-name=altas-web --web --integration-type=score --domains=altas.zentra.dev,www.zentra.dev
# Equipos de cartera y asesores.
gcloud identity groups create gcp-cartera-dev@zentra.dev --organization=zentra.dev --display-name="GCP Cartera - desarrollo" --labels=cloudidentity.googleapis.com/groups.security
gcloud identity groups create gcp-cartera-qa@zentra.dev --organization=zentra.dev --display-name="GCP Cartera - QA" --labels=cloudidentity.googleapis.com/groups.security
gcloud identity groups create gcp-asesores-dev@zentra.dev --organization=zentra.dev --display-name="GCP Asesores - desarrollo" --labels=cloudidentity.googleapis.com/groups.security
gcloud identity groups memberships add --group-email=gcp-cartera-dev@zentra.dev --member-email=sara.benitez@zentra.dev
gcloud identity groups memberships add --group-email=gcp-cartera-qa@zentra.dev --member-email=pablo.costa@zentra.dev
gcloud identity groups memberships add --group-email=gcp-asesores-dev@zentra.dev --member-email=hugo.martin@zentra.dev
gcloud projects add-iam-policy-binding dev-zentra-cartera --member=group:gcp-cartera-dev@zentra.dev --role=roles/viewer
gcloud projects add-iam-policy-binding au-zentra-cartera --member=group:gcp-cartera-dev@zentra.dev --role=roles/viewer
