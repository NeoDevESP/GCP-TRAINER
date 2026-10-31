package sim

import (
	"fmt"
	"strings"
)

// CMEK: a bucket encrypted with a customer-managed key only works while the
// key's primary version is enabled and the Cloud Storage service agent may
// use the key.

// KeyByName finds projects/P/locations/L/keyRings/R/cryptoKeys/K.
func (s *State) KeyByName(name string) (*Project, *KeyRing, *CryptoKey) {
	parts := strings.Split(name, "/")
	if len(parts) != 8 {
		return nil, nil, nil
	}
	p := s.Projects[parts[1]]
	if p == nil || p.KeyRings[parts[5]] == nil {
		return nil, nil, nil
	}
	kr := p.KeyRings[parts[5]]
	return p, kr, kr.Keys[parts[7]]
}

// StorageAgent is the Cloud Storage service agent of a project.
func (s *State) StorageAgent(project string) string {
	if p := s.Projects[project]; p != nil {
		return "service-" + p.Number + "@gs-project-accounts.iam.gserviceaccount.com"
	}
	return ""
}

// CMEKUsable checks that a CMEK bucket can be written (encrypt) or read.
func (s *State) CMEKUsable(b *Bucket, write bool) (bool, string) {
	if b == nil || b.KMSKey == "" {
		return true, ""
	}
	p, _, k := s.KeyByName(b.KMSKey)
	if k == nil {
		return false, fmt.Sprintf("HTTPError 400: Cloud KMS key %s does not exist or is not accessible", b.KMSKey)
	}
	v := k.PrimaryVersion()
	if v == nil || v.State != "ENABLED" {
		state, id := "DESTROYED", 0
		if v != nil {
			state, id = v.State, v.ID
		}
		return false, fmt.Sprintf("HTTPError 400: The Cloud KMS key version %s/cryptoKeyVersions/%d is not enabled (state %s): FAILED_PRECONDITION", b.KMSKey, id, state)
	}
	if !write {
		// existing objects may be encrypted with any older version
		for _, kv := range k.Versions {
			if kv.State != "ENABLED" && kv.State != "DESTROYED" {
				return false, fmt.Sprintf("HTTPError 400: The Cloud KMS key version %s/cryptoKeyVersions/%d is not enabled (state %s): FAILED_PRECONDITION", b.KMSKey, kv.ID, kv.State)
			}
		}
	}
	perm := "cloudkms.cryptoKeyVersions.useToDecrypt"
	if write {
		perm = "cloudkms.cryptoKeyVersions.useToEncrypt"
	}
	agent := s.StorageAgent(b.Project)
	if !s.Allowed("serviceAccount:"+agent, perm, Resource{Project: p.ID, Type: "cloudkms.googleapis.com/CryptoKey", Name: b.KMSKey, Service: "cloudkms.googleapis.com", Policies: []*Policy{&k.IAM}}) {
		return false, fmt.Sprintf("HTTPError 403: Permission denied on Cloud KMS key. Please ensure that your Cloud Storage service account (%s) has been authorized to use this key (roles/cloudkms.cryptoKeyEncrypterDecrypter).", agent)
	}
	return true, ""
}
