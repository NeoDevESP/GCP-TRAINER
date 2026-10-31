# Baseline: plataforma de Zentra, banco digital europeo ficticio (ruta «zentra»).
# Guardarraíles de organización de banca, VPC privada con salida por Cloud NAT,
# SSH solo por IAP, acceso a servicios privados para Cloud SQL y Memorystore,
# una cuenta de servicio por microservicio y el directorio de Google Workspace
# (OU, empleados y grupos gcp-*), con los permisos del proyecto dados a grupos.
gcloud resource-manager org-policies enable-enforce compute.vmExternalIpAccess --project={{.project}}
gcloud resource-manager org-policies enable-enforce sql.restrictPublicIp --project={{.project}}
gcloud resource-manager org-policies enable-enforce iam.managed.disableServiceAccountKeyCreation --project={{.project}}
gcloud resource-manager org-policies enable-enforce storage.publicAccessPrevention --project={{.project}}
gcloud resource-manager org-policies enable-enforce storage.uniformBucketLevelAccess --project={{.project}}
gcloud resource-manager org-policies enable-enforce iam.allowedPolicyMemberDomains --project={{.project}}
gcloud resource-manager org-policies allow gcp.resourceLocations in:eu-locations --project={{.project}}
gcloud compute networks create zentra-vpc --subnet-mode=custom
gcloud compute networks subnets create apps --network=zentra-vpc --range=10.40.1.0/24 --region={{.region}} --enable-private-ip-google-access
gcloud compute networks subnets create datos --network=zentra-vpc --range=10.40.2.0/24 --region={{.region}} --enable-private-ip-google-access
gcloud compute networks subnets create gke --network=zentra-vpc --range=10.40.8.0/22 --region={{.region}} --enable-private-ip-google-access
gcloud compute addresses create google-managed-services --global --purpose=VPC_PEERING --prefix-length=16 --network=zentra-vpc
gcloud services vpc-peerings connect --service=servicenetworking.googleapis.com --ranges=google-managed-services --network=zentra-vpc
gcloud compute routers create zentra-router --network=zentra-vpc --region={{.region}}
gcloud compute routers nats create zentra-nat --router=zentra-router --region={{.region}} --nat-all-subnet-ip-ranges --auto-allocate-nat-external-ips
gcloud compute firewall-rules create allow-iap-ssh --network=zentra-vpc --allow=tcp:22 --source-ranges=35.235.240.0/20
gcloud iam service-accounts create app-gateway-sa --display-name="app-gateway"
gcloud iam service-accounts create pagos-sa --display-name="pagos-api"
gcloud iam service-accounts create cuentas-sa --display-name="cuentas-api"
gcloud iam service-accounts create divisas-sa --display-name="divisas-api"
gcloud iam service-accounts create deploy-sa --display-name="pipeline de despliegue"
gam create org /Ingenieria description "Tecnologia"
gam create org /Ingenieria/Plataforma description "Plataforma Cloud, SRE, Red y Seguridad cloud"
gam create org /Ingenieria/Producto description "Pagos, Tarjetas, Cuentas y Divisas"
gam create org /Ingenieria/Datos description "Datos e IA"
gam create org /Operaciones description "Service Desk, Soporte GCP y NOC"
gam create org /Riesgo description "CISO, Cumplimiento y Auditoria"
gam create org /Finanzas description "Finanzas y FinOps"
gam create org /Externos description "Consultores y proveedores con acceso temporal"
gam create user marta.ibanez@zentra.dev firstname Marta lastname Ibanez org /Ingenieria/Plataforma title "Responsable de Plataforma Cloud"
gam create user diego.ferrer@zentra.dev firstname Diego lastname Ferrer org /Ingenieria/Plataforma title "SRE"
gam create user nadia.kowalski@zentra.dev firstname Nadia lastname Kowalski org /Ingenieria/Plataforma title "Seguridad cloud"
gam create user tomas.vidal@zentra.dev firstname Tomas lastname Vidal org /Ingenieria/Plataforma title "Arquitecto de red"
gam create user lucia.navarro@zentra.dev firstname Lucia lastname Navarro org /Ingenieria/Producto title "Tech lead de Pagos"
gam create user hugo.martin@zentra.dev firstname Hugo lastname Martin org /Ingenieria/Producto title "Desarrollador de Tarjetas"
gam create user sara.benitez@zentra.dev firstname Sara lastname Benitez org /Ingenieria/Datos title "Ingeniera de datos"
gam create user omar.haddad@zentra.dev firstname Omar lastname Haddad org /Ingenieria/Datos title "Cientifico de datos"
gam create user elena.rios@zentra.dev firstname Elena lastname Rios org /Finanzas title "FinOps"
gam create user pablo.costa@zentra.dev firstname Pablo lastname Costa org /Operaciones title "Service Desk"
gam create user irene.soler@zentra.dev firstname Irene lastname Soler org /Riesgo title "Cumplimiento"
gam create user javier.luna@zentra.dev firstname Javier lastname Luna org /Operaciones title "Gestor de cambios"
gam create user carla.mendez@zentra.dev firstname Carla lastname Mendez org /Ingenieria/Producto title "Product owner"
gcloud identity groups create gcp-plataforma@zentra.dev --organization=zentra.dev --display-name="GCP Plataforma" --labels=cloudidentity.googleapis.com/groups.security
gcloud identity groups create gcp-sre@zentra.dev --organization=zentra.dev --display-name="GCP SRE" --labels=cloudidentity.googleapis.com/groups.security
gcloud identity groups create gcp-seguridad@zentra.dev --organization=zentra.dev --display-name="GCP Seguridad" --labels=cloudidentity.googleapis.com/groups.security
gcloud identity groups create gcp-pagos-dev@zentra.dev --organization=zentra.dev --display-name="GCP Pagos - desarrollo" --labels=cloudidentity.googleapis.com/groups.security
gcloud identity groups create gcp-tarjetas-dev@zentra.dev --organization=zentra.dev --display-name="GCP Tarjetas - desarrollo" --labels=cloudidentity.googleapis.com/groups.security
gcloud identity groups create gcp-datos@zentra.dev --organization=zentra.dev --display-name="GCP Datos" --labels=cloudidentity.googleapis.com/groups.security
gcloud identity groups create gcp-finops@zentra.dev --organization=zentra.dev --display-name="GCP FinOps" --labels=cloudidentity.googleapis.com/groups.security
gcloud identity groups create gcp-soporte@zentra.dev --organization=zentra.dev --display-name="GCP Soporte N2" --labels=cloudidentity.googleapis.com/groups.security
gcloud identity groups create gcp-producto@zentra.dev --organization=zentra.dev --display-name="GCP Producto (todos los equipos)" --labels=cloudidentity.googleapis.com/groups.security
gcloud identity groups memberships add --group-email=gcp-plataforma@zentra.dev --member-email=marta.ibanez@zentra.dev --roles=MEMBER,OWNER
gcloud identity groups memberships add --group-email=gcp-plataforma@zentra.dev --member-email=tomas.vidal@zentra.dev
gcloud identity groups memberships add --group-email=gcp-sre@zentra.dev --member-email=diego.ferrer@zentra.dev
gcloud identity groups memberships add --group-email=gcp-seguridad@zentra.dev --member-email=nadia.kowalski@zentra.dev
gcloud identity groups memberships add --group-email=gcp-pagos-dev@zentra.dev --member-email=lucia.navarro@zentra.dev
gcloud identity groups memberships add --group-email=gcp-tarjetas-dev@zentra.dev --member-email=hugo.martin@zentra.dev
gcloud identity groups memberships add --group-email=gcp-datos@zentra.dev --member-email=sara.benitez@zentra.dev
gcloud identity groups memberships add --group-email=gcp-datos@zentra.dev --member-email=omar.haddad@zentra.dev
gcloud identity groups memberships add --group-email=gcp-finops@zentra.dev --member-email=elena.rios@zentra.dev
gcloud identity groups memberships add --group-email=gcp-soporte@zentra.dev --member-email=pablo.costa@zentra.dev
gcloud identity groups memberships add --group-email=gcp-producto@zentra.dev --member-email=gcp-pagos-dev@zentra.dev
gcloud identity groups memberships add --group-email=gcp-producto@zentra.dev --member-email=gcp-tarjetas-dev@zentra.dev
gcloud identity groups memberships add --group-email=gcp-producto@zentra.dev --member-email=carla.mendez@zentra.dev
gcloud projects add-iam-policy-binding {{.project}} --member=group:gcp-producto@zentra.dev --role=roles/run.viewer
gcloud projects add-iam-policy-binding {{.project}} --member=group:gcp-producto@zentra.dev --role=roles/logging.viewer
gcloud projects add-iam-policy-binding {{.project}} --member=group:gcp-sre@zentra.dev --role=roles/monitoring.editor
gcloud projects add-iam-policy-binding {{.project}} --member=group:gcp-sre@zentra.dev --role=roles/logging.viewer
gcloud projects add-iam-policy-binding {{.project}} --member=group:gcp-sre@zentra.dev --role=roles/run.developer
gcloud projects add-iam-policy-binding {{.project}} --member=group:gcp-seguridad@zentra.dev --role=roles/iam.securityReviewer
gcloud projects add-iam-policy-binding {{.project}} --member=group:gcp-finops@zentra.dev --role=roles/billing.viewer
gcloud projects add-iam-policy-binding {{.project}} --member=group:gcp-soporte@zentra.dev --role=roles/browser
gcloud projects add-iam-policy-binding {{.project}} --member=serviceAccount:deploy-sa@{{.project}}.iam.gserviceaccount.com --role=roles/run.admin
