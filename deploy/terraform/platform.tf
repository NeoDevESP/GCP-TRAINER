# Platform: Cloud Run services (API, lab plane, grader), Cloud SQL, secrets
# and a small VPC so internal services are not reachable from the internet.

locals {
  platform_apis = [
    "run.googleapis.com", "sqladmin.googleapis.com", "secretmanager.googleapis.com", "artifactregistry.googleapis.com",
    "cloudbuild.googleapis.com", "compute.googleapis.com", "iamcredentials.googleapis.com", "cloudresourcemanager.googleapis.com",
    "billingbudgets.googleapis.com", "orgpolicy.googleapis.com",
  ]
}

resource "google_project_service" "platform" {
  for_each           = toset(local.platform_apis)
  service            = each.value
  disable_on_destroy = false
}

resource "google_artifact_registry_repository" "images" {
  location      = var.region
  repository_id = "gcplab"
  format        = "DOCKER"
  depends_on    = [google_project_service.platform]
}

resource "google_compute_network" "platform" {
  name                    = "gcplab"
  auto_create_subnetworks = false
  depends_on              = [google_project_service.platform]
}

resource "google_compute_subnetwork" "run" {
  name                     = "gcplab-run"
  network                  = google_compute_network.platform.id
  region                   = var.region
  ip_cidr_range            = "10.8.0.0/24"
  private_ip_google_access = true
}

# ---- identities -------------------------------------------------------------

resource "google_service_account" "api" {
  account_id   = "gcplab-api"
  display_name = "gcplab learning plane"
}

resource "google_service_account" "labplane" {
  account_id   = "gcplab-labplane"
  display_name = "gcplab lab plane"
}

resource "google_service_account" "grader" {
  account_id   = "gcplab-grader"
  display_name = "gcplab grader worker"
}

resource "google_project_iam_member" "sql_client" {
  for_each = { api = google_service_account.api.email, labplane = google_service_account.labplane.email }
  project  = var.platform_project_id
  role     = "roles/cloudsql.client"
  member   = "serviceAccount:${each.value}"
}

# The lab plane acts as the pool admin (to manage sandboxes) and as the
# student identity (to run learner commands); nothing else can.
resource "google_service_account_iam_member" "labplane_as_pool_admin" {
  service_account_id = google_service_account.pool_admin.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:${google_service_account.labplane.email}"
}

resource "google_service_account_iam_member" "labplane_as_student" {
  service_account_id = google_service_account.student.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:${google_service_account.labplane.email}"
}

# ---- secrets ----------------------------------------------------------------

resource "random_password" "jwt" {
  length  = 48
  special = false
}

resource "random_password" "labplane_token" {
  length  = 48
  special = false
}

resource "random_password" "db" {
  length  = 32
  special = false
}

resource "google_secret_manager_secret" "s" {
  for_each  = toset(["jwt-secret", "labplane-token", "database-url", "anthropic-api-key"])
  secret_id = "gcplab-${each.value}"
  replication {
    auto {}
  }
  depends_on = [google_project_service.platform]
}

resource "google_secret_manager_secret_version" "jwt" {
  secret      = google_secret_manager_secret.s["jwt-secret"].id
  secret_data = random_password.jwt.result
}

resource "google_secret_manager_secret_version" "labplane_token" {
  secret      = google_secret_manager_secret.s["labplane-token"].id
  secret_data = random_password.labplane_token.result
}

resource "google_secret_manager_secret_version" "database_url" {
  secret      = google_secret_manager_secret.s["database-url"].id
  secret_data = "postgres://gcplab:${random_password.db.result}@/gcplab?host=/cloudsql/${google_sql_database_instance.db.connection_name}"
}

resource "google_secret_manager_secret_iam_member" "access" {
  for_each = {
    "api/jwt-secret"          = [google_service_account.api.email, "jwt-secret"]
    "api/labplane-token"      = [google_service_account.api.email, "labplane-token"]
    "api/database-url"        = [google_service_account.api.email, "database-url"]
    "api/anthropic-api-key"   = [google_service_account.api.email, "anthropic-api-key"]
    "labplane/labplane-token" = [google_service_account.labplane.email, "labplane-token"]
    "labplane/database-url"   = [google_service_account.labplane.email, "database-url"]
  }
  secret_id = google_secret_manager_secret.s[each.value[1]].id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${each.value[0]}"
}

# ---- database ---------------------------------------------------------------

resource "google_sql_database_instance" "db" {
  name                = "gcplab"
  database_version    = "POSTGRES_16"
  region              = var.region
  deletion_protection = true
  settings {
    edition           = "ENTERPRISE"
    tier              = "db-g1-small"
    availability_type = "ZONAL"
    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = true
    }
    ip_configuration {
      # reachable only through the Cloud SQL connector (IAM-authorised), no authorised networks
      ipv4_enabled = true
      ssl_mode     = "ENCRYPTED_ONLY"
    }
  }
  depends_on = [google_project_service.platform]
}

