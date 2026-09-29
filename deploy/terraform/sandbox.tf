# F2: a folder of disposable sandbox projects with guardrails. Learners never
# get standing access: the lab plane grants a time-boxed binding per session
# and the janitor destroys everything afterwards.

resource "google_folder" "labs" {
  display_name = "gcplab-sandboxes"
  parent       = "organizations/${var.org_id}"
}

locals {
  folder = google_folder.labs.name # folders/123
  boolean_constraints = [
    "iam.disableServiceAccountKeyCreation",
    "iam.disableServiceAccountKeyUpload",
    "storage.publicAccessPrevention",
    "storage.uniformBucketLevelAccess",
    "compute.requireOsLogin",
    "sql.restrictPublicIp",
  ]
}

resource "google_org_policy_policy" "boolean" {
  for_each = toset(local.boolean_constraints)
  name     = "${local.folder}/policies/${each.value}"
  parent   = local.folder
  spec {
    rules {
      enforce = "TRUE"
    }
  }
}

resource "google_org_policy_policy" "locations" {
  name   = "${local.folder}/policies/gcp.resourceLocations"
  parent = local.folder
  spec {
    rules {
      values {
        allowed_values = var.allowed_locations
      }
    }
  }
}

resource "google_org_policy_policy" "no_external_ip" {
  count  = var.deny_vm_external_ip ? 1 : 0
  name   = "${local.folder}/policies/compute.vmExternalIpAccess"
  parent = local.folder
  spec {
    rules {
      deny_all = "TRUE"
    }
  }
}

# Only the platform's own identities may be granted roles in sandboxes.
resource "google_org_policy_policy" "allowed_members" {
  name   = "${local.folder}/policies/iam.allowedPolicyMemberDomains"
  parent = local.folder
  spec {
    rules {
      values {
        allowed_values = [data.google_organization.org.directory_customer_id]
      }
    }
  }
}

data "google_organization" "org" {
  organization = "organizations/${var.org_id}"
}

resource "random_id" "sandbox" {
  count       = var.sandbox_count
  byte_length = 3
}

resource "google_project" "sandbox" {
  count           = var.sandbox_count
  name            = "gcplab sandbox ${count.index + 1}"
  project_id      = "${var.sandbox_prefix}-${random_id.sandbox[count.index].hex}"
  folder_id       = local.folder
  billing_account = var.billing_account
  deletion_policy = "DELETE"
  labels = {
    purpose = "training-lab"
    managed = "gcplab"
  }
  depends_on = [google_org_policy_policy.boolean, google_org_policy_policy.locations]
}

resource "google_project_service" "sandbox" {
  for_each = {
    for pair in setproduct(range(var.sandbox_count), var.sandbox_apis) : "${pair[0]}/${pair[1]}" => {
      project = google_project.sandbox[pair[0]].project_id
      service = pair[1]
    }
  }
  project            = each.value.project
  service            = each.value.service
  disable_on_destroy = false
}

resource "google_billing_budget" "sandbox" {
  count           = var.sandbox_count
  billing_account = var.billing_account
  display_name    = "gcplab ${google_project.sandbox[count.index].project_id}"
  budget_filter {
    projects = ["projects/${google_project.sandbox[count.index].number}"]
  }
  amount {
    specified_amount {
      currency_code = var.budget_currency
      units         = tostring(var.sandbox_budget)
    }
  }
  threshold_rules {
    threshold_percent = 0.5
  }
  threshold_rules {
    threshold_percent = 0.9
  }
  threshold_rules {
    threshold_percent = 1.0
    spend_basis       = "FORECASTED_SPEND"
  }
}

# Pool admin: prepares, resets and destroys sandbox projects. It is only ever
# used through impersonation by the lab plane.
resource "google_service_account" "pool_admin" {
  account_id   = "gcplab-pool-admin"
  display_name = "gcplab F2 pool administrator"
}

resource "google_folder_iam_member" "pool_admin" {
  for_each = toset([
    "roles/editor",
    "roles/resourcemanager.projectIamAdmin",
    "roles/serviceusage.serviceUsageAdmin",
    "roles/iam.serviceAccountAdmin",
  ])
  folder = local.folder
  role   = each.value
  member = "serviceAccount:${google_service_account.pool_admin.email}"
}

# Student identity: holds no standing roles; the lab plane binds it to one
# sandbox project for the duration of a session.
resource "google_service_account" "student" {
  account_id   = "gcplab-student"
  display_name = "gcplab learner session identity"
}
