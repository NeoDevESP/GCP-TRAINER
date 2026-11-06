package scenario

import "strings"

// The free console (sandbox): the same simulated Google Cloud as the labs —
// console and Cloud Shell on one shared state — with no mission, no faults,
// no grading and no hints. The learner is project owner and starts from an
// empty project or from one of the worlds the labs use.

// SandboxPrefix starts the lab id of every sandbox ("sandbox-<world>").
const SandboxPrefix = "sandbox-"

// SandboxProject is the project id of every sandbox, so a saved world can be
// continued later (its state is stored under this project).
const SandboxProject = "mi-sandbox"

// SandboxWorld is a starting point for the free console.
type SandboxWorld struct {
	ID       string   `json:"id"`
	Baseline []string `json:"-"`
	NameES   string   `json:"-"`
	NameEN   string   `json:"-"`
	DescES   string   `json:"-"`
	DescEN   string   `json:"-"`
}

// SandboxWorlds are the worlds offered, in order.
var SandboxWorlds = []SandboxWorld{
	{ID: "empty", Baseline: []string{"empty"},
		NameES: "Proyecto vacío", NameEN: "Empty project",
		DescES: "Un proyecto sin nada, ni siquiera la red por defecto. Para construir desde cero.",
		DescEN: "A project with nothing in it, not even the default network. To build from scratch."},
	{ID: "default-network", Baseline: []string{"default-network"},
		NameES: "Proyecto nuevo", NameEN: "New project",
		DescES: "Como un proyecto recién creado en Google Cloud: con la red default y sus reglas de cortafuegos.",
		DescEN: "Like a freshly created Google Cloud project: with the default network and its firewall rules."},
	{ID: "zentra", Baseline: []string{"zentra-core"},
		NameES: "Zentra (banco digital)", NameEN: "Zentra (digital bank)",
		DescES: "La plataforma de Zentra: VPC privada con Cloud NAT, IAP, políticas de organización de banca, cuentas de servicio y el directorio de empleados.",
		DescEN: "Zentra's platform: private VPC with Cloud NAT, IAP, banking organisation policies, service accounts and the employee directory."},
	{ID: "bank-atlas", Baseline: []string{"bank-atlas"},
		NameES: "Banco Atlas", NameEN: "Banco Atlas",
		DescES: "El entorno del banco de la ruta de soporte: redes, bases de datos, perímetros y permisos de un banco.",
		DescEN: "The bank of the support route: networks, databases, perimeters and permissions of a bank."},
	{ID: "three-tier", Baseline: []string{"three-tier-app"},
		NameES: "Aplicación de tres capas", NameEN: "Three-tier application",
		DescES: "Balanceador, grupo de VMs web y base de datos: la arquitectura clásica para romper y arreglar.",
		DescEN: "Load balancer, web VM group and database: the classic architecture to break and fix."},
	{ID: "shop", Baseline: []string{"default-network", "shop-platform"},
		NameES: "Tienda online", NameEN: "Online shop",
		DescES: "Una plataforma de comercio con varios servicios sobre la red default.",
		DescEN: "A commerce platform with several services on the default network."},
}

// IsSandbox reports whether a lab id is a sandbox.
func IsSandbox(id string) bool { return strings.HasPrefix(id, SandboxPrefix) }

// SandboxWorldByID finds a world.
func SandboxWorldByID(id string) *SandboxWorld {
	for i := range SandboxWorlds {
		if SandboxWorlds[i].ID == id {
			return &SandboxWorlds[i]
		}
	}
	return nil
}

// SandboxLab builds the lab of a sandbox id ("sandbox-<world>"), or nil.
func SandboxLab(id string) *Lab {
	w := SandboxWorldByID(strings.TrimPrefix(id, SandboxPrefix))
	if !IsSandbox(id) || w == nil {
		return nil
	}
	l := &Lab{
		ID:      id,
		Title:   "Consola libre · " + w.NameES,
		Summary: w.DescES,
		Track:   "sandbox",
		Type:    "sandbox",
		Level:   "free",
		Region:  "europe-west1",
		Zone:    "europe-west1-b",
		// four hours of play; the session also lasts while it is used
		Minutes:      240,
		Baseline:     append([]string(nil), w.Baseline...),
		FixedProject: SandboxProject,
		Instructions: "Consola libre: no hay misión, ni nota, ni pistas. Eres propietario del proyecto: crea, rompe y prueba lo que quieras en la consola o en Cloud Shell; las dos comparten el mismo estado. Guarda el mundo para seguir otro día.",
		EN: &LabEN{
			Title:        "Free console · " + w.NameEN,
			Summary:      w.DescEN,
			Instructions: "Free console: no mission, no score, no hints. You own the project: create, break and try anything in the console or in Cloud Shell; both share the same state. Save the world to carry on another day.",
		},
	}
	l.defaults()
	return l
}
