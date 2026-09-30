// Package tutor turns a lab's official solution into a guided walkthrough:
// each command becomes a step with a title, why it matters, where it lives
// in the Google Cloud console and the equivalent command, plus a pattern the
// client uses to notice when the learner has done it (in the console or in
// Cloud Shell). Labs can add their own explanations with tutor notes.
package tutor

import (
	"regexp"
	"strings"
)

// Phases of a walkthrough.
const (
	Setup       = "setup"
	Investigate = "investigate"
	Fix         = "fix"
	Verify      = "verify"
	Communicate = "communicate"
)

// Where is a place in the console.
type Where struct {
	Page   string `json:"page,omitempty"`   // console page id; empty = Cloud Shell only
	Path   string `json:"path"`             // breadcrumb shown to the learner
	Action string `json:"action,omitempty"` // create, row, delete, edit, shell, panel
	Target string `json:"target,omitempty"` // row name or button text to highlight
}

// Step is one step of the walkthrough.
type Step struct {
	N       int     `json:"n"`
	Phase   string  `json:"phase"`
	Title   string  `json:"title"`
	Why     string  `json:"why"`
	Concept string  `json:"concept,omitempty"`
	Note    string  `json:"note,omitempty"` // lab-specific explanation
	Where   Where   `json:"where"`
	Command string  `json:"command"`
	Fields  []Field `json:"fields,omitempty"` // console form fields to fill in
	Match   string  `json:"match,omitempty"`  // JavaScript-compatible regex over executed lines
	Manual  bool    `json:"manual,omitempty"`
}

// Note is a lab-specific explanation attached to the step whose command
// matches Match. "@intro" sets the introduction and "@fix" attaches to the
// first change of the walkthrough.
type Note struct {
	Match string
	Title string
	Why   string
}

// Plan is the whole walkthrough.
type Plan struct {
	Intro string `json:"intro,omitempty"`
	Steps []Step `json:"steps"`
}

// Build creates the walkthrough of a solution script.
func Build(solution string, notes []Note, lang string) Plan {
	en := strings.HasPrefix(strings.ToLower(lang), "en")
	var plan Plan
	fixed := false
	for _, b := range splitScript(solution) {
		st, ok := explain(b.Text, en)
		if !ok {
			continue
		}
		if b.Comment != "" && st.Note == "" {
			st.Note = b.Comment
		}
		// read-only checks after the first change are verification
		if st.Phase == Investigate && fixed {
			st.Phase = Verify
		}
		if st.Phase == Fix {
			fixed = true
		}
		st.N = len(plan.Steps) + 1
		plan.Steps = append(plan.Steps, st)
	}
	for _, n := range notes {
		switch n.Match {
		case "@intro":
			plan.Intro = strings.TrimSpace(strings.Join([]string{plan.Intro, n.Why}, "\n\n"))
			continue
		case "@fix":
			for i := range plan.Steps {
				if plan.Steps[i].Phase == Fix {
					attach(&plan.Steps[i], n)
					break
				}
			}
			continue
		}
		re, err := regexp.Compile(n.Match)
		if err != nil {
			continue
		}
		for i := range plan.Steps {
			if re.MatchString(plan.Steps[i].Command) {
				attach(&plan.Steps[i], n)
				break
			}
		}
	}
	return plan
}

func attach(s *Step, n Note) {
	if n.Title != "" {
		s.Title = n.Title
	}
	if n.Why != "" {
		s.Note = strings.TrimSpace(strings.Join([]string{s.Note, n.Why}, " "))
	}
}

