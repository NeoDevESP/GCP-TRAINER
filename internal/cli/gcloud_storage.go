package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/sim"
)

var reBucket = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,61}[a-z0-9]$`)

func bucketName(ref string) string {
	ref = strings.TrimPrefix(ref, "gs://")
	b, _, _ := strings.Cut(ref, "/")
	return b
}

func objectName(ref string) string {
	ref = strings.TrimPrefix(ref, "gs://")
	_, o, _ := strings.Cut(ref, "/")
	return o
}

func (c *Cmd) bucket(ref string) (*sim.Bucket, *sim.Project, error) {
	b, p := c.S.State.FindBucket(bucketName(ref))
	if b == nil {
		return nil, nil, fmt.Errorf("HTTPError 404: The specified bucket does not exist.")
	}
	return b, p, nil
}

func (s *Session) createBucket(c *Cmd, ref, location, class string, ubla bool, pap string) (string, error) {
	p, err := c.P()
	if err != nil {
		return "", err
	}
	name := bucketName(ref)
	if !reBucket.MatchString(name) || strings.Contains(name, "google") || strings.Contains(name, "..") {
		return "", fmt.Errorf("HTTPError 400: Invalid bucket name: '%s'", name)
	}
	if b, _ := s.State.FindBucket(name); b != nil {
		return "", fmt.Errorf("HTTPError 409: The requested bucket name is not available. The bucket namespace is shared by all users of the system. Please select a different name and try again.")
	}
	if err := c.NeedProject("storage.buckets.create"); err != nil {
		return "", err
	}
	if location == "" {
		location = "US"
	}
	locUp := strings.ToUpper(location)
	locType := "region"
	switch {
	case sim.MultiRegions[locUp]:
		locType = "multi-region"
	case sim.DualRegions[locUp]:
		locType = "dual-region"
	case sim.ValidRegions[strings.ToLower(location)]:
		locType = "region"
	default:
		return "", fmt.Errorf("HTTPError 400: Invalid location: %s", location)
	}
	if err := s.checkRegion(strings.ToLower(location), p.ID); err != nil && locType == "region" {
		return "", err
	}
	if !s.State.OrgPolicyAllows(p.ID, "gcp.resourceLocations", location) {
		return "", fmt.Errorf("HTTPError 412: 'us' violates constraint 'constraints/gcp.resourceLocations'")
	}
	if class == "" {
		class = "STANDARD"
	}
	class = strings.ToUpper(class)
	switch class {
	case "STANDARD", "NEARLINE", "COLDLINE", "ARCHIVE":
	default:
		return "", fmt.Errorf("HTTPError 400: Invalid storage class %s", class)
	}
	if pap == "" {
		pap = "inherited"
		if s.State.OrgPolicyEnforced(p.ID, "storage.publicAccessPrevention") {
			pap = "enforced"
		}
	}
	if s.State.OrgPolicyEnforced(p.ID, "storage.uniformBucketLevelAccess") {
		ubla = true
	}
	p.Buckets[name] = &sim.Bucket{Name: name, Project: p.ID, Location: locUp, LocationType: locType, StorageClass: class, UBLA: ubla, PAP: pap,
		Objects: map[string]*sim.Object{}, Labels: map[string]string{}, SoftDeleteSec: 604800,
		IAM: sim.Policy{Bindings: []sim.Binding{
			{Role: "roles/storage.legacyBucketOwner", Members: []string{"projectEditor:" + p.ID, "projectOwner:" + p.ID}},
			{Role: "roles/storage.legacyBucketReader", Members: []string{"projectViewer:" + p.ID}},
		}}}
	c.S.State.Audit(p.ID, c.Principal(), "storage.googleapis.com", "storage.buckets.create", "projects/_/buckets/"+name)
	return "Creating gs://" + name + "/...\n", nil
}

func parseLifecycle(content string) ([]sim.LifecycleRule, error) {
	var doc struct {
		Rule      []sim.LifecycleRule `json:"rule"`
		Lifecycle struct {
			Rule []sim.LifecycleRule `json:"rule"`
		} `json:"lifecycle"`
	}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return nil, fmt.Errorf("Found invalid JSON/YAML for the lifecycle rule: %v", err)
	}
	rules := doc.Rule
	if len(rules) == 0 {
		rules = doc.Lifecycle.Rule
	}
	for _, r := range rules {
		switch r.Action.Type {
		case "Delete", "SetStorageClass", "AbortIncompleteMultipartUpload":
		default:
			return nil, fmt.Errorf("Invalid lifecycle action type %q", r.Action.Type)
		}
		if r.Action.Type == "SetStorageClass" && r.Action.StorageClass == "" {
			return nil, fmt.Errorf("SetStorageClass requires storageClass")
		}
	}
	return rules, nil
}

func (s *Session) updateBucket(c *Cmd, b *sim.Bucket) error {
	if err := c.Need("storage.buckets.update", s.State.BucketResource(b, "")); err != nil {
		return err
	}
	if c.Has("uniform-bucket-level-access") {
		b.UBLA = c.Bool("uniform-bucket-level-access")
	}
	if c.Bool("no-uniform-bucket-level-access") {
		if s.State.OrgPolicyEnforced(b.Project, "storage.uniformBucketLevelAccess") {
			return fmt.Errorf("HTTPError 412: Request violates constraint 'constraints/storage.uniformBucketLevelAccess'")
		}
		b.UBLA = false
	}
	if v := c.Str("public-access-prevention", ""); v != "" {
		if v == "true" {
			v = "enforced"
		}
		b.PAP = v
	}
	if c.Bool("no-public-access-prevention") {
		b.PAP = "inherited"
	}
	if c.Bool("versioning") {
		b.Versioning = true
	}
	if c.Bool("no-versioning") {
		b.Versioning = false
	}
	if f := c.Str("lifecycle-file", ""); f != "" {
		content, ok := s.Files[s.path(f)]
		if !ok {
			return fmt.Errorf("Unable to read file [%s]: [Errno 2] No such file or directory", f)
		}
		rules, err := parseLifecycle(content)
		if err != nil {
			return err
		}
		b.Lifecycle = rules
	}
	if c.Bool("clear-lifecycle") {
		b.Lifecycle = nil
	}
	if v := c.Str("default-storage-class", ""); v != "" {
		b.StorageClass = strings.ToUpper(v)
	}
	if v := c.Str("default-encryption-key", ""); v != "" {
		if err := s.checkCMEK(v, "service-"+s.State.Projects[b.Project].Number+"@gs-project-accounts.iam.gserviceaccount.com"); err != nil {
			return err
		}
		b.KMSKey = v
	}
	if c.Bool("clear-default-encryption-key") {
		b.KMSKey = ""
	}
	if v := c.Str("retention-period", ""); v != "" {
		b.RetentionSec = parseDurationSec(v)
	}
	if v := c.Str("soft-delete-duration", ""); v != "" {
		b.SoftDeleteSec = parseDurationSec(v)
	}
	for k, v := range c.KV("update-labels") {
		b.Labels[k] = v
	}
	if c.Bool("log-bucket") || c.Has("log-bucket") {
		b.Logging = true
	}
	s.State.Audit(b.Project, c.Principal(), "storage.googleapis.com", "storage.buckets.update", "projects/_/buckets/"+b.Name)
	return nil
}

func parseDurationSec(v string) int {
	n := 0
	unit := v[len(v)-1:]
	fmt.Sscanf(v, "%d", &n)
	switch unit {
	case "d":
		return n * 86400
	case "h":
		return n * 3600
	case "m":
		return n * 60
	case "y":
		return n * 365 * 86400
	}
	return n
}

// checkCMEK validates a KMS key reference and the encrypter permission of the service agent.
func (s *Session) checkCMEK(key, agent string) error {
	parts := strings.Split(key, "/")
	if len(parts) != 8 || parts[0] != "projects" || parts[4] != "keyRings" || parts[6] != "cryptoKeys" {
		return fmt.Errorf("Invalid KMS key name %q (expected projects/P/locations/L/keyRings/R/cryptoKeys/K)", key)
	}
	p := s.State.Projects[parts[1]]
	if p == nil || p.KeyRings[parts[5]] == nil || p.KeyRings[parts[5]].Keys[parts[7]] == nil {
		return fmt.Errorf("NOT_FOUND: KMS key %s not found", key)
	}
	k := p.KeyRings[parts[5]].Keys[parts[7]]
	if !s.State.Allowed("serviceAccount:"+agent, "cloudkms.cryptoKeyVersions.useToEncrypt", sim.Resource{Project: p.ID, Type: "cloudkms.googleapis.com/CryptoKey", Name: key, Service: "cloudkms.googleapis.com", Policies: []*sim.Policy{&k.IAM}}) {
		return fmt.Errorf("HTTPError 403: Permission denied on Cloud KMS key. Please ensure that your Cloud Storage service account (%s) has been authorized to use this key (roles/cloudkms.cryptoKeyEncrypterDecrypter).", agent)
	}
	return nil
}

func bucketView(b *sim.Bucket) map[string]any {
	return map[string]any{"name": b.Name, "storage_url": "gs://" + b.Name + "/", "location": b.Location, "location_type": b.LocationType,
		"default_storage_class": b.StorageClass, "uniform_bucket_level_access": b.UBLA, "public_access_prevention": b.PAP,
		"versioning_enabled": b.Versioning, "lifecycle_config": map[string]any{"rule": b.Lifecycle}, "default_kms_key": b.KMSKey,
		"retention_policy": b.RetentionSec, "labels": b.Labels, "soft_delete_policy": map[string]any{"retentionDurationSeconds": b.SoftDeleteSec}}
}

func (s *Session) storageCp(c *Cmd, src, dst string) (string, error) {
	switch {
	case strings.HasPrefix(dst, "gs://"):
		b, _, err := c.bucket(dst)
		if err != nil {
			return "", err
		}
		obj := objectName(dst)
		var content string
		if strings.HasPrefix(src, "gs://") {
			sb, _, err := c.bucket(src)
			if err != nil {
				return "", err
			}
			if err := c.Need("storage.objects.get", s.State.BucketResource(sb, objectName(src))); err != nil {
				return "", err
			}
			o := sb.Objects[objectName(src)]
			if o == nil {
				return "", fmt.Errorf("The following URLs matched no objects or files:\n-%s", src)
			}
			content = o.Content
			if obj == "" || strings.HasSuffix(obj, "/") {
				obj += o.Name[strings.LastIndex(o.Name, "/")+1:]
			}
		} else {
			if src == "-" {
				content = c.Stdin
			} else {
				v, ok := s.Files[s.path(src)]
				if !ok {
					return "", fmt.Errorf("The following URLs matched no objects or files:\n-%s", src)
				}
				content = v
			}
			if obj == "" || strings.HasSuffix(obj, "/") {
				obj += src[strings.LastIndex(src, "/")+1:]
			}
		}
		if err := c.Need("storage.objects.create", s.State.BucketResource(b, obj)); err != nil {
			return "", err
		}
		gen := 1
		if old := b.Objects[obj]; old != nil {
			if b.RetentionSec > 0 {
				return "", fmt.Errorf("HTTPError 403: Object '%s' is subject to bucket's retention policy and cannot be overwritten", obj)
			}
			gen = old.Generation + 1
			if err := c.Need("storage.objects.delete", s.State.BucketResource(b, obj)); err != nil {
				return "", err
			}
		}
		b.Objects[obj] = &sim.Object{Name: obj, Size: len(content), Content: content, StorageClass: b.StorageClass, Generation: gen, Updated: s.State.Now()}
		s.State.Audit(b.Project, c.Principal(), "storage.googleapis.com", "storage.objects.create", "projects/_/buckets/"+b.Name+"/objects/"+obj)
		return fmt.Sprintf("Copying %s to gs://%s/%s\n  Completed files 1/1 | %dB\n", src, b.Name, obj, len(content)), nil
	case strings.HasPrefix(src, "gs://"):
		b, _, err := c.bucket(src)
		if err != nil {
			return "", err
		}
		obj := objectName(src)
		if err := c.Need("storage.objects.get", s.State.BucketResource(b, obj)); err != nil {
			return "", err
		}
		o := b.Objects[obj]
		if o == nil {
			return "", fmt.Errorf("The following URLs matched no objects or files:\n-%s", src)
		}
		if dst == "-" {
			return o.Content, nil
		}
		if dst == "." || strings.HasSuffix(dst, "/") {
			dst = strings.TrimSuffix(dst, ".") + obj[strings.LastIndex(obj, "/")+1:]
		}
		s.Files[s.path(dst)] = o.Content
		return fmt.Sprintf("Copying %s to file://%s\n  Completed files 1/1\n", src, dst), nil
	}
	return "", fmt.Errorf("at least one of the source or destination must be a gs:// URL")
}

func (s *Session) storageLs(c *Cmd, ref string) (any, error) {
	if ref == "" {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("storage.buckets.list"); err != nil {
			return nil, err
		}
		var out strings.Builder
		for _, k := range sim.SortedKeys(p.Buckets) {
			out.WriteString("gs://" + k + "/\n")
		}
		return out.String(), nil
	}
	b, _, err := c.bucket(ref)
	if err != nil {
		return nil, err
	}
	if err := c.Need("storage.objects.list", s.State.BucketResource(b, "")); err != nil {
		return nil, err
	}
	prefix := objectName(ref)
	var out strings.Builder
	for _, k := range sim.SortedKeys(b.Objects) {
		if strings.HasPrefix(k, strings.TrimSuffix(prefix, "*")) {
			if c.Bool("l") || c.Bool("long") {
				out.WriteString(fmt.Sprintf("%10d  %s  gs://%s/%s\n", b.Objects[k].Size, b.Objects[k].Updated, b.Name, k))
			} else {
				out.WriteString("gs://" + b.Name + "/" + k + "\n")
			}
		}
	}
	return out.String(), nil
}

func (s *Session) storageRm(c *Cmd, refs []string) (string, error) {
	var out strings.Builder
	for _, ref := range refs {
		b, p, err := c.bucket(ref)
		if err != nil {
			return out.String(), err
		}
		obj := objectName(ref)
		if obj == "" || obj == "**" || obj == "*" {
			// remove all objects (and bucket with -r)
			for k := range b.Objects {
				if err := c.Need("storage.objects.delete", s.State.BucketResource(b, k)); err != nil {
					return out.String(), err
				}
				delete(b.Objects, k)
			}
			if c.Bool("r") || c.Bool("recursive") || strings.HasSuffix(ref, "/") && obj == "" {
				if err := c.Need("storage.buckets.delete", s.State.BucketResource(b, "")); err != nil {
					return out.String(), err
				}
				delete(p.Buckets, b.Name)
				out.WriteString("Removing gs://" + b.Name + "/...\n")
			}
			continue
		}
		if b.Objects[obj] == nil {
			return out.String(), fmt.Errorf("The following URLs matched no objects or files:\n-%s", ref)
		}
		if b.RetentionSec > 0 {
			return out.String(), fmt.Errorf("HTTPError 403: Object '%s' is subject to bucket's retention policy and cannot be deleted", obj)
		}
		if err := c.Need("storage.objects.delete", s.State.BucketResource(b, obj)); err != nil {
			return out.String(), err
		}
		delete(b.Objects, obj)
		s.State.Audit(p.ID, c.Principal(), "storage.googleapis.com", "storage.objects.delete", "projects/_/buckets/"+b.Name+"/objects/"+obj)
		out.WriteString("Removing " + ref + "...\n")
	}
	return out.String(), nil
}

func (s *Session) bucketIAM(c *Cmd, b *sim.Bucket, add bool, memberRef, role string, cond *sim.Condition) (string, error) {
	if err := c.Need("storage.buckets.setIamPolicy", s.State.BucketResource(b, "")); err != nil {
		return "", err
	}
	m, err := member(memberRef)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(role, "roles/") {
		role = "roles/" + role
	}
	if add && (m == "allUsers" || m == "allAuthenticatedUsers") {
		if b.PAP == "enforced" || s.State.OrgPolicyEnforced(b.Project, "storage.publicAccessPrevention") {
			return "", fmt.Errorf("HTTPError 412: Public access prevention is enforced on bucket %s; the policy cannot grant access to %s.", b.Name, m)
		}
	}
	if add {
		if !s.State.RoleExists(role) && !strings.HasPrefix(role, "roles/storage.legacy") {
			return "", fmt.Errorf("HTTPError 400: Role %s is not supported for this resource.", role)
		}
		b.IAM.AddBinding(role, m, cond)
	} else if !b.IAM.RemoveBinding(role, m) {
		return "", fmt.Errorf("Policy binding with the specified principal, role, and condition not found!")
	}
	s.State.Audit(b.Project, c.Principal(), "storage.googleapis.com", "storage.setIamPermissions", "projects/_/buckets/"+b.Name)
	r, _ := render(policyView(&b.IAM), Flags{})
	return r, nil
}

func init() {
	reg("storage buckets create", func(c *Cmd) (any, error) {
		ref, err := c.Arg(0, "URL")
		if err != nil {
			return nil, err
		}
		pap := ""
		if c.Has("public-access-prevention") {
			pap = "enforced"
			if v := c.Str("public-access-prevention", ""); v != "true" {
				pap = v
			}
		}
		out, err := c.S.createBucket(c, ref, c.Str("location", c.Str("l", "")), c.Str("default-storage-class", c.Str("c", "")), c.Bool("uniform-bucket-level-access") || c.Bool("b"), pap)
		if err != nil {
			return nil, err
		}
		b, _ := c.S.State.FindBucket(bucketName(ref))
		if c.Has("lifecycle-file") || c.Has("versioning") || c.Has("default-encryption-key") || c.Has("retention-period") || c.Has("soft-delete-duration") || c.Has("update-labels") {
			if err := c.S.updateBucket(c, b); err != nil {
				return nil, err
			}
		}
		for k, v := range c.KV("labels") {
			b.Labels[k] = v
		}
		return out, nil
	})
	reg("storage buckets update", func(c *Cmd) (any, error) {
		ref, err := c.Arg(0, "URL")
		if err != nil {
			return nil, err
		}
		b, _, err := c.bucket(ref)
		if err != nil {
			return nil, err
		}
		if err := c.S.updateBucket(c, b); err != nil {
			return nil, err
		}
		return "Updating gs://" + b.Name + "/...\n  Completed 1\n", nil
	})
	reg("storage buckets describe", func(c *Cmd) (any, error) {
		ref, err := c.Arg(0, "URL")
		if err != nil {
			return nil, err
		}
		b, _, err := c.bucket(ref)
		if err != nil {
			return nil, err
		}
		if err := c.Need("storage.buckets.get", c.S.State.BucketResource(b, "")); err != nil {
			return nil, err
		}
		return Obj{V: bucketView(b)}, nil
	})
	reg("storage buckets list", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		if err := c.NeedProject("storage.buckets.list"); err != nil {
			return nil, err
		}
		var rows []any
		for _, k := range sim.SortedKeys(p.Buckets) {
			rows = append(rows, bucketView(p.Buckets[k]))
		}
		return Table{Cols: []Col{{"NAME", "name"}, {"LOCATION", "location"}, {"STORAGE_CLASS", "default_storage_class"}, {"UBLA", "uniform_bucket_level_access"}, {"PAP", "public_access_prevention"}}, Rows: rows}, nil
	})
	reg("storage buckets delete", func(c *Cmd) (any, error) {
		ref, err := c.Arg(0, "URL")
		if err != nil {
			return nil, err
		}
		b, p, err := c.bucket(ref)
		if err != nil {
			return nil, err
		}
		if err := c.Need("storage.buckets.delete", c.S.State.BucketResource(b, "")); err != nil {
			return nil, err
		}
		if len(b.Objects) > 0 {
			return nil, fmt.Errorf("HTTPError 409: The bucket you tried to delete is not empty.")
		}
		delete(p.Buckets, b.Name)
		c.Audit("storage.googleapis.com", "storage.buckets.delete", "projects/_/buckets/"+b.Name)
		return "Removing gs://" + b.Name + "/...\n", nil
	})
	reg("storage buckets get-iam-policy", func(c *Cmd) (any, error) {
		ref, err := c.Arg(0, "URL")
		if err != nil {
			return nil, err
		}
		b, _, err := c.bucket(ref)
		if err != nil {
			return nil, err
		}
		if err := c.Need("storage.buckets.getIamPolicy", c.S.State.BucketResource(b, "")); err != nil {
			return nil, err
		}
		return policyView(&b.IAM), nil
	})
	iamBind := func(add bool) handler {
		return func(c *Cmd) (any, error) {
			ref, err := c.Arg(0, "URL")
			if err != nil {
				return nil, err
			}
			b, _, err := c.bucket(ref)
			if err != nil {
				return nil, err
			}
			cond, err := parseCondition(c.Str("condition", ""))
			if err != nil {
				return nil, err
			}
			if cond != nil && !b.UBLA {
				return nil, fmt.Errorf("HTTPError 400: IAM Conditions can only be used on buckets with uniform bucket-level access enabled.")
			}
			if !strings.HasPrefix(c.Str("role", ""), "roles/") && c.Str("role", "") == "" {
				return nil, fmt.Errorf("argument --role: Must be specified.")
			}
			return c.S.bucketIAM(c, b, add, c.Str("member", ""), c.Str("role", ""), cond)
		}
	}
	reg("storage buckets add-iam-policy-binding", iamBind(true))
	reg("storage buckets remove-iam-policy-binding", iamBind(false))
	reg("storage cp", func(c *Cmd) (any, error) {
		if len(c.Args) < 2 {
			return nil, fmt.Errorf("argument SOURCE DESTINATION: Must be specified.")
		}
		var out strings.Builder
		dst := c.Args[len(c.Args)-1]
		for _, src := range c.Args[:len(c.Args)-1] {
			o, err := c.S.storageCp(c, src, dst)
			out.WriteString(o)
			if err != nil {
				return out.String(), err
			}
		}
		return out.String(), nil
	})
	reg("storage mv", func(c *Cmd) (any, error) {
		if len(c.Args) < 2 {
			return nil, fmt.Errorf("argument SOURCE DESTINATION: Must be specified.")
		}
		out, err := c.S.storageCp(c, c.Args[0], c.Args[1])
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(c.Args[0], "gs://") {
			if _, err := c.S.storageRm(c, []string{c.Args[0]}); err != nil {
				return nil, err
			}
		}
		return out, nil
	})
	reg("storage ls", func(c *Cmd) (any, error) {
		ref := ""
		if len(c.Args) > 0 {
			ref = c.Args[0]
		}
		return c.S.storageLs(c, ref)
	})
	reg("storage cat", func(c *Cmd) (any, error) {
		ref, err := c.Arg(0, "URL")
		if err != nil {
			return nil, err
		}
		return c.S.storageCp(c, ref, "-")
	})
	reg("storage rm", func(c *Cmd) (any, error) {
		if len(c.Args) == 0 {
			return nil, fmt.Errorf("argument URLS: Must be specified.")
		}
		return c.S.storageRm(c, c.Args)
	})
	reg("storage objects list", registry["storage ls"])
	reg("storage objects describe", func(c *Cmd) (any, error) {
		ref, err := c.Arg(0, "URL")
		if err != nil {
			return nil, err
		}
		b, _, err := c.bucket(ref)
		if err != nil {
			return nil, err
		}
		o := b.Objects[objectName(ref)]
		if o == nil {
			return nil, fmt.Errorf("HTTPError 404: No such object")
		}
		if err := c.Need("storage.objects.get", c.S.State.BucketResource(b, o.Name)); err != nil {
			return nil, err
		}
		return Obj{V: map[string]any{"name": o.Name, "bucket": b.Name, "size": o.Size, "storage_class": o.StorageClass, "generation": o.Generation, "update_time": o.Updated}}, nil
	})
	reg("storage service-agent", func(c *Cmd) (any, error) {
		p, err := c.P()
		if err != nil {
			return nil, err
		}
		return "service-" + p.Number + "@gs-project-accounts.iam.gserviceaccount.com\n", nil
	})
	reg("storage sign-url", func(c *Cmd) (any, error) {
		ref, err := c.Arg(0, "URL")
		if err != nil {
			return nil, err
		}
		b, _, err := c.bucket(ref)
		if err != nil {
			return nil, err
		}
		dur := c.Str("duration", "1h")
		if d := parseDurationSec(dur); d > 7*86400 {
			return nil, fmt.Errorf("Max valid duration allowed is 7 days")
		}
		if c.S.Impersonate == "" && c.Str("private-key-file", "") == "" {
			return nil, fmt.Errorf("Signing requires a service account: use --impersonate-service-account or --private-key-file")
		}
		return fmt.Sprintf("signed_url: https://storage.googleapis.com/%s/%s?X-Goog-Algorithm=GOOG4-RSA-SHA256&X-Goog-Expires=%d&X-Goog-Signature=%s\n", b.Name, objectName(ref), parseDurationSec(dur), c.S.State.ID(64)), nil
	})
}

// gsutil maps the legacy CLI onto the same operations.
func (s *Session) gsutil(args []string, stdin string) (string, error) {
	pos, f := parseArgs(args)
	c := &Cmd{S: s, F: f, Stdin: stdin, Path: "gsutil"}
	if len(pos) == 0 {
		return "Usage: gsutil [-m] COMMAND ...\n", nil
	}
	if pos[0] == "-m" {
		pos = pos[1:]
	}
	c.Args = pos[1:]
	wrap := func(out string, err error) (string, error) {
		if err != nil {
			return out, fail(1, "%s", strings.Replace(err.Error(), "HTTPError", "AccessDeniedException:", 1))
		}
		return out, nil
	}
	switch pos[0] {
	case "mb":
		if len(c.Args) == 0 {
			return "", fail(1, "CommandException: The mb command requires at least 1 argument.")
		}
		pap := ""
		if f["p"] != nil {
			pap = "enforced"
		}
		return wrap(s.createBucket(c, c.Args[0], c.Str("l", ""), c.Str("c", ""), c.Str("b", "") == "on", pap))
	case "ls":
		ref := ""
		if len(c.Args) > 0 {
			ref = c.Args[0]
		}
		v, err := s.storageLs(c, ref)
		if err != nil {
			return wrap("", err)
		}
		return v.(string), nil
	case "cp":
		if len(c.Args) < 2 {
			return "", fail(1, "CommandException: Wrong number of arguments for \"cp\" command.")
		}
		return wrap(s.storageCp(c, c.Args[0], c.Args[1]))
	case "cat":
		if len(c.Args) < 1 {
			return "", fail(1, "CommandException: cat requires a URL")
		}
		return wrap(s.storageCp(c, c.Args[0], "-"))
	case "rm":
		return wrap(s.storageRm(c, c.Args))
	case "rb":
		b, p, err := c.bucket(c.Args[0])
		if err != nil {
			return wrap("", err)
		}
		if len(b.Objects) > 0 {
			return "", fail(1, "BucketNotEmpty: 409 The bucket you tried to delete is not empty.")
		}
		delete(p.Buckets, b.Name)
		return "Removing gs://" + b.Name + "/...\n", nil
	case "iam":
		if len(c.Args) < 2 {
			return "", fail(1, "CommandException: iam requires a subcommand")
		}
		switch c.Args[0] {
		case "get":
			b, _, err := c.bucket(c.Args[1])
			if err != nil {
				return wrap("", err)
			}
			out, _ := render(Obj{V: map[string]any{"bindings": b.IAM.Bindings}}, Flags{"format": {"json"}})
			return out, nil
		case "ch":
			del := false
			rest := c.Args[1:]
			if f["d"] != nil {
				del = true
				rest = append([]string{last(f["d"])}, rest...)
			}
			if len(rest) < 2 {
				return "", fail(1, "CommandException: iam ch requires MEMBER:ROLE and a bucket URL")
			}
			spec, url := rest[0], rest[len(rest)-1]
			b, _, err := c.bucket(url)
			if err != nil {
				return wrap("", err)
			}
			i := strings.LastIndex(spec, ":")
			if del && i < 0 {
				spec += ":objectViewer"
				i = strings.LastIndex(spec, ":")
			}
			mem, role := spec[:i], spec[i+1:]
			if strings.HasPrefix(role, "roles/") || strings.Contains(spec, ":roles/") {
				j := strings.Index(spec, ":roles/")
				mem, role = spec[:j], spec[j+1:]
			}
			short := map[string]string{"objectViewer": "roles/storage.objectViewer", "objectCreator": "roles/storage.objectCreator", "objectAdmin": "roles/storage.objectAdmin", "admin": "roles/storage.admin", "legacyBucketReader": "roles/storage.legacyBucketReader", "objectUser": "roles/storage.objectUser"}
			if r, ok := short[role]; ok {
				role = r
			}
			if !strings.Contains(mem, ":") && mem != "allUsers" && mem != "allAuthenticatedUsers" {
				mem = "user:" + mem
			}
			_, err = s.bucketIAM(c, b, !del, mem, role, nil)
			return wrap("", err)
		}
	case "lifecycle":
		if len(c.Args) >= 3 && c.Args[0] == "set" {
			b, _, err := c.bucket(c.Args[2])
			if err != nil {
				return wrap("", err)
			}
			c.F = Flags{"lifecycle-file": {c.Args[1]}}
			if err := s.updateBucket(c, b); err != nil {
				return wrap("", err)
			}
			return "Setting lifecycle configuration on gs://" + b.Name + "/...\n", nil
		}
		if len(c.Args) >= 2 && c.Args[0] == "get" {
			b, _, err := c.bucket(c.Args[1])
			if err != nil {
				return wrap("", err)
			}
			if len(b.Lifecycle) == 0 {
				return "gs://" + b.Name + "/ has no lifecycle configuration.\n", nil
			}
			j, _ := json.Marshal(map[string]any{"rule": b.Lifecycle})
			return string(j) + "\n", nil
		}
	case "versioning", "pap", "ubla", "uniformbucketlevelaccess", "defstorageclass":
		if len(c.Args) >= 3 && c.Args[0] == "set" {
			b, _, err := c.bucket(c.Args[2])
			if err != nil {
				return wrap("", err)
			}
			v := c.Args[1]
			switch pos[0] {
			case "versioning":
				c.F = Flags{map[string]string{"on": "versioning", "off": "no-versioning"}[v]: {"true"}}
			case "pap":
				c.F = Flags{"public-access-prevention": {v}}
			case "ubla", "uniformbucketlevelaccess":
				c.F = Flags{"uniform-bucket-level-access": {map[string]string{"on": "true", "off": "false"}[v]}}
			case "defstorageclass":
				c.F = Flags{"default-storage-class": {v}}
			}
			if err := s.updateBucket(c, b); err != nil {
				return wrap("", err)
			}
			return "Setting " + pos[0] + " on gs://" + b.Name + "/...\n", nil
		}
		if len(c.Args) >= 2 && c.Args[0] == "get" {
			b, _, err := c.bucket(c.Args[1])
			if err != nil {
				return wrap("", err)
			}
			switch pos[0] {
			case "versioning":
				return fmt.Sprintf("gs://%s: %s\n", b.Name, map[bool]string{true: "Enabled", false: "Suspended"}[b.Versioning]), nil
			case "pap":
				return fmt.Sprintf("gs://%s: %s\n", b.Name, b.PAP), nil
			default:
				return fmt.Sprintf("Uniform bucket-level access setting for gs://%s:\n  Enabled: %v\n", b.Name, b.UBLA), nil
			}
		}
	case "du":
		b, _, err := c.bucket(c.Args[len(c.Args)-1])
		if err != nil {
			return wrap("", err)
		}
		total := 0
		for _, o := range b.Objects {
			total += o.Size
		}
		return fmt.Sprintf("%d  gs://%s\n", total, b.Name), nil
	case "signurl":
		return "", fail(1, "CommandException: use `gcloud storage sign-url` in this simulator")
	}
	return "", fail(1, "CommandException: Invalid command \"%s\" (not supported by the simulator).", pos[0])
}
