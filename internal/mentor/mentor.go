// Package mentor provides feedback that never replaces deterministic grading:
// a Socratic mentor derived from failing validators, and an optional LLM
// reviewer for postmortems (enabled only when ANTHROPIC_API_KEY or another
// Anthropic credential is configured and MENTOR_LLM=on).
package mentor

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/i18n"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

// Model is the Claude model used for optional postmortem reviews.
const Model = "claude-opus-5-5"

// socratic holds guiding questions per failing check type: {Spanish, English}.
var socratic = map[string][][2]string{
	"tcp": {
		{"¿Desde qué origen exacto hacia qué destino y puerto tiene que fluir el tráfico?", "From which exact source to which destination and port does the traffic need to flow?"},
		{"¿Qué reglas de cortafuegos se aplican a las etiquetas o a la cuenta de servicio del destino, y cuál tiene el número de prioridad más bajo?", "Which firewall rules apply to the destination's tags or service account, and which one has the lowest priority number?"},
		{"¿Puedes demostrar el camino con `nc -zv` desde la VM de origen?", "Can you prove the path with `nc -zv` from the source VM?"},
	},
	"http": {
		{"¿Qué devuelve la petición ahora mismo y desde qué capa (balanceador, servicio, aplicación)?", "What does the request return right now, and from which layer (load balancer, service, application)?"},
		{"¿Qué componente produciría ese código de estado?", "Which component would produce that status code?"},
		{"¿Los registros muestran un error de la aplicación o un tiempo de espera de red?", "Did the logs show an application error or a network timeout?"},
	},
	"egress": {
		{"¿Cómo llega a internet una VM sin IP externa?", "How does a VM without an external IP reach the internet?"},
		{"¿Qué región y subred usa la VM, y qué cubre esa subred?", "Which region and subnet does the VM use, and what covers that subnet?"},
	},
	"google_api": {{"¿Qué caminos existen para llegar a *.googleapis.com desde una subred privada?", "Which paths exist to reach *.googleapis.com from a private subnet?"}},
	"iam": {
		{"¿Qué identidad está usando realmente la carga de trabajo?", "Which identity is the workload really using?"},
		{"¿Cuál es el rol más pequeño que contiene el permiso del mensaje de error?", "Which is the smallest role containing the permission in the error message?"},
		{"¿A qué nivel (recurso, proyecto) debería concederse?", "At which level (resource, project) should it be granted?"},
	},
	"no_basic_roles":  {{"¿Cubriría la tarea un rol predefinido sin Owner/Editor?", "Would a predefined role cover the task without Owner/Editor?"}},
	"forbid_firewall": {{"¿Hay alguna regla más amplia de lo necesario? ¿Quién necesita de verdad llegar a ese puerto?", "Is any rule broader than needed? Who really needs to reach that port?"}},
	"policy":          {{"Haz una revisión de seguridad: ¿qué está expuesto a 0.0.0.0/0 o a allUsers?", "Run a security review: what is exposed to 0.0.0.0/0 or allUsers?"}},
	"evidence":        {{"Explica la cadena causal: síntoma → evidencia → causa → corrección → verificación → prevención.", "Explain the causal chain: symptom → evidence → cause → fix → verification → prevention."}},
	"cost_max":        {{"¿Qué línea de la estimación de coste domina? ¿Hace falta para el requisito?", "Which line of the cost estimate dominates? Is it needed for the requirement?"}},
	"log_metric":      {{"¿Qué filtro exacto aísla los errores que viste en el explorador de registros?", "Which exact filter isolates the errors you saw in Logs Explorer?"}},
	"alert_policy":    {{"¿La alerta salta con un síntoma que ve el usuario, y a quién avisa?", "Does the alert fire on a user-visible symptom, and who gets notified?"}},
	"k8s_ready": {
		{"¿Qué dice `kubectl describe pod` en Events?", "What does `kubectl describe pod` say in Events?"},
		{"¿El puerto de la sonda coincide con el del contenedor?", "Does the probe port match the container port?"},
	},
	"hpa_scaled": {
		{"¿Puede el HPA calcular la utilización sin requests de CPU?", "Can the HPA compute utilisation without CPU requests?"},
		{"¿Generaste carga y después la paraste?", "Did you generate load and then stop it?"},
	},
	"terraform_clean": {{"¿Un segundo `terraform plan` no muestra cambios?", "Does a second `terraform plan` show no changes?"}},
	"bq_bytes_max":    {{"¿Qué columnas y particiones necesita de verdad tu consulta? Prueba con `--dry_run`.", "Which columns and partitions does your query really need? Try `--dry_run`."}},
	"build":           {{"¿Qué identidad ejecuta la compilación y puede publicar en el repositorio?", "Which identity runs the build, and can it push to the repository?"}},
	"sql_healthy":     {{"¿Cuántas conexiones abre cada instancia y cuál es max_connections?", "How many connections does each instance open, and what is max_connections?"}},
	"pubsub":          {{"¿Qué le pasa a un mensaje que falla una y otra vez al entregarse?", "What happens to a message that fails delivery repeatedly?"}},
	"exists":          {{"Compara el recurso que creaste con cada requisito indicado (ubicación, opciones, nombres).", "Compare the resource you created with each stated requirement (location, flags, names)."}},
}

