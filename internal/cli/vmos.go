package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

// vmOSCommand implements Linux commands against the guest OS model.
func (s *Session) vmOSCommand(vm *sim.Instance, toks []string) (string, error) {
	o := vm.GuestOS()
	user := "student"
	if s.vmRoot {
		user = "root"
	}
	args := toks[1:]
	var flags, paths []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			flags = append(flags, a)
		} else {
			paths = append(paths, a)
		}
	}
	has := func(f string) bool {
		for _, x := range flags {
			if strings.Contains(x, f) {
				return true
			}
		}
		return false
	}
	needRoot := func(p string) error {
		if user == "root" {
			return nil
		}
		if f := o.Files[p]; f != nil && f.Owner == user {
			return nil
		}
		return fail(1, "%s: changing permissions of '%s': Operation not permitted", toks[0], p)
	}
	switch toks[0] {
	case "id", "groups":
		u := user
		if len(paths) > 0 {
			u = paths[0]
		}
		gs, ok := o.Users[u]
		if !ok {
			return "", fail(1, "id: '%s': no such user", u)
		}
		if toks[0] == "groups" {
			return u + " : " + strings.Join(gs, " ") + "\n", nil
		}
		return fmt.Sprintf("uid=%d(%s) gid=%d(%s) groups=%s\n", 1000+len(u), u, 1000+len(u), u, strings.Join(gs, ",")), nil
	case "usermod":
		// usermod -aG GROUP USER
		if user != "root" {
			return "", fail(1, "usermod: Permission denied.")
		}
		if len(paths) < 2 {
			return "", fail(2, "usage: usermod -aG GROUP USER")
		}
		o.Users[paths[1]] = append(o.Users[paths[1]], paths[0])
		return "", nil
	case "ls":
		p := "/"
		if len(paths) > 0 {
			p = paths[0]
		}
		p = strings.TrimSuffix(p, "/")
		if p == "" {
			p = "/"
		}
		var names []string
		if f := o.Files[p]; f != nil && !f.Dir {
			names = []string{p}
		} else {
			for _, c := range o.Children(p) {
				names = append(names, strings.TrimSuffix(p, "/")+"/"+c)
			}
			if len(names) == 0 && o.Files[p] == nil {
				return "", fail(2, "ls: cannot access '%s': No such file or directory", p)
			}
		}
		var b strings.Builder
		for _, n := range names {
			f := o.Files[n]
			base := n[strings.LastIndex(n, "/")+1:]
			if !has("l") {
				b.WriteString(base + "\n")
				continue
			}
			if f == nil {
				fmt.Fprintf(&b, "drwxr-xr-x 2 root root %8s %s\n", "4.0K", base)
				continue
			}
			fmt.Fprintf(&b, "%s 1 %-8s %-8s %8s %s\n", f.ModeString(), f.Owner, f.Group, human(f.SizeMB), base)
		}
		return b.String(), nil
	case "stat":
		if len(paths) == 0 {
			return "", fail(1, "stat: missing operand")
		}
		f := o.Files[paths[0]]
		if f == nil {
			return "", fail(1, "stat: cannot statx '%s': No such file or directory", paths[0])
		}
		return fmt.Sprintf("  File: %s\n  Size: %s\nAccess: (%04o/%s)  Uid: (%s)   Gid: (%s)\n", paths[0], human(f.SizeMB), f.Mode, f.ModeString(), f.Owner, f.Group), nil
	case "chmod":
		if len(paths) < 2 {
			return "", fail(1, "chmod: missing operand")
		}
		mode, err := strconv.ParseInt(paths[0], 8, 32)
		if err != nil {
			return "", fail(1, "chmod: invalid mode: '%s' (use octal, e.g. 640)", paths[0])
		}
		for _, p := range paths[1:] {
			f := o.Files[p]
			if f == nil {
				return "", fail(1, "chmod: cannot access '%s': No such file or directory", p)
			}
			if err := needRoot(p); err != nil {
				return "", err
			}
			f.Mode = int(mode)
		}
		return "", nil
	case "chown", "chgrp":
		if len(paths) < 2 {
			return "", fail(1, "%s: missing operand", toks[0])
		}
		if user != "root" {
			return "", fail(1, "%s: changing ownership of '%s': Operation not permitted", toks[0], paths[1])
		}
		owner, group, _ := strings.Cut(paths[0], ":")
		for _, p := range paths[1:] {
			f := o.Files[p]
			if f == nil {
				return "", fail(1, "%s: cannot access '%s': No such file or directory", toks[0], p)
			}
			if toks[0] == "chgrp" {
				f.Group = paths[0]
				continue
			}
			if owner != "" {
				f.Owner = owner
			}
			if group != "" {
				f.Group = group
			}
		}
		return "", nil
	case "df":
		used := o.UsedGB()
		pct := int(100 * used / float64(max(1, o.DiskGB)))
		if pct > 100 {
			pct = 100
		}
		return fmt.Sprintf("Filesystem      Size  Used Avail Use%% Mounted on\n/dev/sda1       %3dG  %4.1fG %4.1fG %3d%% /\n", o.DiskGB, used, maxf(0, float64(o.DiskGB)-used), pct), nil
	case "du":
		if len(paths) == 0 {
			paths = []string{"/"}
		}
		var b strings.Builder
		for _, p := range paths {
			if strings.HasSuffix(p, "/*") {
				dir := strings.TrimSuffix(p, "/*")
				for _, c := range o.Children(dir) {
					fp := dir + "/" + c
					fmt.Fprintf(&b, "%s\t%s\n", human(o.DuMB(fp)), fp)
				}
				continue
			}
			fmt.Fprintf(&b, "%s\t%s\n", human(o.DuMB(p)), p)
		}
		return b.String(), nil
	case "find":
		// find PATH -size +NM|+NG
		root := "/"
		if len(paths) > 0 {
			root = strings.TrimSuffix(paths[0], "/")
		}
		minMB := 0.0
		for i, a := range args {
			if a == "-size" && i+1 < len(args) {
				v := strings.TrimPrefix(args[i+1], "+")
				mult := 1.0
				switch {
				case strings.HasSuffix(v, "G"):
					mult, v = 1024, strings.TrimSuffix(v, "G")
				case strings.HasSuffix(v, "M"):
					v = strings.TrimSuffix(v, "M")
				case strings.HasSuffix(v, "k"):
					mult, v = 1.0/1024, strings.TrimSuffix(v, "k")
				}
				n, _ := strconv.ParseFloat(v, 64)
				minMB = n * mult
			}
		}
		var out []string
		for p, f := range o.Files {
			if (root == "" || root == "/" || strings.HasPrefix(p, root+"/") || p == root) && !f.Dir && f.SizeMB > minMB {
				out = append(out, p)
			}
		}
		sort.Strings(out)
		return joinLines(out), nil
	case "rm":
		for _, p := range paths {
			f := o.Files[p]
			if f == nil {
				if has("f") {
					continue
				}
				return "", fail(1, "rm: cannot remove '%s': No such file or directory", p)
			}
			dir := p[:strings.LastIndex(p, "/")]
			if user != "root" && !o.CanWrite(user, dir) && f.Owner != user {
				return "", fail(1, "rm: cannot remove '%s': Permission denied", p)
			}
			delete(o.Files, p)
		}
		return "", nil
	case "truncate":
		// truncate -s 0 FILE
		for _, p := range paths[1:] {
			f := o.Files[p]
			if f == nil {
				return "", fail(1, "truncate: cannot open '%s' for writing: No such file or directory", p)
			}
			if !o.CanWrite(user, p) {
				return "", fail(1, "truncate: cannot open '%s' for writing: Permission denied", p)
			}
			f.SizeMB = 0
		}
		return "", nil
	case "logrotate":
		// logrotate -f /etc/logrotate.d/NAME: compresses and truncates *.log under the configured dir
		if user != "root" {
			return "", fail(1, "error: error opening state file: Permission denied")
		}
		conf := ""
		if len(paths) > 0 {
			conf = paths[0]
		}
		f := o.Files[conf]
		if f == nil {
			return "", fail(1, "error: cannot stat %s: No such file or directory", conf)
		}
		dir := strings.TrimSpace(strings.SplitN(f.Content, " ", 2)[0])
		dir = dir[:strings.LastIndex(dir, "/")]
		n := 0
		for p, fl := range o.Files {
			if strings.HasPrefix(p, dir+"/") && strings.HasSuffix(p, ".log") && fl.SizeMB > 0 {
				o.Files[p+".1.gz"] = &sim.VFile{Mode: fl.Mode, Owner: fl.Owner, Group: fl.Group, SizeMB: fl.SizeMB * 0.08}
				fl.SizeMB = 0
				n++
			}
		}
		return fmt.Sprintf("rotated %d log file(s)\n", n), nil
	}
	return "", fail(127, "%s: command not found", toks[0])
}

func human(mb float64) string {
	switch {
	case mb >= 1024:
		return fmt.Sprintf("%.1fG", mb/1024)
	case mb >= 1:
		return fmt.Sprintf("%.0fM", mb)
	case mb > 0:
		return fmt.Sprintf("%.0fK", mb*1024)
	}
	return "0"
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
