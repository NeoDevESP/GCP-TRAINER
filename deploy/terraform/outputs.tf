output "url" {
  description = "Public URL of the platform."
  value       = google_cloud_run_v2_service.api.uri
}

output "sandbox_projects" {
  description = "F2 pool projects."
  value       = google_project.sandbox[*].project_id
}

output "sandbox_folder" {
  value = google_folder.labs.name
}

output "image_repository" {
  value = "${var.region}-docker.pkg.dev/${var.platform_project_id}/${google_artifact_registry_repository.images.repository_id}"
}