var reAssign = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)=`)

// explain turns one command into a step.
func explain(cmd string, en bool) (Step, bool) {
	L := func(es, e string) string {
		if en {
			return e
		}
		return es
	}
	fails := strings.HasPrefix(cmd, "!")
	cmd = strings.TrimPrefix(cmd, "!")
	st, ok := explainCmd(cmd, en)
	if ok && fails {
		st.Note = L("Este paso falla a propósito: lee el mensaje de error, es la pista para seguir.", "This step fails on purpose: read the error message, it is the clue to continue.")
	}
	return st, ok
}

func explainCmd(cmd string, en bool) (Step, bool) {
	L := func(es, e string) string {
		if en {
			return e
		}
		return es
	}
	first := strings.SplitN(cmd, "\n", 2)[0]
	ws := words(first)
	for len(ws) > 0 && reAssign.MatchString(ws[0]) && len(ws) > 1 {
		ws = ws[1:] // VAR=value command
	}
	if len(ws) == 0 {
		return Step{}, false
	}
	tool := strings.TrimPrefix(ws[0], "!")
	st := Step{Command: cmd, Phase: Fix}
	shell := Where{Path: "Cloud Shell", Action: "shell"}
	switch {
	case tool == "sleep" || tool == "set" || tool == "export" && len(ws) == 1:
		return Step{}, false
	case reAssign.MatchString(ws[0]):
		m := reAssign.FindStringSubmatch(ws[0])
		st.Phase = Setup
		st.Title = L("Guarda "+m[1]+" para los siguientes comandos", "Store "+m[1]+" for the next commands")
		st.Why = L("Una variable evita copiar a mano valores largos (IDs, IPs, URLs) y equivocarse.", "A variable avoids copying long values (IDs, IPs, URLs) by hand and making mistakes.")
		st.Where = shell
		st.Manual = true
		return st, true
	case tool == "gcloud":
		return gcloudStep(st, ws, en), true
	case tool == "gsutil":
		st.Where = Where{Page: "buckets", Path: L("Cloud Storage › Buckets", "Cloud Storage › Buckets")}
		st.Title = L("Trabaja con Cloud Storage (gsutil "+arg(ws, 1)+")", "Work with Cloud Storage (gsutil "+arg(ws, 1)+")")
		st.Why = L("gsutil es la herramienta clásica de Cloud Storage; hoy se prefiere gcloud storage.", "gsutil is the classic Cloud Storage tool; gcloud storage is preferred today.")
		st.Match = prefixMatch(ws[:min(2, len(ws))])
		readVerb(&st, arg(ws, 1))
	case tool == "bq":
		sub := arg(ws, 1)
		st.Where = Where{Page: "bigquery", Path: L("BigQuery › Estudio de BigQuery", "BigQuery › BigQuery Studio"), Action: "shell"}
		st.Match = prefixMatch(ws[:min(2, len(ws))])
		switch sub {
		case "query":
			st.Phase = Investigate
			st.Title = L("Consulta los datos en BigQuery", "Query the data in BigQuery")
			st.Why = L("En el Estudio de BigQuery puedes pegar la misma consulta y ver el resultado en una tabla. Revisa cuántos bytes lee: es lo que se paga.", "In BigQuery Studio you can paste the same query and see the result as a table. Check how many bytes it reads: that is what you pay for.")
			if isDML(first) {
				st.Phase = Fix
				st.Title = L("Modifica los datos de BigQuery", "Change the BigQuery data")
			}
		case "mk":
			st.Title = L("Crea el conjunto de datos o la tabla", "Create the dataset or table")
			st.Why = L("En BigQuery los datos se organizan en conjuntos de datos (con ubicación) y tablas (con esquema).", "In BigQuery data is organised in datasets (with a location) and tables (with a schema).")
		case "load":
			st.Title = L("Carga datos en la tabla", "Load data into the table")
			st.Why = L("Cargar desde Cloud Storage es gratis; se paga el almacenamiento y las consultas.", "Loading from Cloud Storage is free; you pay for storage and queries.")
		default:
			st.Title = L("Trabaja con BigQuery (bq "+sub+")", "Work with BigQuery (bq "+sub+")")
			st.Why = L("bq es la herramienta de línea de comandos de BigQuery.", "bq is BigQuery's command-line tool.")
			readVerb(&st, sub)
		}
	case tool == "kubectl":
		kubectlStep(&st, ws, en)
	case tool == "terraform":
		sub := arg(ws, 1)
		st.Where = shell
		st.Match = prefixMatch(ws[:min(2, len(ws))])
		st.Title = "terraform " + sub
		switch sub {
		case "init":
			st.Phase = Setup
			st.Why = L("Descarga los proveedores y prepara el backend donde se guarda el estado.", "Downloads the providers and prepares the backend where state is kept.")
		case "plan":
			st.Phase = Investigate
			st.Why = L("Muestra qué cambiaría sin tocar nada: léelo siempre antes de aplicar. Busca «destroy».", "Shows what would change without touching anything: always read it before applying. Look for \"destroy\".")
		case "apply":
			st.Why = L("Aplica el plan: crea, cambia o borra recursos para que coincidan con el código.", "Applies the plan: creates, changes or deletes resources to match the code.")
		case "import":
			st.Why = L("Adopta en el estado un recurso que ya existe, para que Terraform no intente recrearlo.", "Adopts an existing resource into state so Terraform does not try to recreate it.")
		default:
			st.Why = L("Terraform describe la infraestructura como código.", "Terraform describes infrastructure as code.")
		}
	case tool == "curl" || tool == "wget":
		st.Phase = Verify
		st.Title = L("Prueba el servicio como lo haría un cliente", "Test the service as a customer would")
		st.Why = L("Comprobar desde fuera demuestra que funciona de verdad, no solo que la configuración parece correcta. Fíjate en el código HTTP.", "Checking from outside proves it really works, not just that the configuration looks right. Watch the HTTP status code.")
		st.Where = shell
		st.Match = "^(curl|wget)\\b"
		if strings.Contains(first, "firestore.googleapis.com") && (strings.Contains(first, "-X POST") || strings.Contains(first, "-X PATCH")) {
			st.Phase = Fix
			st.Title = L("Escribe el documento con la API REST", "Write the document through the REST API")
			st.Why = L("Las APIs de Google se llaman por HTTPS con un token de acceso; la consola y gcloud hacen lo mismo por debajo.", "Google APIs are called over HTTPS with an access token; the console and gcloud do the same under the hood.")
			st.Match = "^curl\\b.*-X (POST|PATCH).*firestore"
		}
	case tool == "ticket":
		st.Phase = Communicate
		st.Where = Where{Path: L("Panel del laboratorio › Ticket", "Lab panel › Ticket"), Action: "panel", Target: "desk"}
		st.Match = prefixMatch(ws[:min(2, len(ws))])
		if arg(ws, 1) == "resolve" {
			st.Title = L("Cierra el ticket explicando la solución", "Resolve the ticket explaining the fix")
			st.Why = L("Cerrar bien un incidente es decir qué pasó, qué se hizo y cómo se comprobó.", "Closing an incident well means saying what happened, what was done and how it was checked.")
		} else {
			st.Title = L("Informa al negocio en el ticket", "Update the business on the ticket")
			st.Why = L("Durante un incidente, informar pronto (qué pasa, impacto, siguiente paso y cuándo vuelves a informar) es parte del trabajo.", "During an incident, updating early (what is happening, impact, next step and when you will update again) is part of the job.")
		}
	case tool == "ask":
		st.Phase = Investigate
		st.Where = Where{Path: L("Panel del laboratorio › Ticket › Preguntar", "Lab panel › Ticket › Ask"), Action: "panel", Target: "desk"}
		st.Title = L("Pregunta a "+arg(ws, 1), "Ask "+arg(ws, 1))
		st.Why = L("Las personas implicadas saben cosas que no están en los registros: qué cambió, cuándo y por qué.", "The people involved know things that are not in the logs: what changed, when and why.")
		st.Match = "^ask\\s+" + regexp.QuoteMeta(arg(ws, 1))
	case tool == "cat" && strings.Contains(first, "<<"):
		file := redirectTarget(first)
		st.Phase = Setup
		st.Title = L("Crea el archivo "+file, "Create the file "+file)
		st.Why = L("Pega el contenido en Cloud Shell o créalo con «Abrir editor». Los siguientes comandos lo leen.", "Paste the content in Cloud Shell or create it with \"Open editor\". The next commands read it.")
		st.Where = Where{Path: L("Cloud Shell › Abrir editor", "Cloud Shell › Open editor"), Action: "shell"}
		st.Manual = true
		if file != "" {
			st.Match = "^cat\\b.*" + regexp.QuoteMeta(file)
		}
	case tool == "for" || tool == "while" || tool == "if":
		st.Title = L("Ejecuta este pequeño script", "Run this small script")
		st.Why = L("Repite el mismo comando sobre varios recursos. Puedes pegarlo tal cual en Cloud Shell.", "Repeats the same command over several resources. You can paste it as is into Cloud Shell.")
		st.Where = shell
		st.Manual = true
		if strings.Contains(cmd, "curl") {
			st.Phase = Verify
		}
	case tool == "git" || tool == "docker":
		st.Title = tool + " " + arg(ws, 1)
		st.Why = L("Se ejecuta en Cloud Shell, igual que en tu ordenador.", "It runs in Cloud Shell, just like on your computer.")
		st.Where = shell
		st.Match = prefixMatch(ws[:min(2, len(ws))])
	case tool == "psql" || tool == "mysql" || tool == "redis-cli":
		st.Phase = Verify
		st.Title = L("Conéctate con "+tool, "Connect with "+tool)
		st.Why = L("Probar la conexión con el cliente real demuestra que red, usuario y contraseña están bien.", "Testing with the real client proves network, user and password are right.")
		st.Where = shell
		st.Match = "^" + tool + "\\b"
	case tool == "echo" || tool == "printf":
		if strings.Contains(first, ">") {
			st.Title = L("Escribe el archivo "+redirectTarget(first), "Write the file "+redirectTarget(first))
			st.Phase = Setup
			st.Where = shell
			st.Manual = true
			st.Why = L("Los siguientes comandos leen este archivo.", "The next commands read this file.")
			return st, true
		}
		return Step{}, false
	default:
		st.Title = L("Ejecuta: ", "Run: ") + strings.Join(ws[:min(3, len(ws))], " ")
		st.Why = L("Comando de la plataforma o del sistema; escríbelo en Cloud Shell.", "Platform or system command; type it in Cloud Shell.")
		st.Where = shell
		st.Match = prefixMatch(ws[:min(2, len(ws))])
		readVerb(&st, arg(ws, 1))
	}
	return st, true
}

func arg(ws []string, i int) string {
	if i < len(ws) {
		return ws[i]
	}
	return ""
}

func isDML(s string) bool {
	u := strings.ToUpper(s)
	for _, k := range []string{"INSERT ", "UPDATE ", "DELETE ", "MERGE ", "CREATE ", "DROP "} {
		if strings.Contains(u, k) {
			return true
		}
	}
	return false
}

var reRedirect = regexp.MustCompile(`>\s*([^\s<>|;&]+)`)

func redirectTarget(s string) string {
	if m := reRedirect.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// prefixMatch builds a regex matching a command that starts with these words.
func prefixMatch(ws []string) string {
	q := make([]string, 0, len(ws))
	for _, w := range ws {
		if strings.ContainsAny(w, "$`") {
			break
		}
		q = append(q, regexp.QuoteMeta(w))
	}
	if len(q) == 0 {
		return ""
	}
	return "^" + strings.Join(q, "\\s+") + "(\\s|$)"
}

