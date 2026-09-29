package sim

import (
	"fmt"
	"regexp"
)

// "Teach me why" (Blueprint §13): the request path is instrumented so that a
// single request can be explained as a causal chain
//
//	DNS → IP → TCP → TLS → load balancer → firewall → service → workload →
//	application → dependencies (database, APIs, IAM)
//
// Recording is opt-in and never changes behaviour.

// Hop is one link of the causal chain.
type Hop struct {
	Layer  string `json:"layer"`
	Status string `json:"status"` // ok, fail, info
	Detail string `json:"detail"`
}

func (s *State) note(layer, status, format string, a ...any) {
	if s.explain == nil {
		return
	}
	detail := fmt.Sprintf(format, a...)
	if s.explainLang != "en" {
		if es, ok := noteES[format]; ok {
			detail = fmt.Sprintf(es, a...)
		}
		for _, r := range reasonES {
			detail = r.re.ReplaceAllString(detail, r.es)
		}
	}
	*s.explain = append(*s.explain, Hop{Layer: layer, Status: status, Detail: detail})
}

// reasonES rewrites the simulator's network and health reasons (shared with
// the English gcloud output) when they appear inside a Spanish explanation.
var reasonES = []struct {
	re *regexp.Regexp
	es string
}{
	{regexp.MustCompile(`health check probes from 35\.191\.0\.0/16 and 130\.211\.0\.0/22 are blocked: `), "las sondas de comprobación de estado desde 35.191.0.0/16 y 130.211.0.0/22 están bloqueadas: "},
	{regexp.MustCompile(`ingress to (\S+) blocked by (\S+)`), "entrada a $1 bloqueada por $2"},
	{regexp.MustCompile(`egress blocked by firewall rule (\S+)`), "salida bloqueada por la regla de cortafuegos $1"},
	{regexp.MustCompile(`\ballowed by (\S+)`), "permitido por $1"},
	{regexp.MustCompile(`no route from (\S+) to (\S+) \(VPCs are not peered\)`), "no hay ruta de $1 a $2 (las VPC no están emparejadas)"},
	{regexp.MustCompile(`destination instance is not running`), "la instancia de destino no está en ejecución"},
	{regexp.MustCompile(`instance has no external IP address`), "la instancia no tiene IP externa"},
	{regexp.MustCompile(`no default route to the internet gateway for return traffic`), "no hay ruta por defecto a la pasarela de internet para el tráfico de vuelta"},
	{regexp.MustCompile(`health check (\S+) returned HTTP (\d+)`), "la comprobación de estado $1 devolvió HTTP $2"},
	{regexp.MustCompile(`backend service has no health check`), "el servicio de backend no tiene comprobación de estado"},
	{regexp.MustCompile(`instance group has no named port "([^"]*)"`), "el grupo de instancias no tiene el puerto con nombre \"$1\""},
	{regexp.MustCompile(`zone (\S+) unavailable \(outage\)`), "zona $1 no disponible (caída)"},
	{regexp.MustCompile(`Cloud Run service not found`), "no se encuentra el servicio de Cloud Run"},
	{regexp.MustCompile(`\breachable\b`), "accesible"},
}