// Socratic returns guiding questions for the validators that are failing,
// without revealing expected values, in the learner's language.
func Socratic(lab *scenario.Lab, res *grader.Result, lang string) []string {
	en := i18n.Norm(lang) == i18n.EN
	seen := map[string]bool{}
	var out []string
	for _, it := range res.Items {
		for _, c := range it.Checks {
			if c.Pass || seen[c.Type] {
				continue
			}
			seen[c.Type] = true
			if qs, ok := socratic[c.Type]; ok {
				q := qs[len(out)%len(qs)]
				text := q[0]
				if en {
					text = q[1]
				}
				out = append(out, fmt.Sprintf("%s — %s", it.Name, text))
			}
		}
	}
	if len(out) == 0 {
		out = append(out, i18n.P(lang, "Todo cuadra. ¿Sabrías explicar por qué tu corrección es la mínima?", "Everything checks out. Can you explain why your fix is the minimal one?"))
	}
	sort.Strings(out)
	return out
}

// Enabled reports whether the optional LLM reviewer is configured.
func Enabled() bool {
	return os.Getenv("MENTOR_LLM") == "on"
}

// ReviewPostmortem asks Claude for qualitative feedback on a postmortem. The
// technical score always comes from the deterministic grader.
func ReviewPostmortem(ctx context.Context, lab *scenario.Lab, evidence map[string]string, lang string) (string, error) {
	if !Enabled() {
		return "", fmt.Errorf("%s", i18n.P(lang, "el mentor con IA está desactivado", "LLM mentor disabled"))
	}
	client := anthropic.NewClient()
	var b strings.Builder
	for _, k := range []string{"rootCause", "fix", "verification", "prevention", "postmortem", "explanation"} {
		if v := evidence[k]; v != "" {
			fmt.Fprintf(&b, "## %s\n%s\n\n", k, v)
		}
	}
	system := "You are a senior SRE mentoring a learner on Google Cloud. Review the postmortem for causal clarity, evidence, verification and prevention. Ask at most three Socratic questions and give at most three concrete improvements. Never give a score and never reveal a full solution."
	if i18n.Norm(lang) == i18n.EN {
		system += " Answer in English."
	} else {
		system += " Responde en español de España, con un tono cercano y profesional."
	}
	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     Model,
		MaxTokens: 16000,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(fmt.Sprintf("Lab: %s\nStory:\n%s\n\nPostmortem:\n%s", lab.Title, lab.Story, b.String()))),
		},
	})
	if err != nil {
		return "", err
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return "", fmt.Errorf("the reviewer declined this request")
	}
	var out strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			out.WriteString(t.Text)
		}
	}
	return out.String(), nil
}
