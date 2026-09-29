# Nebula prod-web extras: exports bucket and a deployer SA with a legacy basic role.
gcloud storage buckets create gs://exports-{{.project}} --location=europe-west1 --uniform-bucket-level-access
echo "order_id,total" > export.csv
gcloud storage cp export.csv gs://exports-{{.project}}/2026-08/orders.csv
gcloud iam service-accounts create web-deployer --display-name="legacy deployer"
gcloud projects add-iam-policy-binding {{.project}} --member=serviceAccount:web-deployer@{{.project}}.iam.gserviceaccount.com --role=roles/editor
