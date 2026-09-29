variable "org_id" {
  description = "Numeric organization id that will hold the sandbox folder."
  type        = string
}

variable "billing_account" {
  description = "Billing account for the sandbox projects (XXXXXX-XXXXXX-XXXXXX)."
  type        = string
}

variable "platform_project_id" {
  description = "Existing project that runs the platform (Cloud Run, Cloud SQL, secrets, registry)."
  type        = string
}

variable "region" {
  description = "Region for the platform and the only region labs may use."
  type        = string
  default     = "europe-west1"
}

variable "allowed_locations" {
  description = "Value group or locations allowed in sandbox projects (gcp.resourceLocations)."
  type        = list(string)
  default     = ["in:europe-west1-locations"]
}

variable "sandbox_count" {
  description = "Number of real-GCP sandbox projects in the F2 pool."
  type        = number
  default     = 5
}

variable "sandbox_prefix" {
  description = "Prefix for sandbox project ids (a random suffix is appended)."
  type        = string
  default     = "gcplab-sbx"
}

variable "sandbox_budget" {
  description = "Monthly budget per sandbox project, in the billing account currency."
  type        = number
  default     = 20
}

variable "budget_currency" {
  description = "Currency of the billing account."
  type        = string
  default     = "EUR"
}

variable "sandbox_apis" {
  description = "APIs enabled in every sandbox project."
  type        = list(string)
  default = [
    "compute.googleapis.com", "storage.googleapis.com", "iam.googleapis.com", "run.googleapis.com",
    "sqladmin.googleapis.com", "pubsub.googleapis.com", "logging.googleapis.com", "monitoring.googleapis.com",
    "cloudresourcemanager.googleapis.com", "artifactregistry.googleapis.com", "cloudbuild.googleapis.com",
    "bigquery.googleapis.com", "container.googleapis.com", "secretmanager.googleapis.com", "dns.googleapis.com",
  ]
}

variable "image" {
  description = "Container image built from deploy/Dockerfile (target runtime-gcloud for F2)."
  type        = string
}

variable "f2_monthly_sessions" {
  description = "Real-GCP sessions per learner per month."
  type        = number
  default     = 10
}

variable "mentor_llm" {
  description = "Enable the LLM post-mortem reviewer (requires an anthropic-api-key secret version)."
  type        = bool
  default     = false
}

variable "deny_vm_external_ip" {
  description = "Forbid external IPs on sandbox VMs (labs then expose services through load balancers or IAP)."
  type        = bool
  default     = true
}
