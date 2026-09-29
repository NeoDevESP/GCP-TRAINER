# Nebula prod-data: orders database (zonal, no backups yet), data lake bucket,
# analytics dataset and an ETL service account with a legacy basic role.
gcloud sql instances create orders-db --database-version=POSTGRES_15 --tier=db-custom-2-7680 --region=europe-west1 --availability-type=ZONAL --no-backup
gcloud sql databases create orders --instance=orders-db
gcloud storage buckets create gs://datalake-{{.project}} --location=europe-west1 --uniform-bucket-level-access
bq mk --dataset --location=EU analytics
bq mk --table --sim_rows=50000000 analytics.orders order_id:STRING:16,amount:NUMERIC:16,created:TIMESTAMP:8,country:STRING:4
gcloud iam service-accounts create etl-sa --display-name="etl"
gcloud projects add-iam-policy-binding {{.project}} --member=serviceAccount:etl-sa@{{.project}}.iam.gserviceaccount.com --role=roles/editor
