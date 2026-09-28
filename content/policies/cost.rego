# Cost and hygiene validator.
package gcplab.cost

import rego.v1

deny contains msg if {
	limit := input.params.maxMonthlyEur
	input.project.cost.monthlyEur > limit
	msg := sprintf("OVER_BUDGET: estimated %.2f EUR/month exceeds %.2f", [input.project.cost.monthlyEur, limit])
}

deny contains msg if {
	some name, a in input.project.addresses
	a.addressType == "EXTERNAL"
	object.get(a, "user", "") == ""
	msg := sprintf("IDLE_STATIC_IP: address %s is reserved but unused", [name])
}

deny contains msg if {
	some name, vm in input.project.instances
	regex.match(`^(n2|c2|e2)-standard-(16|32|60|64)$`, vm.machineType)
	msg := sprintf("OVERSIZED_VM: %s uses %s", [name, vm.machineType])
}

deny contains msg if {
	some name, d in input.project.disks
	count(d.users) == 0
	not input.project.instances[name]
	msg := sprintf("UNATTACHED_DISK: disk %s is not attached", [name])
}

deny contains msg if {
	some name in object.get(input, ["params", "mustBeDeleted"], [])
	input.project.instances[name]
	msg := sprintf("LEFTOVER_RESOURCE: temporary instance %s was not cleaned up", [name])
}
