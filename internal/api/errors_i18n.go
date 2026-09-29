package api

import (
	"strings"

	"github.com/neodevesp/gcp-trainer/internal/i18n"
)

// errorsES translates API error messages into Spanish, the platform's primary
// language. Messages already written per language (grader, terminal, company)
// pass through unchanged.
var errorsES = map[string]string{
	"rate limit exceeded": "demasiadas peticiones; espera un momento",
	"OIDC not configured": "el inicio de sesión con OIDC no está configurado",
	"session expired":     "la sesión ha caducado",
	"session not found":   "no se encuentra la sesión",
	"not your session":    "esta sesión no es tuya",
	"findings are visible through `gcloud scc findings list`": "los hallazgos se consultan con `gcloud scc findings list`",
	"class not found":             "no se encuentra la clase",
	"name required":               "el nombre es obligatorio",
	"user not found":              "no se encuentra el usuario",
	"pool disabled":               "el pool de proyectos está desactivado",
	"invalid role":                "rol no válido",
	"unknown user":                "usuario desconocido",
	"email and name are required": "el correo y el nombre son obligatorios",
	"email already registered":    "ese correo ya está registrado",
	"invalid credentials":         "credenciales no válidas",
	"lab not found":               "no se encuentra el laboratorio",
	"you already have 3 running labs; stop one first": "ya tienes 3 laboratorios en marcha; detén uno primero",
	"failure library not loaded":                      "la biblioteca de fallos no está cargada",
	"attempt not running":                             "el intento no está en curso",
	"boss battles have no hints":                      "los jefes finales no tienen pistas",
	"attempt already submitted":                       "el intento ya se ha enviado",
	"generated lab id mismatch":                       "el identificador del laboratorio generado no coincide",
	"invalid file":                                    "archivo no válido",
	"no more hints":                                   "no quedan más pistas",
	"password must have at least 8 characters":        "la contraseña debe tener al menos 8 caracteres",
	"malformed token":                                 "token mal formado",
	"invalid signature":                               "firma no válida",
	"token expired":                                   "el token ha caducado",
	"token exchange failed":                           "falló el intercambio de token",
	"userinfo failed":                                 "falló la consulta de userinfo",
}

// errorPrefixES translates messages that carry a variable suffix.
var errorPrefixES = [][2]string{
	{"requires role ", "requiere el rol "},
	{"unauthenticated: ", "no autenticado: "},
	{"monthly real-GCP quota reached", "se ha alcanzado la cuota mensual de GCP real"},
	{"mission ", "misión "},
	{"lab ", "laboratorio "},
	{"unknown view ", "vista desconocida "},
}

var errorSuffixES = [][2]string{
	{" is not available", " no está disponible"},
	{" not found", " no encontrado"},
	{" sessions)", " sesiones)"},
}

// localizeError returns an API error message in the given language.
func localizeError(lang, msg string) string {
	if lang == i18n.EN {
		return msg
	}
	if v, ok := errorsES[msg]; ok {
		return v
	}
	out, changed := msg, false
	for _, p := range errorPrefixES {
		if strings.HasPrefix(out, p[0]) {
			out, changed = p[1]+strings.TrimPrefix(out, p[0]), true
			if rest, ok := strings.CutPrefix(out, p[1]); ok {
				if v, ok := errorsES[rest]; ok {
					out = p[1] + v
				}
			}
			break
		}
	}
	for _, p := range errorSuffixES {
		if changed && strings.HasSuffix(out, p[0]) {
			out = strings.TrimSuffix(out, p[0]) + p[1]
		}
	}
	return out
}
