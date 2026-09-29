package api

import "testing"

func TestLocalizeError(t *testing.T) {
	for in, want := range map[string]string{
		"invalid credentials":                         "credenciales no válidas",
		"lab ace-x not found":                         "laboratorio ace-x no encontrado",
		"mission m1 is not available":                 "misión m1 no está disponible",
		"unauthenticated: token expired":              "no autenticado: el token ha caducado",
		"se obtuvo 1, se esperaba 2":                  "se obtuvo 1, se esperaba 2",
		"monthly real-GCP quota reached (5 sessions)": "se ha alcanzado la cuota mensual de GCP real (5 sesiones)",
	} {
		if got := localizeError("es", in); got != want {
			t.Errorf("localizeError(%q) = %q, want %q", in, got, want)
		}
	}
	if got := localizeError("en", "invalid credentials"); got != "invalid credentials" {
		t.Errorf("english must pass through, got %q", got)
	}
}