var readVerbs = map[string]bool{
	"list": true, "describe": true, "get": true, "read": true, "logs": true, "cat": true, "ls": true, "show": true, "estimate": true,
	"report": true, "get-health": true, "get-iam-policy": true, "get-serial-port-output": true, "troubleshoot-policy": true, "top": true,
	"list-configs": true, "search-all-resources": true, "get-ancestors": true, "get-value": true, "browse": true, "list-grantable-roles": true,
	"get-named-ports": true, "print-access-token": true, "print-identity-token": true, "history": true,
}

func readVerb(st *Step, verb string) {
	if readVerbs[verb] {
		st.Phase = Investigate
	}
}

// gcloudStep explains a gcloud command.
func gcloudStep(st Step, ws []string, en bool) Step {
	L := func(es, e string) string {
		if en {
			return e
		}
		return es
	}
	p := parse(ws[1:])
	pos := p.Pos
	for len(pos) > 0 && (pos[0] == "alpha" || pos[0] == "beta") {
		pos = pos[1:]
	}
	// longest known resource group
	key, n := "", 0
	for k := min(4, len(pos)); k >= 1; k-- {
		if _, ok := resources[strings.Join(pos[:k], " ")]; ok {
			key, n = strings.Join(pos[:k], " "), k
			break
		}
	}
	if key == "" && len(pos) > 0 {
		key, n = pos[0], 1
	}
	r, known := resources[key]
	verb := arg(pos, n)
	name := arg(pos, n+1)
	if strings.ContainsAny(name, "$`") {
		name = ""
	}
	// the matcher: group words + verb (+ resource name), with the IAM
	// member and role as lookaheads so each binding is its own step
	var q []string
	for _, w := range pos[:min(n+1, len(pos))] {
		q = append(q, regexp.QuoteMeta(w))
	}
	base := "gcloud\\s+(?:(?:alpha|beta)\\s+)?" + strings.Join(q, "\\s+")
	if name != "" {
		base += "\\s+" + regexp.QuoteMeta(name)
	}
	base += "(\\s|$)"
	look := ""
	for _, f := range []string{"member", "role"} {
		if v := p.Flags[f]; v != "" && !strings.ContainsAny(v, "$`") {
			look += "(?=.*" + regexp.QuoteMeta(v) + ")"
		}
	}
	st.Match = "^" + look + base
	st.Where = Where{Page: r.page, Path: L(r.pathES, r.pathEN)}
	st.Fields = fieldsOf(p.Flags, en)
	if r.page == "" {
		st.Where.Path = "Cloud Shell"
		st.Where.Action = "shell"
	}
	one, many := L(r.oneES, r.oneEN), L(r.manyES, r.manyEN)
	if !known {
		one, many = key, key
	}
	st.Concept = L(r.whatES, r.whatEN)
	named := func(s string) string {
		if name == "" {
			return s
		}
		if en {
			return s + " “" + name + "”"
		}
		return s + " «" + name + "»"
	}
	role, member := p.Flags["role"], p.Flags["member"]
	switch {
	// special commands first
	case key == "compute" && verb == "ssh" || key == "compute ssh":
		st.Phase = Investigate
		vm := arg(pos, 2)
		st.Title = L("Entra por SSH en la VM «"+vm+"»", "SSH into the VM “"+vm+"”")
		st.Why = L("Dentro de la VM ves lo que la consola no enseña: servicios, disco, configuración y registros del sistema.", "Inside the VM you see what the console does not show: services, disk, configuration and system logs.")
		st.Concept = L("En la lista de VMs, el botón SSH abre la misma sesión. Con --tunnel-through-iap entras sin exponer el puerto 22 a internet.", "In the VM list the SSH button opens the same session. With --tunnel-through-iap you get in without exposing port 22 to the internet.")
		st.Where = Where{Page: "vm", Path: L("Compute Engine › Instancias de VM › SSH", "Compute Engine › VM instances › SSH"), Action: "row", Target: vm}
		st.Match = "^gcloud\\s+compute\\s+ssh\\s+" + regexp.QuoteMeta(vm) + "(\\s|$)"
		if c := p.Flags["command"]; c != "" && !readish(c) {
			st.Phase = Fix
		}
		return st
	case key == "logging" && verb == "read":
		st.Phase = Investigate
		st.Title = L("Busca en los registros", "Search the logs")
		if f := arg(pos, n+1); f != "" {
			st.Title += L(": "+short(f), ": "+short(f))
		}
		st.Why = L("Los registros cuentan qué ha pasado de verdad. Filtra por gravedad, recurso o método (registros de auditoría = quién hizo qué).", "Logs tell what really happened. Filter by severity, resource or method (audit logs = who did what).")
		st.Where = Where{Page: "logs", Path: L("Logging › Explorador de registros", "Logging › Logs Explorer")}
		st.Match = "^gcloud\\s+logging\\s+read\\b"
		return st
	case key == "services" && (verb == "enable" || verb == "disable"):
		st.Title = L("Activa la API ", "Enable the API ") + strings.Join(pos[n+1:], ", ")
		if verb == "disable" {
			st.Title = L("Desactiva la API ", "Disable the API ") + strings.Join(pos[n+1:], ", ")
		}
		st.Why = L("En Google Cloud cada servicio se activa por proyecto antes de usarlo; si no, los comandos fallan con «API not enabled».", "In Google Cloud each service is enabled per project before use; otherwise commands fail with \"API not enabled\".")
		st.Where = Where{Page: "apis", Path: L("APIs y servicios", "APIs & Services")}
		st.Phase = Setup
		st.Match = "^gcloud\\s+services\\s+" + verb + "\\b"
		return st
	case key == "config":
		st.Phase = Setup
		st.Title = L("Configura gcloud: ", "Configure gcloud: ") + strings.Join(pos[n:], " ")
		st.Why = L("Así no tienes que repetir proyecto, región o zona en cada comando. La consola usa el proyecto del selector de arriba.", "So you do not repeat project, region or zone on every command. The console uses the project picker at the top.")
		return st
	case (key == "container clusters") && verb == "get-credentials":
		st.Phase = Setup
		st.Title = L("Conecta kubectl al clúster «"+name+"»", "Connect kubectl to the cluster “"+name+"”")
		st.Why = L("Descarga las credenciales para que kubectl hable con este clúster. En la consola es el botón «Conectar».", "Downloads the credentials so kubectl talks to this cluster. In the console it is the \"Connect\" button.")
		st.Where.Action, st.Where.Target = "row", name
		return st
	case (key == "run" && verb == "deploy") || (key == "functions" && verb == "deploy") || (key == "app" && verb == "deploy"):
		st.Title = L("Despliega ", "Deploy ") + named(one)
		if key == "app" {
			st.Title = L("Despliega una versión de App Engine", "Deploy an App Engine version")
			if _, ok := p.Flags["no-promote"]; ok {
				st.Title += L(" sin darle tráfico", " without giving it traffic")
			}
		}
		st.Why = L("Desplegar sube el código o la imagen y crea una revisión nueva. Revisa región, cuenta de servicio y si debe ser público.", "Deploying uploads the code or image and creates a new revision. Check region, service account and whether it must be public.")
		st.Where.Action = "create"
		return st
	}
	switch {
	case verb == "create" || verb == "deploy" || verb == "upload" || verb == "submit" || verb == "run" || verb == "connect" || verb == "import" || verb == "export" || verb == "publish" || verb == "restore":
		st.Title = L(capital(verbES(verb))+" ", capital(verb)+" ") + named(one)
		st.Why = L("Crea exactamente lo que pide la misión: nombre, región y opciones importan, porque la evaluación comprueba la configuración.", "Create exactly what the mission asks for: name, region and options matter, because grading checks the configuration.")
		st.Where.Action = "create"
		if verb == "publish" {
			st.Phase = Verify
			st.Where.Action = "row"
			st.Where.Target = name
		}
		if verb == "export" || verb == "restore" || verb == "import" {
			st.Where.Action, st.Where.Target = "row", name
		}
	case verb == "delete":
		st.Title = L("Elimina ", "Delete ") + named(one)
		st.Why = L("Lo que sobra cuesta dinero o es una puerta abierta. Antes de borrar, asegúrate de que nadie lo usa.", "Leftovers cost money or leave a door open. Before deleting, make sure nobody uses it.")
		st.Where.Action, st.Where.Target = "row", name
	case verb == "list":
		st.Phase = Investigate
		st.Title = L("Revisa ", "Review ") + many
		st.Why = L("Antes de cambiar nada, mira qué existe y en qué estado está: así sabes de dónde partes.", "Before changing anything, look at what exists and in what state: that is your starting point.")
	case verb == "describe" || strings.HasPrefix(verb, "get-") || verb == "get" || verb == "list-configs":
		st.Phase = Investigate
		st.Title = L("Mira el detalle de ", "Look at the details of ") + named(one)
		if verb == "get-iam-policy" {
			st.Title = L("Mira quién tiene acceso a ", "See who has access to ") + named(one)
		}
		if verb == "get-health" {
			st.Title = L("Mira si los backends están sanos", "Check whether the backends are healthy")
		}
		st.Why = L("En el detalle está la configuración exacta; muchas causas se ven aquí (redes, etiquetas, puertos, cuentas).", "The details show the exact configuration; many causes show up here (networks, tags, ports, accounts).")
		st.Where.Action, st.Where.Target = "row", name
	case verb == "add-iam-policy-binding" || verb == "add-invoker-policy-binding":
		if role == "" {
			role = "roles/run.invoker"
		}
		st.Title = L("Da el rol "+role+" a "+member, "Grant "+role+" to "+member)
		if name != "" {
			st.Title += L(" en ", " on ") + named(one)
		}
		st.Why = L("IAM = quién (principal) puede hacer qué (rol) sobre qué recurso. Da el rol más pequeño que sirva y en el recurso concreto, no en todo el proyecto.", "IAM = who (principal) can do what (role) on which resource. Grant the smallest role that works, on the specific resource, not the whole project.")
		st.Where.Action, st.Where.Target = "grant", name
		if key == "projects" {
			st.Where.Target = ""
		}
	case verb == "remove-iam-policy-binding":
		st.Title = L("Quita el rol "+role+" a "+member, "Remove "+role+" from "+member)
		st.Why = L("Mínimo privilegio: retira los permisos que sobran, sobre todo roles básicos (Owner, Editor) o acceso público (allUsers).", "Least privilege: remove permissions nobody needs, above all basic roles (Owner, Editor) or public access (allUsers).")
		st.Where.Action, st.Where.Target = "row", name
		if key == "projects" {
			st.Where.Target = member
		}
	case verb == "start" || verb == "stop" || verb == "reset" || verb == "resume" || verb == "pause" || verb == "cancel" || verb == "drain":
		st.Title = L(capital(verbES(verb))+" ", capital(verb)+" ") + named(one)
		st.Why = L("Cambia el estado sin borrar nada: se puede deshacer.", "Changes the state without deleting anything: it can be undone.")
		st.Where.Action, st.Where.Target = "row", name
	default:
		st.Title = L("Cambia ", "Change ") + named(one)
		if len(st.Fields) > 0 {
			var parts []string
			for _, f := range st.Fields[:min(2, len(st.Fields))] {
				parts = append(parts, f.Label+" → "+f.Value)
			}
			st.Title += ": " + strings.Join(parts, ", ")
		} else {
			st.Title += " (" + verb + ")"
		}
		st.Why = L("Modifica solo lo necesario. En la consola está en el detalle del recurso, botón «Editar».", "Change only what is needed. In the console it is in the resource details, \"Edit\" button.")
		st.Where.Action, st.Where.Target = "edit", name
		if readVerbs[verb] {
			st.Phase = Investigate
			st.Title = L("Consulta ", "Check ") + named(one) + " (" + verb + ")"
		}
	}
	if st.Where.Page == "" {
		st.Where.Action = "shell"
	}
	return st
}

