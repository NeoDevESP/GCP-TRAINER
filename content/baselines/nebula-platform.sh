# Nebula prod-platform: container registry and event bus.
gcloud artifacts repositories create images --repository-format=docker --location=europe-west1
gcloud pubsub topics create events
