package cli

import (
	"strings"
	"testing"
)

func TestDiskGrowNeedsGuestResize(t *testing.T) {
	s := newTestSession()
	run(t, s, "gcloud compute networks create vpc --subnet-mode=custom")
	run(t, s, "gcloud compute networks subnets create apps --network=vpc --range=10.1.0.0/24 --region=europe-west1")
	run(t, s, "gcloud compute instances create db --zone=europe-west1-b --machine-type=e2-small --subnet=apps --no-address")
	vm := s.State.Projects["lab-proj"].Instances["db"]
	o := vm.GuestOS()
	o.DiskGB = 20
	if out := run(t, s, "gcloud compute disks resize db --zone=europe-west1-b --size=50 --quiet"); !strings.Contains(out, "growpart") {
		t.Fatalf("resize should warn about the guest: %s", out)
	}
	if o.DiskGB != 20 || o.PendingGB != 50 {
		t.Fatalf("guest should still see 20 GB, got %d (pending %d)", o.DiskGB, o.PendingGB)
	}
	s.vmRoot = true
	if _, err := s.vmOSCommand(vm, []string{"resize2fs", "/dev/sda1"}); err == nil {
		t.Fatal("resize2fs before growpart should not grow")
	}
	if _, err := s.vmOSCommand(vm, []string{"growpart", "/dev/sda", "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.vmOSCommand(vm, []string{"resize2fs", "/dev/sda1"}); err != nil || o.DiskGB != 50 {
		t.Fatalf("resize2fs should grow to 50 GB: %v %d", err, o.DiskGB)
	}
	// a reboot also applies a pending growth
	run(t, s, "gcloud compute disks resize db --zone=europe-west1-b --size=80 --quiet")
	run(t, s, "gcloud compute instances reset db --zone=europe-west1-b")
	if o.DiskGB != 80 {
		t.Fatalf("reboot should grow the file system, got %d", o.DiskGB)
	}
}

func TestRetentionLockSnapshotRestoreAndZoneDown(t *testing.T) {
	s := newTestSession()
	run(t, s, "gcloud storage buckets create gs://regs --location=EU")
	if out := run(t, s, "gcloud storage buckets update gs://regs --lock-retention-period"); !strings.Contains(out, "no retention policy") {
		t.Fatalf("locking without a policy should fail: %s", out)
	}
	run(t, s, "gcloud storage buckets update gs://regs --retention-period=10y")
	run(t, s, "gcloud storage buckets update gs://regs --lock-retention-period")
	if out := run(t, s, "gcloud storage buckets update gs://regs --retention-period=1d"); !strings.Contains(out, "locked") {
		t.Fatalf("a locked policy can't be shortened: %s", out)
	}
	if out := run(t, s, "gcloud storage buckets update gs://regs --clear-retention-period"); !strings.Contains(out, "locked") {
		t.Fatalf("a locked policy can't be removed: %s", out)
	}

	run(t, s, "gcloud compute networks create vpc --subnet-mode=custom")
	run(t, s, "gcloud compute networks subnets create apps --network=vpc --range=10.1.0.0/24 --region=europe-west1")
	run(t, s, "gcloud compute instances create batch --zone=europe-west1-b --machine-type=e2-small --subnet=apps --no-address")
	run(t, s, "gcloud compute disks snapshot batch --zone=europe-west1-b --snapshot-names=batch-snap")
	s.State.Extra["outage:europe-west1-b"] = "down"
	if out := run(t, s, "gcloud compute instances create batch2 --zone=europe-west1-b --machine-type=e2-small --subnet=apps --no-address"); !strings.Contains(out, "unavailable") {
		t.Fatalf("creating in a downed zone should fail: %s", out)
	}
	run(t, s, "gcloud compute instances create batch-c --zone=europe-west1-c --source-snapshot=batch-snap --machine-type=e2-small --subnet=apps --no-address")
	if d := s.State.Projects["lab-proj"].Disks["batch-c"]; d == nil || d.Snapshot != "batch-snap" {
		t.Fatal("boot disk should come from the snapshot")
	}
}
