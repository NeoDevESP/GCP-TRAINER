# Security policy validator (OPA/Rego). Input: {"project": <project view>, "params": {...}}.
# Each message starts with a stable ID so labs can select the rules they grade.
package gcplab.security

import rego.v1

admin_ports := {"22", "3389", "5432", "3306", "6379", "9200"}

sensitive_rule(r) if r.IPProtocol == "all"

sensitive_rule(r) if {
	r.IPProtocol == "tcp"
	not r.ports
}

sensitive_rule(r) if {
	r.IPProtocol == "tcp"
	some p in r.ports
	p in admin_ports
}

sensitive_rule(r) if {
	r.IPProtocol == "tcp"
	some p in r.ports
	contains(p, "-")
	parts := split(p, "-")
	to_number(parts[0]) <= 22
	to_number(parts[1]) >= 22
}

deny contains msg if {
	some name, fw in input.project.firewalls
	fw.direction == "INGRESS"
	fw.action == "ALLOW"
	not fw.disabled
	"0.0.0.0/0" in fw.sourceRanges
	some r in fw.rules
	sensitive_rule(r)
	msg := sprintf("OPEN_ADMIN_PORT: firewall %s exposes administrative or database ports to 0.0.0.0/0", [name])
}

deny contains msg if {
	some name, b in input.project.buckets
	some bnd in b.iamPolicy.bindings
	some m in bnd.members
	m in {"allUsers", "allAuthenticatedUsers"}
	msg := sprintf("PUBLIC_BUCKET: bucket %s grants %s to %s", [name, bnd.role, m])
}

deny contains msg if {
	some bnd in input.project.iamPolicy.bindings
	bnd.role in {"roles/owner", "roles/editor"}
	some m in bnd.members
	startswith(m, "serviceAccount:")
	not endswith(m, "-compute@developer.gserviceaccount.com")
	msg := sprintf("BASIC_ROLE_SERVICE_ACCOUNT: %s has %s on the project", [m, bnd.role])
}

deny contains msg if {
	some bnd in input.project.iamPolicy.bindings
	bnd.role in {"roles/owner", "roles/editor"}
	some m in bnd.members
	startswith(m, "user:")
	some allowed in object.get(input, ["params", "noBasicRolesFor"], [])
	m == allowed
	msg := sprintf("BASIC_ROLE_USER: %s has %s on the project", [m, bnd.role])
}

deny contains msg if {
	some name, db in input.project.sqlInstances
	"0.0.0.0/0" in db.authorizedNetworks
	msg := sprintf("PUBLIC_SQL: Cloud SQL %s is reachable from 0.0.0.0/0", [name])
}

deny contains msg if {
	some name, svc in input.project.runServices
	some k, v in svc.env
	regex.match(`(?i)(password|secret|api_key|apikey|token|private_key)`, k)
	v != ""
	msg := sprintf("SECRET_IN_ENV: Cloud Run service %s stores %s in a plain environment variable", [name, k])
}

deny contains msg if {
	some email, sa in input.project.serviceAccounts
	count(sa.keys) > 0
	msg := sprintf("SA_USER_MANAGED_KEY: %s has user-managed keys", [email])
}

deny contains msg if {
	some name, vm in input.project.instances
	endswith(vm.serviceAccount, "-compute@developer.gserviceaccount.com")
	"cloud-platform" in vm.scopes
	msg := sprintf("DEFAULT_SA_FULL_SCOPE: VM %s uses the default compute SA with cloud-platform scope", [name])
}

deny contains msg if {
	some name, svc in input.project.runServices
	contains(name, "admin")
	some bnd in svc.iamPolicy.bindings
	bnd.role == "roles/run.invoker"
	"allUsers" in bnd.members
	msg := sprintf("PUBLIC_ADMIN_SERVICE: %s is invokable by allUsers", [name])
}

deny contains msg if {
	some name, vm in input.project.instances
	vm.gpus > 0
	msg := sprintf("GPU_INSTANCE: %s has %d GPUs attached", [name, vm.gpus])
}

deny contains msg if {
	some name, vm in input.project.instances
	vm.labels.tier == "db"
	vm.natIP != ""
	msg := sprintf("DB_PUBLIC_IP: database VM %s has an external IP", [name])
}

deny contains msg if {
	some bn in object.get(input, ["params", "cmekBuckets"], [])
	b := input.project.buckets[bn]
	object.get(b, "defaultKmsKey", "") == ""
	msg := sprintf("MISSING_CMEK: bucket %s is not encrypted with a customer-managed key", [bn])
}

deny contains msg if {
	some kr, ring in input.project.keyRings
	some kn, key in ring.keys
	key.purpose == "ENCRYPT_DECRYPT"
	object.get(key, "rotationPeriod", "") == ""
	msg := sprintf("KMS_NO_ROTATION: key %s/%s has no rotation period", [kr, kn])
}

deny contains msg if {
	some name, s in input.project.secrets
	some v in s.versions
	v.exposed
	v.state == "ENABLED"
	msg := sprintf("EXPOSED_SECRET_ENABLED: secret %s version %d is known to be exposed and still enabled", [name, v.id])
}

deny contains msg if {
	some name, b in input.project.buckets
	b.publicAccessPrevention != "enforced"
	name in object.get(input, ["params", "papBuckets"], [])
	msg := sprintf("PAP_NOT_ENFORCED: bucket %s does not enforce public access prevention", [name])
}
