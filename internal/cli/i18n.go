package cli

import "github.com/neodevesp/gcp-trainer/internal/i18n"

// tr picks the Spanish or English text of the platform's own commands
// (help, ticket, why, whatif, chaos, interview, arch). Output that imitates
// real tools (gcloud, kubectl, terraform…) stays in English, as in real life.
func (s *Session) tr(es, en string) string { return i18n.P(s.Lang, es, en) }