resource "google_sql_database" "gcplab" {
  name     = "gcplab"
  instance = google_sql_database_instance.db.name
}

resource "google_sql_user" "gcplab" {
  name     = "gcplab"
  instance = google_sql_database_instance.db.name
  password = random_password.db.result
}

# ---- services ---------------------------------------------------------------

locals {
  sandbox_ids = join(",", google_project.sandbox[*].project_id)
}

resource "google_cloud_run_v2_service" "grader" {
  name                = "gcplab-grader"
  location            = var.region
  ingress             = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  deletion_protection = false
  template {
    service_account = google_service_account.grader.email
    scaling {
      min_instance_count = 0
      max_instance_count = 5
    }
    containers {
      image   = var.image
      command = ["/usr/local/bin/grader-worker"]
      env {
        name  = "ADDR"
        value = ":8080"
      }
    }
  }
  depends_on = [google_project_service.platform]
}

# The lab plane keeps live simulator sessions in memory: exactly one instance.
resource "google_cloud_run_v2_service" "labplane" {
  name                = "gcplab-labplane"
  location            = var.region
  ingress             = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  deletion_protection = false
  template {
    service_account = google_service_account.labplane.email
    timeout         = "300s"
    scaling {
      min_instance_count = 1
      max_instance_count = 1
    }
    vpc_access {
      network_interfaces {
        network    = google_compute_network.platform.id
        subnetwork = google_compute_subnetwork.run.id
      }
      egress = "ALL_TRAFFIC"
    }
    volumes {
      name = "cloudsql"
      cloud_sql_instance {
        instances = [google_sql_database_instance.db.connection_name]
      }
    }
    containers {
      image = var.image
      resources {
        limits   = { cpu = "2", memory = "2Gi" }
        cpu_idle = false
      }
      volume_mounts {
        name       = "cloudsql"
        mount_path = "/cloudsql"
      }
      dynamic "env" {
        for_each = {
          MODE        = "labplane"
          ADDR        = ":8080"
          GRADER_URL  = google_cloud_run_v2_service.grader.uri
          F2_PROJECTS = local.sandbox_ids
          F2_DRIVER   = "gcloud"
          F2_ADMIN_SA = google_service_account.pool_admin.email
          F2_LAB_SA   = google_service_account.student.email
        }
        content {
          name  = env.key
          value = env.value
        }
      }
      dynamic "env" {
        for_each = { LABPLANE_TOKEN = "labplane-token", DATABASE_URL = "database-url" }
        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.s[env.value].secret_id
              version = "latest"
            }
          }
        }
      }
    }
  }
  depends_on = [google_secret_manager_secret_iam_member.access, google_secret_manager_secret_version.labplane_token, google_secret_manager_secret_version.database_url]
}

resource "google_cloud_run_v2_service" "api" {
  name                = "gcplab-api"
  location            = var.region
  ingress             = "INGRESS_TRAFFIC_ALL"
  deletion_protection = false
  template {
    service_account = google_service_account.api.email
    scaling {
      min_instance_count = 0
      max_instance_count = 10
    }
    vpc_access {
      network_interfaces {
        network    = google_compute_network.platform.id
        subnetwork = google_compute_subnetwork.run.id
      }
      egress = "ALL_TRAFFIC"
    }
    volumes {
      name = "cloudsql"
      cloud_sql_instance {
        instances = [google_sql_database_instance.db.connection_name]
      }
    }
    containers {
      image = var.image
      volume_mounts {
        name       = "cloudsql"
        mount_path = "/cloudsql"
      }
      dynamic "env" {
        for_each = {
          MODE         = "api"
          ADDR         = ":8080"
          LABPLANE_URL = google_cloud_run_v2_service.labplane.uri
          F2_MONTHLY   = tostring(var.f2_monthly_sessions)
          MENTOR_LLM   = var.mentor_llm ? "on" : "off"
        }
        content {
          name  = env.key
          value = env.value
        }
      }
      dynamic "env" {
        for_each = merge(
          { JWT_SECRET = "jwt-secret", LABPLANE_TOKEN = "labplane-token", DATABASE_URL = "database-url" },
          var.mentor_llm ? { ANTHROPIC_API_KEY = "anthropic-api-key" } : {},
        )
        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.s[env.value].secret_id
              version = "latest"
            }
          }
        }
      }
    }
  }
  depends_on = [google_secret_manager_secret_iam_member.access, google_secret_manager_secret_version.jwt]
}

# Public entry point. The internal services accept only VPC traffic and, on
# top of that, the lab plane requires the shared LABPLANE_TOKEN.
resource "google_cloud_run_v2_service_iam_member" "public" {
  for_each = {
    api      = google_cloud_run_v2_service.api.name
    labplane = google_cloud_run_v2_service.labplane.name
    grader   = google_cloud_run_v2_service.grader.name
  }
  name     = each.value
  location = var.region
  role     = "roles/run.invoker"
  member   = "allUsers"
}
