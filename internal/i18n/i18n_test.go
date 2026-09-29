package i18n

import (
	"net/http/httptest"
	"testing"
)

func TestNormAndPick(t *testing.T) {
	for in, want := range map[string]string{"": ES, "es-ES": ES, "EN": EN, "en-GB": EN, "fr": ES} {
		if got := Norm(in); got != want {
			t.Errorf("Norm(%q)=%q want %q", in, got, want)
		}
	}
	if P(EN, "hola", "hello") != "hello" || P(ES, "hola", "hello") != "hola" || P("", "hola", "hello") != "hola" {
		t.Fatal("P picks the wrong language")
	}
}

func TestFromRequest(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	if FromRequest(r, "") != ES {
		t.Fatal("default must be Spanish")
	}
	r.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if FromRequest(r, "") != EN {
		t.Fatal("Accept-Language en")
	}
	if FromRequest(r, "es") != ES {
		t.Fatal("user preference beats Accept-Language")
	}
	r.Header.Set("X-Lang", "en")
	if FromRequest(r, "es") != EN {
		t.Fatal("explicit header wins")
	}
}

func TestGlossaryBothWays(t *testing.T) {
	cases := []struct {
		text, kw string
		want     bool
	}{
		{"La clave de la cuenta de servicio se filtró en GitHub", "key", true},
		{"The service-account key leaked", "clave", true},
		{"Restauramos la copia de seguridad en una instancia temporal", "backup", true},
		{"Se añadió una regla de cortafuegos", "firewall", true},
		{"La rotación del secreto falló", "rotation", true},
		{"nothing relevant here", "firewall", false},
		{"la credencial caducó", "network", false},
		{"la red no llega", "network", true},
	}
	for _, c := range cases {
		if got := ContainsAny(c.text, c.kw); got != c.want {
			t.Errorf("ContainsAny(%q, %q)=%v", c.text, c.kw, got)
		}
	}
}
