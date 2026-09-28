package sim

import (
	"fmt"
	"strings"
)

// Finding is a simulated Security Command Center finding.
type Finding struct {
	Category string `json:"category"`
	Severity string `json:"severity"`
	Resource string `json:"resourceName"`
	Detail   string `json:"description"`
	State    string `json:"state"`
}

var sensitiveEnv = []string{"PASSWORD", "SECRET", "API_KEY", "TOKEN", "PRIVATE_KEY"}

// Findings derives SCC-style findings from current configuration.
func (s *State) Findings(project string) []Finding {
	p := s.Projects[project]
	if p == nil {
		return nil
	}
	var out []Finding
	add := func(cat, sev, res, d string) {
		out = append(out, Finding{Category: cat, Severity: sev, Resource: res, Detail: d, State: "ACTIVE"})
	}
	for _, n := range SortedKeys(p.Buckets) {
		b := p.Buckets[n]
		if b.IAM.HasMember("roles/storage.objectViewer", "allUsers") || b.IAM.HasMember("roles/storage.legacyObjectReader", "allUsers") || b.IAM.HasMember("roles/storage.objectViewer", "allAuthenticatedUsers") {
			add("PUBLIC_BUCKET_ACL", "HIGH", "//storage.googleapis.com/"+n, "Bucket is readable by allUsers/allAuthenticatedUsers")
		}
		if !b.UBLA {
			add("BUCKET_POLICY_ONLY_DISABLED", "MEDIUM", "//storage.googleapis.com/"+n, "Uniform bucket-level access is disabled")
		}
	}
	for _, n := range SortedKeys(p.Firewalls) {
		fw := p.Firewalls[n]
		if fw.Direction != "INGRESS" || fw.Action != "ALLOW" || fw.Disabled {
			continue
		}
		open := false
		for _, r := range fw.SourceRanges {
			if r == "0.0.0.0/0" {
				open = true
			}
		}
		if !open {
			continue
		}
		for _, port := range []int{22, 3389, 5432, 3306} {
			if portMatches(fw.Rules, "tcp", port) {
				cat := "OPEN_FIREWALL"
				switch port {
				case 22:
					cat = "OPEN_SSH_PORT"
				case 3389:
					cat = "OPEN_RDP_PORT"
				case 5432, 3306:
					cat = "OPEN_DATABASE_PORT"
				}
				add(cat, "HIGH", "//compute.googleapis.com/projects/"+project+"/global/firewalls/"+n, fmt.Sprintf("Firewall rule allows 0.0.0.0/0 on tcp:%d", port))
			}
		}
		for _, r := range fw.Rules {
			if strings.EqualFold(r.Protocol, "all") {
				add("OPEN_FIREWALL", "HIGH", "//compute.googleapis.com/projects/"+project+"/global/firewalls/"+n, "Firewall rule allows all protocols from 0.0.0.0/0")
			}
		}
	}
	for _, b := range p.IAM.Bindings {
		if b.Role != "roles/owner" && b.Role != "roles/editor" {
			continue
		}
		for _, m := range b.Members {
			if strings.HasPrefix(m, "serviceAccount:") {
				if strings.Contains(m, "-compute@developer.gserviceaccount.com") {
					add("DEFAULT_SERVICE_ACCOUNT_USED", "MEDIUM", "//cloudresourcemanager.googleapis.com/projects/"+project, "Default compute service account has "+b.Role)
				} else {
					add("OVER_PRIVILEGED_SERVICE_ACCOUNT_USER", "HIGH", "//cloudresourcemanager.googleapis.com/projects/"+project, m+" has basic role "+b.Role)
				}
			}
		}
	}
	for _, n := range SortedKeys(p.ServiceAccounts) {
		sa := p.ServiceAccounts[n]
		for _, k := range sa.Keys {
			sev := "MEDIUM"
			cat := "USER_MANAGED_SERVICE_ACCOUNT_KEY"
			if k.Leaked {
				sev, cat = "CRITICAL", "LEAKED_SERVICE_ACCOUNT_KEY"
			}
			add(cat, sev, "//iam.googleapis.com/projects/"+project+"/serviceAccounts/"+n+"/keys/"+k.ID, "User-managed key exists")
		}
	}
	for _, n := range SortedKeys(p.SQLInstances) {
		in := p.SQLInstances[n]
		for _, an := range in.AuthorizedNets {
			if an == "0.0.0.0/0" {
				add("PUBLIC_SQL_INSTANCE", "HIGH", "//cloudsql.googleapis.com/projects/"+project+"/instances/"+n, "Cloud SQL instance accepts connections from 0.0.0.0/0")
			}
		}
		if !in.BackupsEnabled {
			add("AUTO_BACKUP_DISABLED", "MEDIUM", "//cloudsql.googleapis.com/projects/"+project+"/instances/"+n, "Automated backups are disabled")
		}
	}
	for _, n := range SortedKeys(p.RunServices) {
		svc := p.RunServices[n]
		for k, v := range svc.Env {
			for _, sen := range sensitiveEnv {
				if strings.Contains(strings.ToUpper(k), sen) && v != "" {
					add("SECRET_IN_ENVIRONMENT_VARIABLE", "HIGH", "//run.googleapis.com/projects/"+project+"/locations/"+svc.Region+"/services/"+n, "Plain-text secret in environment variable "+k)
				}
			}
		}
		if svc.IAM.HasMember("roles/run.invoker", "allUsers") && (strings.Contains(n, "admin") || strings.Contains(n, "internal")) {
			add("PUBLIC_SENSITIVE_SERVICE", "HIGH", "//run.googleapis.com/projects/"+project+"/locations/"+svc.Region+"/services/"+n, "Sensitive service is publicly invokable")
		}
	}
	for _, n := range SortedKeys(p.Instances) {
		vm := p.Instances[n]
		if vm.GPUs > 0 && vm.CreatedBy != "" && !strings.HasPrefix(vm.CreatedBy, "user:") {
			add("CRYPTOMINING_SUSPECTED", "CRITICAL", "//compute.googleapis.com/projects/"+project+"/zones/"+vm.Zone+"/instances/"+n, "GPU instance created by "+vm.CreatedBy+" with high CPU usage and connections to mining pools")
		}
		if vm.ExternalIP != "" && vm.Labels["tier"] == "db" {
			add("PUBLIC_IP_ADDRESS", "HIGH", "//compute.googleapis.com/projects/"+project+"/zones/"+vm.Zone+"/instances/"+n, "Database VM has a public IP")
		}
	}
	for _, kr := range SortedKeys(p.KeyRings) {
		for _, kn := range SortedKeys(p.KeyRings[kr].Keys) {
			k := p.KeyRings[kr].Keys[kn]
			if k.RotationPeriod == "" && k.Purpose == "ENCRYPT_DECRYPT" {
				add("KMS_KEY_NOT_ROTATED", "MEDIUM", "//cloudkms.googleapis.com/projects/"+project+"/locations/"+p.KeyRings[kr].Location+"/keyRings/"+kr+"/cryptoKeys/"+kn, "Key has no rotation period")
			}
		}
	}
	for _, sn := range SortedKeys(p.Secrets) {
		for _, v := range p.Secrets[sn].Versions {
			if v.Exposed && v.State == "ENABLED" {
				add("EXPOSED_SECRET_VERSION_ENABLED", "HIGH", "//secretmanager.googleapis.com/projects/"+project+"/secrets/"+sn+fmt.Sprintf("/versions/%d", v.ID), "A secret version known to be exposed is still enabled")
			}
		}
	}
	return out
}
