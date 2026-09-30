package learning

import "testing"

func TestStudyMaterial(t *testing.T) {
	c, err := LoadCatalog("../../content")
	if err != nil {
		t.Fatal(err)
	}
	if p := c.validateStudy(); len(p) > 0 {
		t.Fatal(p)
	}
	topics := map[string]bool{}
	for _, s := range c.Study {
		topics[s.ID] = true
	}
	// Every branch weighted by a certification blueprint has study material.
	for _, ce := range c.Certs {
		for b := range ce.Weights {
			if !topics[b] {
				t.Errorf("cert %s: branch %s has no study topic", ce.ID, b)
			}
		}
	}
	v := c.StudyView("en")
	if len(v) != len(c.Study) || v[0]["name"] == c.Study[0].Name[0] && c.Study[0].Name[0] != c.Study[0].Name[1] {
		t.Fatalf("english view not localized: %v", v[0]["name"])
	}
}
