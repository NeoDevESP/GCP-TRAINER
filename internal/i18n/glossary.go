package i18n

import "strings"

// glossary pairs cloud vocabulary in English with the Spanish forms learners
// actually write. Evidence and justification grading accepts either side, so
// a learner can explain a root cause in Spanish or English and be graded the
// same way. Spanish entries are accent-free because text is normalised first.
var glossary = [][]string{
	{"key", "clave", "llave"},
	{"firewall", "cortafuegos"},
	{"rule", "regla"},
	{"backup", "copia de seguridad", "respaldo", "copia"},
	{"restore", "restaur", "recuper"},
	{"overwrite", "sobrescrib", "machac"},
	{"timeline", "cronologia", "linea temporal"},
	{"impact", "impacto"},
	{"action", "accion"},
	{"prevent", "prevenc", "prevenir", "evitar"},
	{"root cause", "causa raiz"},
	{"cause", "causa"},
	{"permission", "permiso"},
	{"role", "rol"},
	{"service account", "cuenta de servicio"},
	{"identity", "identidad"},
	{"least privilege", "minimo privilegio", "privilegio minimo"},
	{"public", "public"},
	{"private", "privad"},
	{"internet", "internet"},
	{"network", "red"},
	{"subnet", "subred"},
	{"route", "ruta"},
	{"peering", "emparejamiento", "peering"},
	{"overlap", "solap"},
	{"range", "rango"},
	{"address", "direccion"},
	{"port", "puerto"},
	{"load balancer", "balanceador"},
	{"health check", "comprobacion de estado", "chequeo de salud", "health check"},
	{"healthy", "sano", "saludable"},
	{"unhealthy", "no saludable", "insano"},
	{"latency", "latencia"},
	{"error", "error"},
	{"timeout", "tiempo de espera", "timeout"},
	{"memory", "memoria"},
	{"disk", "disco"},
	{"full", "llen"},
	{"storage", "almacenamiento"},
	{"bucket", "bucket", "cubo"},
	{"object", "objeto"},
	{"database", "base de datos"},
	{"replica", "replica"},
	{"failover", "conmutacion", "failover"},
	{"region", "region"},
	{"zone", "zona"},
	{"outage", "caida", "interrupcion"},
	{"availability", "disponibilidad"},
	{"reliability", "fiabilidad"},
	{"cost", "coste", "costo"},
	{"budget", "presupuesto"},
	{"bill", "factura"},
	{"idle", "ocios", "inactiv"},
	{"rightsiz", "dimension"},
	{"label", "etiqueta"},
	{"tag", "etiqueta"},
	{"secret", "secreto"},
	{"password", "contrasena"},
	{"encrypt", "cifr", "encript"},
	{"rotation", "rotacion"},
	{"rotate", "rot"},
	{"audit", "auditor"},
	{"log", "registro", "log"},
	{"metric", "metrica"},
	{"alert", "alerta"},
	{"monitor", "monitoriz", "supervis"},
	{"dashboard", "panel"},
	{"deploy", "despleg", "despliegue"},
	{"release", "version", "lanzamiento", "release"},
	{"rollback", "revert", "vuelta atras", "rollback"},
	{"roll back", "revert", "vuelta atras"},
	{"canary", "canario"},
	{"traffic", "trafico"},
	{"split", "repart", "divi"},
	{"promote", "promocion", "promover"},
	{"image", "imagen"},
	{"container", "contenedor"},
	{"pod", "pod"},
	{"node", "nodo"},
	{"cluster", "cluster", "clúster"},
	{"probe", "sonda"},
	{"readiness", "preparacion", "readiness"},
	{"selector", "selector"},
	{"network policy", "politica de red"},
	{"networkpolicy", "politica de red"},
	{"policy", "politica"},
	{"organization", "organizacion"},
	{"folder", "carpeta"},
	{"project", "proyecto"},
	{"quota", "cuota"},
	{"scale", "escal"},
	{"autoscal", "autoescal"},
	{"instance", "instancia"},
	{"vm", "maquina virtual", "vm"},
	{"snapshot", "instantanea", "snapshot"},
	{"evidence", "evidencia", "prueba"},
	{"forensic", "forense"},
	{"attacker", "atacante"},
	{"compromise", "comprometid", "compromiso"},
	{"leak", "filtr"},
	{"persistence", "persistencia"},
	{"external", "extern"},
	{"exposed", "expuest"},
	{"exposure", "exposicion"},
	{"miner", "minero", "minado"},
	{"delete", "borr", "elimin"},
	{"deleted", "borrad", "eliminad"},
	{"missing", "falt"},
	{"not found", "no encontrad", "no existe"},
	{"migration", "migracion"},
	{"temporary", "temporal"},
	{"since", "desde"},
	{"week", "semana"},
	{"increase", "aument"},
	{"decrease", "reduc", "disminu"},
	{"query", "consulta"},
	{"partition", "particion"},
	{"cluster by", "agrupa", "clustering"},
	{"bytes", "bytes"},
	{"table", "tabla"},
	{"dataset", "conjunto de datos", "dataset"},
	{"model", "modelo"},
	{"endpoint", "endpoint", "punto de conexion"},
	{"prediction", "prediccion"},
	{"drift", "deriva"},
	{"message", "mensaje"},
	{"dead letter", "mensajes fallidos", "dead letter", "dlq"},
	{"subscription", "suscripcion"},
	{"topic", "tema", "topic"},
	{"push", "push", "envio"},
	{"design", "diseno"},
	{"requirement", "requisito"},
	{"hypothesis", "hipotesis"},
	{"verify", "verific", "comprob"},
	{"test", "prueba", "test"},
	{"user", "usuario"},
	{"group", "grupo"},
	{"owner", "propietari"},
	{"team", "equipo"},
	{"customer", "cliente"},
	{"ticket", "ticket", "incidencia"},
	{"update", "actualiz"},
	{"change", "cambio"},
	{"restart", "reinici"},
	{"stop", "deten", "par"},
	{"start", "arranc", "inici"},
	{"schedule", "program"},
	{"permission denied", "permiso denegado"},
	{"denied", "denegad"},
	{"allow", "permit"},
	{"block", "bloque"},
	{"deny", "deneg"},
	{"source", "origen"},
	{"destination", "destino"},
	{"egress", "salida"},
	{"ingress", "entrada"},
	{"certificate", "certificado"},
	{"domain", "dominio"},
	{"dns", "dns"},
	{"record", "registro"},
	{"terraform", "terraform"},
	{"state", "estado"},
	{"drift", "desviacion", "deriva"},
	{"import", "import"},
	{"plan", "plan"},
	{"pipeline", "canalizacion", "pipeline"},
	{"build", "compilacion", "build"},
	{"service", "servicio"},
	{"workload identity", "workload identity", "identidad de carga"},
	{"token", "token"},
	{"short-lived", "corta duracion", "temporal"},
	{"interview", "entrevista"},
}

