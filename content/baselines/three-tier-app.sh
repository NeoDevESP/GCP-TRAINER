# Baseline: three-tier checkout application behind a global HTTP load balancer.
# web/app/db subnets in prod-vpc, Cloud NAT, API VM running checkout-api (docker),
# PostgreSQL VM, health-check and IAP firewall rules.
gcloud compute networks create prod-vpc --subnet-mode=custom
gcloud compute networks subnets create web-subnet --network=prod-vpc --range=10.10.1.0/24 --region={{.region}}
gcloud compute networks subnets create app-subnet --network=prod-vpc --range=10.10.2.0/24 --region={{.region}}
gcloud compute networks subnets create db-subnet --network=prod-vpc --range=10.10.3.0/24 --region={{.region}}
gcloud compute routers create prod-router --network=prod-vpc --region={{.region}}
gcloud compute routers nats create prod-nat --router=prod-router --region={{.region}} --nat-all-subnet-ip-ranges --auto-allocate-nat-external-ips
gcloud compute firewall-rules create allow-health --network=prod-vpc --allow=tcp:8080 --source-ranges=35.191.0.0/16,130.211.0.0/22 --target-tags=api
gcloud compute firewall-rules create allow-iap-ssh --network=prod-vpc --allow=tcp:22 --source-ranges=35.235.240.0/20
gcloud compute firewall-rules create allow-api-to-db --network=prod-vpc --allow=tcp:5432 --source-tags=api --target-tags=database
gcloud iam service-accounts create api-sa --display-name="checkout api"
gcloud compute instances create sql-1 --zone={{.zone}} --machine-type=e2-medium --subnet=db-subnet --no-address --tags=database --labels=tier=db --private-network-ip=10.10.3.5 --metadata=sim-preinstalled=postgresql,startup-script='apt-get install -y postgresql'
gcloud compute instances create api-1 --zone={{.zone}} --machine-type=e2-small --subnet=app-subnet --no-address --tags=api --service-account=api-sa@{{.project}}.iam.gserviceaccount.com --scopes=cloud-platform --metadata=startup-script='docker run -d -p 8080:8080 -e DB_HOST=10.10.3.5 -e DB_USER=app europe-docker.pkg.dev/gcplab-public/apps/checkout-api:1.4'
gcloud compute instance-groups unmanaged create api-ig --zone={{.zone}}
gcloud compute instance-groups unmanaged add-instances api-ig --instances=api-1 --zone={{.zone}}
gcloud compute instance-groups set-named-ports api-ig --named-ports=http:8080 --zone={{.zone}}
gcloud compute health-checks create http api-hc --port=8080 --request-path=/healthz
gcloud compute backend-services create checkout-be --protocol=HTTP --port-name=http --health-checks=api-hc --global
gcloud compute backend-services add-backend checkout-be --instance-group=api-ig --instance-group-zone={{.zone}} --global
gcloud compute url-maps create checkout-map --default-service=checkout-be
gcloud compute target-http-proxies create checkout-proxy --url-map=checkout-map
gcloud compute forwarding-rules create checkout-lb --global --target-http-proxy=checkout-proxy --ports=80
