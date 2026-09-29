// Package i18n holds the platform's language support. Spanish is the primary
// language; English is a translation.
//
// Server-side texts keep both versions side by side in the code:
//
//	i18n.P(lang, "resuelto sin pistas", "solved without hints")
//
// Content (labs, skills, tracks...) is written in Spanish with an `en:`
// overlay per file; see scenario.Lab.Localized.
package i18n

import (
	"fmt"
	"net/http"
	"strings"
)

const (
	ES = "es"
	EN = "en"
	// Default is the primary language of the platform.
	Default = ES
)

// Supported lists the available languages, primary first.
var Supported = []string{ES, EN}

// Norm maps any language tag ("en-GB", "EN", "es_ES", "") to a supported
// language, defaulting to Spanish.
func Norm(tag string) string {
	t := strings.ToLower(strings.TrimSpace(tag))
	if strings.HasPrefix(t, "en") {
		return EN
	}
	return ES
}

// P picks the text for the language.
func P(lang, es, en string) string {
	if lang == EN {
		return en
	}
	return es
}

// Pf is P with fmt formatting.
func Pf(lang, es, en string, a ...any) string {
	return fmt.Sprintf(P(lang, es, en), a...)
}

// FromRequest resolves the language of an HTTP request: explicit X-Lang
// header or ?lang= parameter, then the user's saved preference, then the
// browser's Accept-Language, then Spanish.
func FromRequest(r *http.Request, userPref string) string {
	if v := r.Header.Get("X-Lang"); v != "" {
		return Norm(v)
	}
	if v := r.URL.Query().Get("lang"); v != "" {
		return Norm(v)
	}
	if userPref != "" {
		return Norm(userPref)
	}
	if al := r.Header.Get("Accept-Language"); al != "" {
		first := strings.SplitN(strings.SplitN(al, ",", 2)[0], ";", 2)[0]
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(first)), "en") {
			return EN
		}
	}
	return Default
}
