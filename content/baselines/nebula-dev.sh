# Nebula dev-web: a developer VM and a scratch bucket.
gcloud compute instances create dev-web-1 --zone=europe-west1-b --machine-type=e2-small --image-family=debian-12 --image-project=debian-cloud --labels=env=dev
gcloud storage buckets create gs://dev-assets-{{.project}} --location=europe-west1 --uniform-bucket-level-access