func readish(c string) bool {
	f := firstWord(strings.TrimPrefix(strings.TrimSpace(c), "sudo "))
	switch f {
	case "cat", "ls", "df", "du", "systemctl", "journalctl", "tail", "head", "grep", "curl", "ps", "free", "top", "ss", "netstat", "hostname", "ip", "dig", "ping", "nc", "ncat", "telnet", "nslookup", "wget", "redis-cli", "psql", "mysql", "env", "id", "whoami", "uptime", "mount", "lsblk", "findmnt", "stat", "getent":
		return !strings.Contains(c, "restart") && !strings.Contains(c, "start ") && !strings.Contains(c, "enable")
	}
	return false
}

func short(s string) string {
	if len(s) > 60 {
		return s[:57] + "…"
	}
	return s
}

func capital(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func verbES(v string) string {
	m := map[string]string{
		"create": "crea", "deploy": "despliega", "upload": "sube", "submit": "envía", "run": "ejecuta", "connect": "conecta",
		"import": "importa", "export": "exporta", "publish": "publica un mensaje en", "restore": "restaura", "start": "arranca",
		"stop": "detén", "reset": "reinicia", "resume": "reanuda", "pause": "pausa", "cancel": "cancela", "drain": "drena",
	}
	if x, ok := m[v]; ok {
		return x
	}
	return v
}

// kubectlStep explains a kubectl command.
func kubectlStep(st *Step, ws []string, en bool) {
	L := func(es, e string) string {
		if en {
			return e
		}
		return es
	}
	p := parse(ws[1:])
	verb := arg(p.Pos, 0)
	kind := arg(p.Pos, 1)
	st.Match = "^kubectl\\s+(?:-\\S+\\s+\\S+\\s+)*" + regexp.QuoteMeta(verb) + "(\\s|$)"
	st.Where = Where{Page: "workloads", Path: L("Kubernetes Engine › Cargas de trabajo", "Kubernetes Engine › Workloads")}
	if strings.HasPrefix(kind, "svc") || strings.HasPrefix(kind, "service") || strings.HasPrefix(kind, "ing") {
		st.Where = Where{Page: "k8sservices", Path: L("Kubernetes Engine › Servicios e Ingress", "Kubernetes Engine › Services & Ingress")}
	}
	st.Concept = L("kubectl habla con el clúster de GKE; la consola enseña lo mismo en Cargas de trabajo y Servicios.", "kubectl talks to the GKE cluster; the console shows the same under Workloads and Services.")
	switch verb {
	case "get", "describe", "logs", "top", "events", "explain", "auth":
		st.Phase = Investigate
		st.Title = L("Mira el estado en Kubernetes (kubectl "+verb+" "+kind+")", "Check the state in Kubernetes (kubectl "+verb+" "+kind+")")
		st.Why = L("describe y los eventos suelen decir por qué un pod no arranca (imagen, recursos, sondas, permisos).", "describe and the events usually say why a pod does not start (image, resources, probes, permissions).")
		if verb == "logs" {
			st.Why = L("Los registros del contenedor muestran el error de la aplicación.", "The container logs show the application error.")
		}
	case "apply", "create":
		st.Title = L("Aplica el manifiesto en el clúster", "Apply the manifest to the cluster")
		st.Why = L("Kubernetes es declarativo: describes lo que quieres en YAML y el clúster lo hace realidad.", "Kubernetes is declarative: you describe what you want in YAML and the cluster makes it happen.")
	case "rollout":
		st.Title = L("Controla el despliegue (kubectl rollout "+kind+")", "Control the rollout (kubectl rollout "+kind+")")
		st.Why = L("rollout status espera a que termine; rollout undo vuelve a la versión anterior.", "rollout status waits for it to finish; rollout undo goes back to the previous version.")
		if kind == "status" || kind == "history" {
			st.Phase = Investigate
		}
	case "set", "scale", "patch", "edit", "label", "annotate", "delete", "autoscale", "expose":
		st.Title = L("Cambia el recurso de Kubernetes (kubectl "+verb+")", "Change the Kubernetes resource (kubectl "+verb+")")
		st.Why = L("Cambio directo sobre el clúster; en equipos reales se hace en el YAML y se vuelve a aplicar.", "A direct change to the cluster; real teams change the YAML and apply it again.")
	default:
		st.Title = "kubectl " + verb
		st.Why = L("Comando de Kubernetes.", "Kubernetes command.")
	}
}

var reLook = regexp.MustCompile(`^\^?\(\?=\.\*((?:\\.|[^)\\])*)\)`)

// MatchLine evaluates a step pattern (which may start with lookaheads, not
// supported by Go's regexp) against an executed command line.
func MatchLine(pattern, line string) bool {
	if pattern == "" {
		return false
	}
	p := pattern
	for {
		m := reLook.FindStringSubmatch(p)
		if m == nil {
			break
		}
		re, err := regexp.Compile(m[1])
		if err != nil || !re.MatchString(line) {
			return false
		}
		p = "^" + strings.TrimPrefix(p[len(m[0]):], "^")
	}
	re, err := regexp.Compile(p)
	return err == nil && re.MatchString(strings.TrimSpace(line))
}