// index maps every normalised term to all its equivalents.
var index = func() map[string][]string {
	m := map[string][]string{}
	for _, group := range glossary {
		norm := make([]string, len(group))
		for i, t := range group {
			norm[i] = Fold(t)
		}
		for _, t := range norm {
			m[t] = appendUnique(m[t], norm...)
		}
	}
	return m
}()

func appendUnique(dst []string, items ...string) []string {
	for _, it := range items {
		found := false
		for _, d := range dst {
			if d == it {
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, it)
		}
	}
	return dst
}

// Fold lower-cases text and removes Spanish accents so "Rotación" matches
// "rotacion".
func Fold(s string) string {
	return strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
		"Á", "a", "É", "e", "Í", "i", "Ó", "o", "Ú", "u", "Ñ", "n").Replace(strings.ToLower(s))
}

// Variants returns the keyword and its translations (normalised).
func Variants(keyword string) []string {
	k := Fold(keyword)
	if v, ok := index[k]; ok {
		return v
	}
	return []string{k}
}

// ContainsAny reports whether text (any language) mentions the keyword or one
// of its translations. The keyword itself matches anywhere (as authored);
// translations must start a word, so "red" (network) does not match inside
// "credencial".
func ContainsAny(text, keyword string) bool {
	t := Fold(text)
	k := Fold(keyword)
	if strings.Contains(t, k) {
		return true
	}
	for _, v := range Variants(keyword) {
		if v != k && wordStart(t, v) {
			return true
		}
	}
	return false
}

func wordStart(text, term string) bool {
	for i := 0; i+len(term) <= len(text); {
		j := strings.Index(text[i:], term)
		if j < 0 {
			return false
		}
		pos := i + j
		if pos == 0 || !isLetter(text[pos-1]) {
			return true
		}
		i = pos + 1
	}
	return false
}

func isLetter(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b >= 0x80
}
