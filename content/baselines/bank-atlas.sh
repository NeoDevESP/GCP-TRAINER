# Baseline: plataforma GCP de un banco regulado (ficticio: Banco Atlas).
# Guardarraíles de organización típicos de banca, red privada con salida por
# Cloud NAT y SSH solo por IAP, identidades por aplicación y una app interna
# de «consulta de movimientos» (API en VM + base de datos PostgreSQL).
gcloud resource-manager org-policies enable-enforce compute.vmExternalIpAccess --project={{.project}}
gcloud resource-manager org-policies enable-enforce sql.restrictPublicIp --project={{.project}}
gcloud resource-manager org-policies enable-enforce iam.managed.disableServiceAccountKeyCreation --project={{.project}}
gcloud resource-manager org-policies enable-enforce storage.publicAccessPrevention --project={{.project}}
gcloud resource-manager org-policies enable-enforce storage.uniformBucketLevelAccess --project={{.project}}
gcloud resource-manager org-policies enable-enforce iam.allowedPolicyMemberDomains --project={{.project}}
gcloud resource-manager org-policies allow gcp.resourceLocations in:eu-locations --project={{.project}}
gcloud compute networks create atlas-vpc --subnet-mode=custom
gcloud compute networks subnets create app-subnet --network=atlas-vpc --range=10.20.1.0/24 --region={{.region}} --enable-private-ip-google-access
gcloud compute networks subnets create data-subnet --network=atlas-vpc --range=10.20.2.0/24 --region={{.region}} --enable-private-ip-google-access
gcloud compute routers create atlas-router --network=atlas-vpc --region={{.region}}
gcloud compute routers nats create atlas-nat --router=atlas-router --region={{.region}} --nat-all-subnet-ip-ranges --auto-allocate-nat-external-ips
gcloud compute firewall-rules create allow-iap-ssh --network=atlas-vpc --allow=tcp:22 --source-ranges=35.235.240.0/20
gcloud compute firewall-rules create allow-app-to-db --network=atlas-vpc --allow=tcp:5432 --source-tags=movimientos-api --target-tags=movimientos-db
gcloud iam service-accounts create movimientos-sa --display-name="API de movimientos"
gcloud compute instances create movimientos-db --zone={{.zone}} --machine-type=e2-medium --subnet=data-subnet --no-address --tags=movimientos-db --labels=app=movimientos,tier=db --private-network-ip=10.20.2.5 --metadata=sim-preinstalled=postgresql,startup-script='apt-get install -y postgresql'
gcloud compute instances create movimientos-api --zone={{.zone}} --machine-type=e2-small --subnet=app-subnet --no-address --tags=movimientos-api --labels=app=movimientos,tier=app --service-account=movimientos-sa@{{.project}}.iam.gserviceaccount.com --scopes=cloud-platform --metadata=startup-script='docker run -d -p 8080:8080 -e DB_HOST=10.20.2.5 -e DB_USER=app europe-docker.pkg.dev/gcplab-public/apps/checkout-api:1.4'
