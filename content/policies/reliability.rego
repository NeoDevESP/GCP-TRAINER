# Reliability validator (Well-Architected: reliability pillar).
package gcplab.reliability

import rego.v1

deny contains msg if {
	some name in object.get(input, ["params", "haDatabases"], [])
	db := input.project.sqlInstances[name]
	db.availabilityType != "REGIONAL"
	msg := sprintf("SQL_NOT_HA: %s is ZONAL", [name])
}

deny contains msg if {
	some name in object.get(input, ["params", "haDatabases"], [])
	db := input.project.sqlInstances[name]
	not db.backupEnabled
	msg := sprintf("SQL_NO_BACKUPS: %s has no automated backups", [name])
}

deny contains msg if {
	some name in object.get(input, ["params", "resilientGroups"], [])
	g := input.project.instanceGroups[name]
	g.targetSize < 2
	not g.autoscaler
	msg := sprintf("SINGLE_INSTANCE_GROUP: %s runs fewer than 2 instances", [name])
}

deny contains msg if {
	some name in object.get(input, ["params", "resilientGroups"], [])
	g := input.project.instanceGroups[name]
	not g.autoHealing
	msg := sprintf("NO_AUTOHEALING: %s has no autohealing health check", [name])
}
