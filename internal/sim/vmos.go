package sim

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Guest OS model (Blueprint §4, Linux block): a small, explicit model of what
// matters for troubleshooting — files with owner/group/mode, users and groups,
// disk usage and services that depend on readable configuration and free disk.
// It is attached lazily to a VM and serialised with it.

// VFile is a file or directory in the guest OS.
type VFile struct {
	Content string  `json:"content,omitempty"`
	Mode    int     `json:"mode"` // octal permissions, e.g. 0640
	Owner   string  `json:"owner"`
	Group   string  `json:"group"`
	SizeMB  float64 `json:"sizeMb,omitempty"`
	Dir     bool    `json:"dir,omitempty"`
}

// ServiceSpec links a systemd service to what it needs from the OS.
type ServiceSpec struct {
	User   string `json:"user"`             // account the service runs as
	Config string `json:"config,omitempty"` // file it must read at start-up
	Writes string `json:"writes,omitempty"` // directory it writes to (fails when the disk is full)
}

// VMOS is the guest OS state.
type VMOS struct {
	Files      map[string]*VFile       `json:"files"`
	Users      map[string][]string     `json:"users"` // user -> groups
	DiskGB     int                     `json:"diskGb"`
	BaseUsedGB float64                 `json:"baseUsedGb"` // OS + packages
	Services   map[string]*ServiceSpec `json:"services"`
}

// OS returns (creating) the guest OS of a VM.
func (vm *Instance) GuestOS() *VMOS {
	if vm.OS == nil {
		vm.OS = &VMOS{Files: map[string]*VFile{}, Users: map[string][]string{"root": {"root"}, "student": {"student", "google-sudoers"}}, DiskGB: 10, BaseUsedGB: 2.4, Services: map[string]*ServiceSpec{}}
	}
	if vm.OS.Services == nil {
		vm.OS.Services = map[string]*ServiceSpec{}
	}
	return vm.OS
}

// UsedGB is the used space of the root filesystem.
func (o *VMOS) UsedGB() float64 {
	used := o.BaseUsedGB
	for _, f := range o.Files {
		used += f.SizeMB / 1024
	}
	return used
}

// Full reports whether the root filesystem is full.
func (o *VMOS) Full() bool { return o.DiskGB > 0 && o.UsedGB() >= float64(o.DiskGB)*0.999 }

// InGroup reports group membership.
func (o *VMOS) InGroup(user, group string) bool {
	for _, g := range o.Users[user] {
		if g == group {
			return true
		}
	}
	return user == group
}

// CanRead evaluates POSIX read permission (root bypasses).
func (o *VMOS) CanRead(user, p string) bool {
	return o.can(user, p, 4)
}

// CanWrite evaluates POSIX write permission on a file or directory.
func (o *VMOS) CanWrite(user, p string) bool {
	return o.can(user, p, 2)
}

func (o *VMOS) can(user, p string, bit int) bool {
	if user == "root" {
		return true
	}
	f := o.Files[p]
	if f == nil {
		return false
	}
	switch {
	case f.Owner == user:
		return f.Mode&(bit<<6) != 0
	case o.InGroup(user, f.Group):
		return f.Mode&(bit<<3) != 0
	default:
		return f.Mode&bit != 0
	}
}

// ModeString renders -rw-r----- style permissions.
func (f *VFile) ModeString() string {
	var b strings.Builder
	if f.Dir {
		b.WriteByte('d')
	} else {
		b.WriteByte('-')
	}
	for i := 2; i >= 0; i-- {
		m := (f.Mode >> (3 * i)) & 7
		for j, c := range "rwx" {
			if m&(4>>j) != 0 {
				b.WriteRune(c)
			} else {
				b.WriteByte('-')
			}
		}
	}
	return b.String()
}

// Children lists files directly under a directory path.
func (o *VMOS) Children(dir string) []string {
	dir = strings.TrimSuffix(dir, "/")
	var out []string
	seen := map[string]bool{}
	for p := range o.Files {
		if !strings.HasPrefix(p, dir+"/") {
			continue
		}
		rest := strings.TrimPrefix(p, dir+"/")
		name := strings.SplitN(rest, "/", 2)[0]
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// DuMB sums sizes under a path.
func (o *VMOS) DuMB(p string) float64 {
	p = strings.TrimSuffix(p, "/")
	total := 0.0
	for fp, f := range o.Files {
		if fp == p || strings.HasPrefix(fp, p+"/") || p == "" {
			total += f.SizeMB
		}
	}
	return total
}

// serviceProblem explains why a service cannot run given the OS state.
func (o *VMOS) serviceProblem(name string) string {
	spec := o.Services[name]
	if spec == nil {
		return ""
	}
	if spec.Config != "" {
		if _, ok := o.Files[spec.Config]; !ok {
			return fmt.Sprintf("open %s: no such file or directory", spec.Config)
		}
		if !o.CanRead(spec.User, spec.Config) {
			return fmt.Sprintf("open %s: permission denied (running as %s)", spec.Config, spec.User)
		}
	}
	if spec.Writes != "" && o.Full() {
		return fmt.Sprintf("write %s: no space left on device", path.Join(spec.Writes, name+".log"))
	}
	return ""
}
