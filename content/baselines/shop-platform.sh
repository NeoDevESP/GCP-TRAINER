# Baseline: serverless shop platform.
# Cloud SQL shop-db (db-g1-small, max 50 connections), Cloud Run checkout and
# stock-api using the connector with dedicated service accounts, exports bucket
# and a legacy batch VM owned by another team.
gcloud iam service-accounts create checkout-sa --display-name="checkout"
gcloud iam service-accounts create stock-sa --display-name="stock"
gcloud projects add-iam-policy-binding {{.project}} --member=serviceAccount:checkout-sa@{{.project}}.iam.gserviceaccount.com --role=roles/cloudsql.client
gcloud projects add-iam-policy-binding {{.project}} --member=serviceAccount:stock-sa@{{.project}}.iam.gserviceaccount.com --role=roles/cloudsql.client
gcloud sql instances create shop-db --database-version=POSTGRES_15 --tier=db-g1-small --region={{.region}} --backup-start-time=02:00
gcloud sql databases create shop --instance=shop-db
gcloud sql users create app --instance=shop-db --password=pw-shop-1
echo -n "pw-shop-1" | gcloud secrets create shop-db-password --data-file=-
gcloud secrets add-iam-policy-binding shop-db-password --member=serviceAccount:checkout-sa@{{.project}}.iam.gserviceaccount.com --role=roles/secretmanager.secretAccessor
gcloud secrets add-iam-policy-binding shop-db-password --member=serviceAccount:stock-sa@{{.project}}.iam.gserviceaccount.com --role=roles/secretmanager.secretAccessor
gcloud run deploy checkout --image=europe-docker.pkg.dev/gcplab-public/apps/checkout-api:1.4 --region={{.region}} --service-account=checkout-sa@{{.project}}.iam.gserviceaccount.com --add-cloudsql-instances={{.project}}:{{.region}}:shop-db --set-env-vars=DB_HOST=/cloudsql/{{.project}}:{{.region}}:shop-db,DB_USER=app,DB_NAME=shop,DB_POOL_SIZE=5 --set-secrets=DB_PASSWORD=shop-db-password:latest --max-instances=4 --allow-unauthenticated
gcloud run deploy stock-api --image=europe-docker.pkg.dev/gcplab-public/apps/inventory-api:1.0 --region={{.region}} --service-account=stock-sa@{{.project}}.iam.gserviceaccount.com --add-cloudsql-instances={{.project}}:{{.region}}:shop-db --set-env-vars=DB_HOST=/cloudsql/{{.project}}:{{.region}}:shop-db,DB_USER=app,DB_NAME=shop,DB_POOL_SIZE=5 --set-secrets=DB_PASSWORD=shop-db-password:latest --max-instances=4 --allow-unauthenticated
gcloud storage buckets create gs://exports-{{.project}} --location={{.region}} --uniform-bucket-level-access
echo "order_id,total" > export.csv
gcloud storage cp export.csv gs://exports-{{.project}}/2026-09/orders.csv
gcloud compute instances create legacy-batch --zone={{.zone}} --machine-type=e2-standard-4 --image-family=debian-12 --image-project=debian-cloud --labels=owner=finance,purpose=quarter-close
