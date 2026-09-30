package tutor

import (
	"sort"
	"strings"
)

// Field is a form field of the console with the value to fill in.
type Field struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// flagLabels maps gcloud flags to the name of the console field (ES, EN).
var flagLabels = map[string][2]string{
	"zone":                           {"Zona", "Zone"},
	"region":                         {"Región", "Region"},
	"location":                       {"Ubicación", "Location"},
	"machine-type":                   {"Tipo de máquina", "Machine type"},
	"tags":                           {"Etiquetas de red", "Network tags"},
	"target-tags":                    {"Etiquetas de destino", "Target tags"},
	"source-ranges":                  {"Intervalos de IPv4 de origen", "Source IPv4 ranges"},
	"source-tags":                    {"Etiquetas de origen", "Source tags"},
	"allow":                          {"Protocolos y puertos (permitir)", "Protocols and ports (allow)"},
	"rules":                          {"Protocolos y puertos", "Protocols and ports"},
	"action":                         {"Acción", "Action"},
	"direction":                      {"Dirección del tráfico", "Direction of traffic"},
	"priority":                       {"Prioridad", "Priority"},
	"network":                        {"Red", "Network"},
	"subnet":                         {"Subred", "Subnetwork"},
	"range":                          {"Intervalo de direcciones IP", "IP address range"},
	"image-family":                   {"Imagen (familia)", "Image (family)"},
	"image-project":                  {"Proyecto de la imagen", "Image project"},
	"image":                          {"Imagen", "Image"},
	"metadata":                       {"Metadatos", "Metadata"},
	"metadata-from-file":             {"Metadatos › Secuencia de comandos de inicio", "Metadata › Startup script"},
	"service-account":                {"Cuenta de servicio", "Service account"},
	"scopes":                         {"Permisos de acceso (ámbitos)", "Access scopes"},
	"no-address":                     {"IP externa: ninguna", "External IP: none"},
	"boot-disk-size":                 {"Tamaño del disco de arranque", "Boot disk size"},
	"member":                         {"Principal", "Principal"},
	"role":                           {"Rol", "Role"},
	"display-name":                   {"Nombre visible", "Display name"},
	"description":                    {"Descripción", "Description"},
	"database-version":               {"Versión de la base de datos", "Database version"},
	"tier":                           {"Tipo de máquina / nivel", "Machine type / tier"},
	"availability-type":              {"Disponibilidad por zonas", "Zonal availability"},
	"backup-start-time":              {"Hora de las copias automáticas", "Automated backup time"},
	"enable-point-in-time-recovery":  {"Recuperación a un momento dado", "Point-in-time recovery"},
	"authorized-networks":            {"Redes autorizadas", "Authorized networks"},
	"instance":                       {"Instancia", "Instance"},
	"database":                       {"Base de datos", "Database"},
	"image-uri":                      {"URL de la imagen del contenedor", "Container image URL"},
	"allow-unauthenticated":          {"Autenticación: permitir acceso público", "Authentication: allow public access"},
	"no-allow-unauthenticated":       {"Autenticación: requerir autenticación", "Authentication: require authentication"},
	"set-env-vars":                   {"Variables de entorno", "Environment variables"},
	"update-env-vars":                {"Variables de entorno", "Environment variables"},
	"set-secrets":                    {"Secretos como variables", "Secrets as variables"},
	"min-instances":                  {"Número mínimo de instancias", "Minimum number of instances"},
	"max-instances":                  {"Número máximo de instancias", "Maximum number of instances"},
	"ingress":                        {"Entrada (ingress)", "Ingress"},
	"vpc-connector":                  {"Conector de VPC", "VPC connector"},
	"memory":                         {"Memoria", "Memory"},
	"runtime":                        {"Entorno de ejecución", "Runtime"},
	"entry-point":                    {"Punto de entrada", "Entry point"},
	"trigger-topic":                  {"Activador: tema de Pub/Sub", "Trigger: Pub/Sub topic"},
	"trigger-bucket":                 {"Activador: bucket", "Trigger: bucket"},
	"trigger-http":                   {"Activador: HTTPS", "Trigger: HTTPS"},
	"schedule":                       {"Frecuencia (cron)", "Frequency (cron)"},
	"topic":                          {"Tema", "Topic"},
	"message-body":                   {"Mensaje", "Message"},
	"uri":                            {"URL", "URL"},
	"oidc-service-account-email":     {"Cuenta de servicio (OIDC)", "Service account (OIDC)"},
	"ack-deadline":                   {"Plazo de confirmación", "Acknowledgement deadline"},
	"dead-letter-topic":              {"Tema de mensajes fallidos", "Dead-letter topic"},
	"max-delivery-attempts":          {"Número máximo de intentos", "Maximum delivery attempts"},
	"push-endpoint":                  {"Endpoint de envío (push)", "Push endpoint"},
	"uniform-bucket-level-access":    {"Control de acceso: uniforme", "Access control: uniform"},
	"public-access-prevention":       {"Prevención del acceso público", "Public access prevention"},
	"default-storage-class":          {"Clase de almacenamiento", "Storage class"},
	"lifecycle-file":                 {"Reglas de ciclo de vida", "Lifecycle rules"},
	"versioning":                     {"Control de versiones de objetos", "Object versioning"},
	"retention-period":               {"Política de retención", "Retention policy"},
	"default-encryption-key":         {"Cifrado: clave gestionada por el cliente", "Encryption: customer-managed key"},
	"num-nodes":                      {"Número de nodos", "Number of nodes"},
	"enable-autoscaling":             {"Autoescalado", "Autoscaling"},
	"min-nodes":                      {"Nodos mínimos", "Minimum nodes"},
	"max-nodes":                      {"Nodos máximos", "Maximum nodes"},
	"workload-pool":                  {"Workload Identity", "Workload Identity"},
	"size":                           {"Tamaño", "Size"},
	"template":                       {"Plantilla de instancia", "Instance template"},
	"health-check":                   {"Comprobación de estado", "Health check"},
	"health-checks":                  {"Comprobación de estado", "Health check"},
	"port":                           {"Puerto", "Port"},
	"request-path":                   {"Ruta de la solicitud", "Request path"},
	"protocol":                       {"Protocolo", "Protocol"},
	"port-name":                      {"Nombre del puerto", "Named port"},
	"named-ports":                    {"Puertos con nombre", "Named ports"},
	"instance-group":                 {"Grupo de instancias", "Instance group"},
	"default-service":                {"Servicio de backend predeterminado", "Default backend service"},
	"url-map":                        {"Mapa de URLs", "URL map"},
	"target-http-proxy":              {"Proxy de destino", "Target proxy"},
	"ports":                          {"Puertos", "Ports"},
	"address":                        {"Dirección IP", "IP address"},
	"security-policy":                {"Política de Cloud Armor", "Cloud Armor policy"},
	"src-ip-ranges":                  {"Rangos de IP de origen", "Source IP ranges"},
	"expression":                     {"Expresión", "Expression"},
	"peer-network":                   {"Red emparejada", "Peer network"},
	"nat-all-subnet-ip-ranges":       {"Subredes: todas", "Subnets: all"},
	"auto-allocate-nat-external-ips": {"IPs de NAT: automáticas", "NAT IPs: automatic"},
	"router":                         {"Cloud Router", "Cloud Router"},
	"log-filter":                     {"Filtro", "Filter"},
	"budget-amount":                  {"Importe", "Amount"},
	"threshold-rule":                 {"Umbral", "Threshold"},
	"data-file":                      {"Valor del secreto (archivo)", "Secret value (file)"},
	"replication-policy":             {"Política de replicación", "Replication policy"},
	"repository-format":              {"Formato", "Format"},
	"config":                         {"Configuración", "Configuration"},
	"processing-units":               {"Unidades de procesamiento", "Processing units"},
	"ddl":                            {"Esquema (DDL)", "Schema (DDL)"},
	"redis-version":                  {"Versión", "Version"},
	"splits":                         {"Reparto del tráfico", "Traffic split"},
	"split-by":                       {"Dividir por", "Split by"},
	"version":                        {"Versión", "Version"},
	"no-promote":                     {"No dar tráfico a la nueva versión", "Do not give the new version traffic"},
	"gcs-location":                   {"Plantilla", "Template"},
	"parameters":                     {"Parámetros de la plantilla", "Template parameters"},
	"enable-drop-protection":         {"Protección contra borrado", "Drop protection"},
	"delete-protection":              {"Protección contra borrado", "Delete protection"},
	"purpose":                        {"Finalidad", "Purpose"},
	"keyring":                        {"Llavero", "Key ring"},
	"rotation-period":                {"Periodo de rotación", "Rotation period"},
	"condition":                      {"Condición de IAM", "IAM condition"},
	"notification-channels":          {"Canales de notificación", "Notification channels"},
}

// hiddenFlags are flags with no console field worth showing.
var hiddenFlags = map[string]bool{"quiet": true, "format": true, "project": true, "global": true, "async": true, "verbosity": true, "gen2": true, "source": true, "tunnel-through-iap": true, "command": true, "limit": true, "freshness": true, "filter": true, "zones": false}

// fieldsOf lists the console fields a gcloud command fills in.
func fieldsOf(flags map[string]string, en bool) []Field {
	var keys []string
	for k := range flags {
		if !hiddenFlags[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var out []Field
	for _, k := range keys {
		v := flags[k]
		label := "--" + k
		if l, ok := flagLabels[k]; ok {
			label = l[0]
			if en {
				label = l[1]
			}
		}
		if v == "" {
			v = map[bool]string{true: "on", false: "sí"}[en]
		}
		if len(v) > 90 {
			v = v[:87] + "…"
		}
		out = append(out, Field{Label: label, Value: strings.TrimSpace(v)})
	}
	return out
}
