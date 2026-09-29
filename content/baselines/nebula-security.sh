# Nebula security-monitoring: audit archive bucket.
gcloud storage buckets create gs://audit-archive-{{.project}} --location=europe-west1 --uniform-bucket-level-access --public-access-prevention
