package cli

import (
	"encoding/json"

	"github.com/neodevesp/gcp-trainer/internal/archsim"
	"gopkg.in/yaml.v3"
)

// arch evaluate DESIGN.yaml [REQUIREMENTS.yaml] [--format=json]
// Architecture Simulator: evaluate a design against requirements.
func (s *Session) archCmd(args []string) (string, error) {
	pos, f := parseArgs(args[1:])
	if len(pos) < 2 || pos[0] != "evaluate" {
		return "", fail(2, "%s", s.tr("uso: arch evaluate design.yaml [requirements.yaml] [--format=json]", "usage: arch evaluate design.yaml [requirements.yaml] [--format=json]"))
	}
	raw, ok := s.Files[s.path(pos[1])]
	if !ok {
		return "", fail(1, s.tr("%s: no existe el archivo", "%s: No such file"), pos[1])
	}
	d, err := archsim.Parse([]byte(raw))
	if err != nil {
		return "", fail(1, "%v", err)
	}
	var req archsim.Requirements
	if len(pos) > 2 {
		rr, ok := s.Files[s.path(pos[2])]
		if !ok {
			return "", fail(1, s.tr("%s: no existe el archivo", "%s: No such file"), pos[2])
		}
		if err := yaml.Unmarshal([]byte(rr), &req); err != nil {
			return "", fail(1, "requirements: %v", err)
		}
	}
	rep := archsim.Evaluate(d, req, s.Lang)
	if firstOr(f["format"], "") == "json" {
		b, _ := json.MarshalIndent(rep, "", "  ")
		return string(b) + "\n", nil
	}
	return rep.Render(s.Lang), nil
}