// noteES translates the causal-chain explanations (keyed by the English
// format string) into Spanish, the platform's primary language.
var noteES = map[string]string{
	"no application is serving requests on %s":              "ninguna aplicación atiende peticiones en %s",
	"%s (%s) crashes at start-up and never listens":         "%s (%s) falla al arrancar y nunca llega a escuchar",
	"%s: handler %s panics (code regression in this image)": "%s: el manejador de %s entra en pánico (regresión de código en esta imagen)",
	"%s handles %s as %s":                                   "%s atiende %s como %s",
	"%s (%s=%q): %s":                                        "%s (%s=%q): %s",
	"%s (%s=%q) reachable and authorised":                   "%s (%s=%q) accesible y autorizado",
	"%s is the Cloud Run URL of service %s (Google front end, TLS terminated by Google)": "%s es la URL de Cloud Run del servicio %s (front end de Google, TLS terminado por Google)",
	"%s does not resolve from %s (no public/private DNS record)":                         "%s no se resuelve desde %s (no hay registro DNS público ni privado)",
	"%s resolves to %s": "%s se resuelve a %s",
	"forwarding rule %s listens on %v, not on port %d":                                                           "la regla de reenvío %s escucha en %v, no en el puerto %d",
	"%s:%d is forwarding rule %s (global external HTTP(S) load balancer, TCP/TLS terminated at the Google edge)": "%s:%d es la regla de reenvío %s (balanceador HTTP(S) externo global; TCP/TLS terminado en el borde de Google)",
	"TCP %s → %s:%d allowed: %s":                                                                                 "TCP %s → %s:%d permitido: %s",
	"TCP %s → %s:%d blocked: %s":                                                                                 "TCP %s → %s:%d bloqueado: %s",
	"zone %s of %s is unavailable (outage)":                                                                      "la zona %s de %s no está disponible (caída)",
	"%s on %s is not running (%s) — nothing accepts connections on port %d":                                      "%s en %s no se está ejecutando (%s): nadie acepta conexiones en el puerto %d",
	"%s on %s listens only on %s:%d — remote clients are refused":                                                "%s en %s solo escucha en %s:%d: se rechazan los clientes remotos",
	"%s listening on %s:%d on %s":                                                                                "%s escuchando en %s:%d en %s",
	"nothing listens on port %d on %s (connection refused)":                                                      "nada escucha en el puerto %d de %s (conexión rechazada)",
	"region %s is unavailable (regional outage): Cloud Run service %s cannot serve":                              "la región %s no está disponible (caída regional): el servicio de Cloud Run %s no puede responder",
	"Cloud Run service %s accepts only internal traffic":                                                         "el servicio de Cloud Run %s solo acepta tráfico interno",
	"%s lacks run.routes.invoke on Cloud Run service %s (no roles/run.invoker for it or allUsers)":               "%s no tiene run.routes.invoke en el servicio de Cloud Run %s (ni él ni allUsers tienen roles/run.invoker)",
	"%s may invoke %s (ingress %s)":                                                                              "%s puede invocar %s (ingress %s)",
	"Cloud Run revision of %s cannot serve: %s":                                                                  "la revisión de Cloud Run de %s no puede responder: %s",
	"revision of %s runs %s as %s":                                                                               "la revisión de %s ejecuta %s como %s",
	"the URL map sends %s%s to %q which does not exist":                                                          "el mapa de URL envía %s%s a %q, que no existe",
	"URL map routes %s%s to backend service %s":                                                                  "el mapa de URL dirige %s%s al servicio de backend %s",
	"security policy %s rule %s matched source %s → %s":                                                          "la política de seguridad %s, regla %s, coincide con el origen %s → %s",
	"serverless NEG %s (%s) skipped: region unavailable":                                                         "NEG sin servidor %s (%s) omitido: región no disponible",
	"%s is HEALTHY":                                  "%s está SANO (HEALTHY)",
	"%s is UNHEALTHY: %s":                            "%s NO está sano (UNHEALTHY): %s",
	"backend service %s has no backends":             "el servicio de backend %s no tiene backends",
	"load balancer proxies to %s:%d (named port %q)": "el balanceador reenvía a %s:%d (puerto con nombre %q)",
}

// ExplainHTTP executes a request and returns the causal chain it followed.
// lang selects the language of the explanations (es primary, en).
func (s *State) ExplainHTTP(req HTTPRequest, lang string) (HTTPResponse, []Hop) {
	var hops []Hop
	prev, prevLang := s.explain, s.explainLang
	s.explain, s.explainLang = &hops, lang
	defer func() { s.explain, s.explainLang = prev, prevLang }()
	r := s.HTTP(req)
	switch {
	case r.Error != "":
		s.note("result", "fail", "%s", r.Error)
	case r.Status >= 400:
		s.note("result", "fail", "HTTP %d: %s", r.Status, firstLine(r.Body))
	default:
		s.note("result", "ok", "HTTP %d", r.Status)
	}
	return r, hops
}

func firstLine(s string) string {
	for i := range s {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}

// RegionDown reports a simulated regional outage (DR drills, chaos).
func (s *State) RegionDown(region string) bool {
	return region != "" && s.Extra["outage:"+region] == "down"
}

// ZoneDown reports a zonal outage (explicit zone or its region).
func (s *State) ZoneDown(zone string) bool {
	return zone != "" && (s.Extra["outage:"+zone] == "down" || s.RegionDown(RegionOf(zone)))
}
