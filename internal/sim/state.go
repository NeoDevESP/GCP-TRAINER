// Package sim implements the F0 fidelity layer: a deterministic, in-memory
// model of a Google Cloud organization. It is not a clone of GCP; it models the
// state, decisions, errors and consequences that matter for practice labs
// (resource hierarchy, IAM, VPC reachability, workloads, observability, cost).
package sim

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"
)

// State is the whole simulated world for one lab session.
type State struct {
	Seed     int64               `json:"seed"`
	Clock    time.Time           `json:"clock"`
	Org      *Organization       `json:"org"`
	Folders  map[string]*Folder  `json:"folders"`
	Projects map[string]*Project `json:"projects"`
	// Buckets live in a global namespace in GCS.
	Logs    []LogEntry           `json:"logs"`
	Metrics map[string][]Point   `json:"metrics"`
	Traffic []TrafficSpec        `json:"traffic"`
	Images  map[string]*Behavior `json:"images"` // image catalog (behaviour models)
	Tick    int                  `json:"tick"`
	Extra   map[string]string    `json:"extra,omitempty"`
	ipSeq   map[string]int
	rng     *rand.Rand
	explain *[]Hop // causal-chain recorder for "teach me why" (not serialised)
}

type Organization struct {
	ID          string                `json:"id"`
	DisplayName string                `json:"displayName"`
	IAM         Policy                `json:"iamPolicy"`
	OrgPolicies map[string]*OrgPolicy `json:"orgPolicies"`
}

type Folder struct {
	ID          string                `json:"id"`
	DisplayName string                `json:"displayName"`
	Parent      string                `json:"parent"`
	IAM         Policy                `json:"iamPolicy"`
	OrgPolicies map[string]*OrgPolicy `json:"orgPolicies,omitempty"`
}

type OrgPolicy struct {
	Constraint    string   `json:"constraint"`
	Enforce       bool     `json:"enforce"`
	AllowedValues []string `json:"allowedValues,omitempty"`
	DeniedValues  []string `json:"deniedValues,omitempty"`
}

type Policy struct {
	Bindings []Binding `json:"bindings"`
	Version  int       `json:"version"`
}

type Binding struct {
	Role      string     `json:"role"`
	Members   []string   `json:"members"`
	Condition *Condition `json:"condition,omitempty"`
}

type Condition struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Expression  string `json:"expression"`
}

type Project struct {
	ID             string                `json:"projectId"`
	Number         string                `json:"projectNumber"`
	Name           string                `json:"name"`
	Parent         string                `json:"parent"`
	State          string                `json:"lifecycleState"`
	Labels         map[string]string     `json:"labels"`
	IAM            Policy                `json:"iamPolicy"`
	Services       map[string]bool       `json:"services"`
	OrgPolicies    map[string]*OrgPolicy `json:"orgPolicies"`
	BillingEnabled bool                  `json:"billingEnabled"`

	ServiceAccounts map[string]*ServiceAccount `json:"serviceAccounts"`
	CustomRoles     map[string]*Role           `json:"customRoles"`

	Networks          map[string]*Network          `json:"networks"`
	Subnets           map[string]*Subnet           `json:"subnets"`
	Firewalls         map[string]*Firewall         `json:"firewalls"`
	Routes            map[string]*Route            `json:"routes"`
	Routers           map[string]*Router           `json:"routers"`
	Addresses         map[string]*Address          `json:"addresses"`
	Instances         map[string]*Instance         `json:"instances"`
	InstanceTemplates map[string]*InstanceTemplate `json:"instanceTemplates"`
	InstanceGroups    map[string]*InstanceGroup    `json:"instanceGroups"`
	Disks             map[string]*Disk             `json:"disks"`
	Snapshots         map[string]*Snapshot         `json:"snapshots"`
	HealthChecks      map[string]*HealthCheck      `json:"healthChecks"`
	BackendServices   map[string]*BackendService   `json:"backendServices"`
	URLMaps           map[string]*URLMap           `json:"urlMaps"`
	TargetProxies     map[string]*TargetProxy      `json:"targetProxies"`
	ForwardingRules   map[string]*ForwardingRule   `json:"forwardingRules"`
	SecurityPolicies  map[string]*SecurityPolicy   `json:"securityPolicies"`
	NEGs              map[string]*NEG              `json:"negs"`
	VPNGateways       map[string]*VPNGateway       `json:"vpnGateways"`
	DNSZones          map[string]*DNSZone          `json:"dnsZones"`

	Buckets      map[string]*Bucket       `json:"buckets"`
	RunServices  map[string]*RunService   `json:"runServices"`
	SQLInstances map[string]*SQLInstance  `json:"sqlInstances"`
	Topics       map[string]*Topic        `json:"topics"`
	Subs         map[string]*Subscription `json:"subscriptions"`
	Secrets      map[string]*Secret       `json:"secrets"`
	KeyRings     map[string]*KeyRing      `json:"keyRings"`
	Clusters     map[string]*Cluster      `json:"clusters"`

	ArtifactRepos map[string]*ArtifactRepo     `json:"artifactRepos"`
	Builds        []*Build                     `json:"builds"`
	Triggers      map[string]*BuildTrigger     `json:"buildTriggers"`
	SourceRepos   map[string]*SourceRepo       `json:"sourceRepos"`
	Pipelines     map[string]*DeliveryPipeline `json:"deliveryPipelines"`
	DeployTargets map[string]*DeployTarget     `json:"deployTargets"`
	Releases      []*Release                   `json:"releases"`

	Datasets    map[string]*Dataset    `json:"datasets"`
	BQJobs      []*BQJob               `json:"bqJobs"`
	AIModels    map[string]*AIModel    `json:"aiModels"`
	AIEndpoints map[string]*AIEndpoint `json:"aiEndpoints"`

	LogMetrics    map[string]*LogMetric           `json:"logMetrics"`
	AlertPolicies map[string]*AlertPolicy         `json:"alertPolicies"`
	Channels      map[string]*NotificationChannel `json:"notificationChannels"`
	UptimeChecks  map[string]*UptimeCheck         `json:"uptimeChecks"`
	Budgets       map[string]*Budget              `json:"budgets"`
	LogSinks      map[string]*LogSink             `json:"logSinks"`
	Perimeters    map[string]*Perimeter           `json:"perimeters"`
}

type ServiceAccount struct {
	Email       string  `json:"email"`
	DisplayName string  `json:"displayName"`
	Disabled    bool    `json:"disabled"`
	IAM         Policy  `json:"iamPolicy"`
	Keys        []SAKey `json:"keys"`
}

type SAKey struct {
	ID      string `json:"id"`
	Created string `json:"created"`
	Leaked  bool   `json:"leaked,omitempty"`
}

type Role struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Permissions []string `json:"includedPermissions"`
	Stage       string   `json:"stage"`
}

type Network struct {
	Name         string    `json:"name"`
	Mode         string    `json:"subnetMode"`
	Peerings     []Peering `json:"peerings"`
	PSARanges    []string  `json:"psaRanges"`
	PSAConnected bool      `json:"psaConnected"`
}

type Peering struct {
	Name    string `json:"name"`
	Network string `json:"network"` // project/network
	State   string `json:"state"`
}

type Subnet struct {
	Name                string `json:"name"`
	Region              string `json:"region"`
	Network             string `json:"network"`
	Range               string `json:"ipCidrRange"`
	PrivateGoogleAccess bool   `json:"privateIpGoogleAccess"`
	FlowLogs            bool   `json:"flowLogs"`
	Purpose             string `json:"purpose,omitempty"`
}

type Firewall struct {
	Name         string   `json:"name"`
	Network      string   `json:"network"`
	Direction    string   `json:"direction"`
	Priority     int      `json:"priority"`
	Action       string   `json:"action"` // ALLOW or DENY
	Rules        []FWRule `json:"rules"`
	SourceRanges []string `json:"sourceRanges"`
	DestRanges   []string `json:"destinationRanges"`
	SourceTags   []string `json:"sourceTags"`
	TargetTags   []string `json:"targetTags"`
	SourceSAs    []string `json:"sourceServiceAccounts"`
	TargetSAs    []string `json:"targetServiceAccounts"`
	Disabled     bool     `json:"disabled"`
	Logging      bool     `json:"logging"`
	Description  string   `json:"description,omitempty"`
}

type FWRule struct {
	Protocol string   `json:"IPProtocol"`
	Ports    []string `json:"ports,omitempty"`
}

type Route struct {
	Name            string   `json:"name"`
	Network         string   `json:"network"`
	DestRange       string   `json:"destRange"`
	Priority        int      `json:"priority"`
	NextHopGateway  string   `json:"nextHopGateway,omitempty"`
	NextHopInstance string   `json:"nextHopInstance,omitempty"`
	NextHopIP       string   `json:"nextHopIp,omitempty"`
	NextHopVPN      string   `json:"nextHopVpnTunnel,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	System          bool     `json:"system,omitempty"`
}

type Router struct {
	Name     string    `json:"name"`
	Region   string    `json:"region"`
	Network  string    `json:"network"`
	ASN      int       `json:"asn"`
	NATs     []NAT     `json:"nats"`
	BGPPeers []BGPPeer `json:"bgpPeers,omitempty"`
}

type BGPPeer struct {
	Name      string `json:"name"`
	PeerASN   int    `json:"peerAsn"`
	Interface string `json:"interface"`
}

type NAT struct {
	Name       string   `json:"name"`
	AllSubnets bool     `json:"allSubnets"`
	Subnets    []string `json:"subnets"`
	AutoIPs    bool     `json:"autoAllocateIps"`
	LogErrors  bool     `json:"logErrors"`
}

type Address struct {
	Name    string `json:"name"`
	Region  string `json:"region"`
	Address string `json:"address"`
	Type    string `json:"addressType"`
	Purpose string `json:"purpose,omitempty"`
	Prefix  int    `json:"prefixLength,omitempty"`
	Network string `json:"network,omitempty"`
	User    string `json:"user,omitempty"`
}

type Instance struct {
	Name               string            `json:"name"`
	Zone               string            `json:"zone"`
	MachineType        string            `json:"machineType"`
	Status             string            `json:"status"`
	Tags               []string          `json:"tags"`
	Labels             map[string]string `json:"labels"`
	Metadata           map[string]string `json:"metadata"`
	Network            string            `json:"network"`
	Subnet             string            `json:"subnetwork"`
	InternalIP         string            `json:"networkIP"`
	ExternalIP         string            `json:"natIP,omitempty"`
	ServiceAccount     string            `json:"serviceAccount"`
	Scopes             []string          `json:"scopes"`
	Image              string            `json:"image"`
	BootDisk           string            `json:"bootDisk"`
	Disks              []string          `json:"disks"`
	GPUs               int               `json:"gpus"`
	Spot               bool              `json:"spot"`
	DeletionProtection bool              `json:"deletionProtection"`
	Group              string            `json:"group,omitempty"`
	CPU                float64           `json:"cpuUtilization"`
	CreatedBy          string            `json:"createdBy,omitempty"`
	ShieldedVM         bool              `json:"shieldedVm"`
	OSLogin            bool              `json:"osLogin"`
}

type InstanceTemplate struct {
	Name           string            `json:"name"`
	MachineType    string            `json:"machineType"`
	Tags           []string          `json:"tags"`
	Metadata       map[string]string `json:"metadata"`
	Network        string            `json:"network"`
	Subnet         string            `json:"subnetwork"`
	Region         string            `json:"region"`
	NoAddress      bool              `json:"noAddress"`
	ServiceAccount string            `json:"serviceAccount"`
	Image          string            `json:"image"`
	Labels         map[string]string `json:"labels"`
}

type InstanceGroup struct {
	Name             string         `json:"name"`
	Zone             string         `json:"zone"`
	Region           string         `json:"region,omitempty"`
	Zones            []string       `json:"distributionZones,omitempty"` // regional MIG: zones instances are spread over
	Managed          bool           `json:"managed"`
	Template         string         `json:"instanceTemplate,omitempty"`
	TargetSize       int            `json:"targetSize"`
	Instances        []string       `json:"instances"`
	NamedPorts       map[string]int `json:"namedPorts"`
	BaseInstanceName string         `json:"baseInstanceName"`
	Autoscaler       *Autoscaler    `json:"autoscaler,omitempty"`
	AutoHealing      *AutoHealing   `json:"autoHealing,omitempty"`
}

type Autoscaler struct {
	Min       int     `json:"minNumReplicas"`
	Max       int     `json:"maxNumReplicas"`
	TargetCPU float64 `json:"targetCpuUtilization"`
	Cooldown  int     `json:"coolDownPeriodSec"`
}

type AutoHealing struct {
	HealthCheck  string `json:"healthCheck"`
	InitialDelay int    `json:"initialDelaySec"`
}

type Disk struct {
	Name     string   `json:"name"`
	Zone     string   `json:"zone"`
	SizeGB   int      `json:"sizeGb"`
	Type     string   `json:"type"`
	Image    string   `json:"sourceImage,omitempty"`
	Users    []string `json:"users"`
	Snapshot string   `json:"sourceSnapshot,omitempty"`
	KMSKey   string   `json:"kmsKey,omitempty"`
}

type Snapshot struct {
	Name       string `json:"name"`
	SourceDisk string `json:"sourceDisk"`
	Location   string `json:"storageLocation"`
	SizeGB     int    `json:"diskSizeGb"`
	Created    string `json:"creationTimestamp"`
}

type HealthCheck struct {
	Name        string `json:"name"`
	Protocol    string `json:"type"`
	Port        int    `json:"port"`
	RequestPath string `json:"requestPath"`
	Interval    int    `json:"checkIntervalSec"`
	Region      string `json:"region,omitempty"`
}

type BackendService struct {
	Name           string    `json:"name"`
	Protocol       string    `json:"protocol"`
	Global         bool      `json:"global"`
	Region         string    `json:"region,omitempty"`
	Scheme         string    `json:"loadBalancingScheme"`
	HealthChecks   []string  `json:"healthChecks"`
	Backends       []Backend `json:"backends"`
	PortName       string    `json:"portName"`
	SecurityPolicy string    `json:"securityPolicy,omitempty"`
	CDN            bool      `json:"enableCDN"`
	TimeoutSec     int       `json:"timeoutSec"`
	Logging        bool      `json:"logging"`
}

type Backend struct {
	Group         string `json:"group,omitempty"`
	Zone          string `json:"zone,omitempty"`
	NEG           string `json:"neg,omitempty"`
	BalancingMode string `json:"balancingMode,omitempty"`
}

type URLMap struct {
	Name           string        `json:"name"`
	DefaultService string        `json:"defaultService"`
	HostRules      []HostRule    `json:"hostRules"`
	PathMatchers   []PathMatcher `json:"pathMatchers"`
}

type HostRule struct {
	Hosts       []string `json:"hosts"`
	PathMatcher string   `json:"pathMatcher"`
}

type PathMatcher struct {
	Name           string     `json:"name"`
	DefaultService string     `json:"defaultService"`
	PathRules      []PathRule `json:"pathRules"`
}

type PathRule struct {
	Paths   []string `json:"paths"`
	Service string   `json:"service"`
}

type TargetProxy struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"` // http or https
	URLMap   string   `json:"urlMap"`
	SSLCerts []string `json:"sslCertificates,omitempty"`
}

type ForwardingRule struct {
	Name           string   `json:"name"`
	IP             string   `json:"IPAddress"`
	Ports          []string `json:"ports"`
	Target         string   `json:"target,omitempty"`
	BackendService string   `json:"backendService,omitempty"`
	Global         bool     `json:"global"`
	Region         string   `json:"region,omitempty"`
	Scheme         string   `json:"loadBalancingScheme"`
	Network        string   `json:"network,omitempty"`
	Subnet         string   `json:"subnetwork,omitempty"`
}

type SecurityPolicy struct {
	Name  string   `json:"name"`
	Rules []SPRule `json:"rules"`
}

type SPRule struct {
	Priority    int      `json:"priority"`
	Action      string   `json:"action"`
	SrcIPRanges []string `json:"srcIpRanges,omitempty"`
	Expression  string   `json:"expression,omitempty"`
	Preview     bool     `json:"preview"`
	Description string   `json:"description,omitempty"`
}

type NEG struct {
	Name       string `json:"name"`
	Region     string `json:"region"`
	Type       string `json:"networkEndpointType"`
	RunService string `json:"cloudRunService,omitempty"`
}

type VPNGateway struct {
	Name    string      `json:"name"`
	Region  string      `json:"region"`
	Network string      `json:"network"`
	HA      bool        `json:"ha"`
	Tunnels []VPNTunnel `json:"tunnels"`
}

type VPNTunnel struct {
	Name      string `json:"name"`
	PeerIP    string `json:"peerIp"`
	Router    string `json:"router"`
	Status    string `json:"status"`
	Interface int    `json:"interface"`
}

type DNSZone struct {
	Name        string      `json:"name"`
	DNSName     string      `json:"dnsName"`
	Visibility  string      `json:"visibility"`
	Networks    []string    `json:"networks,omitempty"`
	Records     []DNSRecord `json:"records"`
	DNSSEC      bool        `json:"dnssec"`
	Description string      `json:"description"`
}

type DNSRecord struct {
	Name string   `json:"name"`
	Type string   `json:"type"`
	TTL  int      `json:"ttl"`
	Data []string `json:"rrdatas"`
}

type Bucket struct {
	Name          string             `json:"name"`
	Project       string             `json:"project"`
	Location      string             `json:"location"`
	LocationType  string             `json:"locationType"`
	StorageClass  string             `json:"storageClass"`
	UBLA          bool               `json:"uniformBucketLevelAccess"`
	PAP           string             `json:"publicAccessPrevention"`
	Versioning    bool               `json:"versioning"`
	Lifecycle     []LifecycleRule    `json:"lifecycle"`
	IAM           Policy             `json:"iamPolicy"`
	Objects       map[string]*Object `json:"objects"`
	KMSKey        string             `json:"defaultKmsKey,omitempty"`
	RetentionSec  int                `json:"retentionPeriodSec,omitempty"`
	Labels        map[string]string  `json:"labels"`
	SoftDeleteSec int                `json:"softDeleteRetentionSec"`
	Logging       bool               `json:"accessLogging"`
}

type LifecycleRule struct {
	Action    LifecycleAction    `json:"action"`
	Condition LifecycleCondition `json:"condition"`
}

type LifecycleAction struct {
	Type         string `json:"type"`
	StorageClass string `json:"storageClass,omitempty"`
}

type LifecycleCondition struct {
	Age                 int      `json:"age,omitempty"`
	NumNewerVersions    int      `json:"numNewerVersions,omitempty"`
	IsLive              *bool    `json:"isLive,omitempty"`
	MatchesStorageClass []string `json:"matchesStorageClass,omitempty"`
	MatchesPrefix       []string `json:"matchesPrefix,omitempty"`
}

type Object struct {
	Name         string `json:"name"`
	Size         int    `json:"size"`
	Content      string `json:"content,omitempty"`
	StorageClass string `json:"storageClass"`
	Generation   int    `json:"generation"`
	Updated      string `json:"updated"`
	ACLPublic    bool   `json:"publicRead,omitempty"`
}

type RunService struct {
	Name         string            `json:"name"`
	Region       string            `json:"region"`
	Image        string            `json:"image"`
	URL          string            `json:"url"`
	Env          map[string]string `json:"env"`
	Secrets      map[string]string `json:"secrets"` // ENV -> secret:version
	SA           string            `json:"serviceAccount"`
	Ingress      string            `json:"ingress"`
	IAM          Policy            `json:"iamPolicy"`
	MinInstances int               `json:"minInstances"`
	MaxInstances int               `json:"maxInstances"`
	Concurrency  int               `json:"concurrency"`
	Memory       string            `json:"memory"`
	CPU          string            `json:"cpu"`
	Port         int               `json:"port"`
	Revisions    []Revision        `json:"revisions"`
	Network      string            `json:"network,omitempty"`
	Subnet       string            `json:"subnet,omitempty"`
	VPCConnector string            `json:"vpcConnector,omitempty"`
	VPCEgress    string            `json:"vpcEgress,omitempty"`
	CloudSQL     []string          `json:"cloudSqlInstances,omitempty"`
	Labels       map[string]string `json:"labels"`
	Instances    int               `json:"activeInstances"`
}

type Revision struct {
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	Env     map[string]string `json:"env"`
	Secrets map[string]string `json:"secrets"`
	Traffic int               `json:"traffic"`
	Tag     string            `json:"tag,omitempty"`
}

type SQLInstance struct {
	Name               string            `json:"name"`
	Version            string            `json:"databaseVersion"`
	Tier               string            `json:"tier"`
	Region             string            `json:"region"`
	PublicIP           string            `json:"publicIp,omitempty"`
	PrivateIP          string            `json:"privateIp,omitempty"`
	Network            string            `json:"network,omitempty"`
	AuthorizedNets     []string          `json:"authorizedNetworks"`
	Availability       string            `json:"availabilityType"`
	BackupsEnabled     bool              `json:"backupEnabled"`
	BackupStart        string            `json:"backupStartTime,omitempty"`
	PITR               bool              `json:"pointInTimeRecovery"`
	Flags              map[string]string `json:"databaseFlags"`
	State              string            `json:"state"`
	Databases          []string          `json:"databases"`
	Users              map[string]string `json:"users"`
	Master             string            `json:"masterInstanceName,omitempty"`
	Replicas           []string          `json:"replicaNames"`
	Backups            []SQLBackup       `json:"backups"`
	RequireSSL         bool              `json:"requireSsl"`
	DeletionProtection bool              `json:"deletionProtection"`
	Connections        int               `json:"activeConnections"`
	CPU                float64           `json:"cpuUtilization"`
	SlowQueries        []string          `json:"slowQueries,omitempty"`
	Indexes            []string          `json:"indexes,omitempty"`
}

type SQLBackup struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Time   string `json:"windowStartTime"`
}

type Topic struct {
	Name      string `json:"name"`
	Retention string `json:"messageRetentionDuration,omitempty"`
	KMSKey    string `json:"kmsKeyName,omitempty"`
	Published int    `json:"publishedCount"`
}

type Subscription struct {
	Name                string    `json:"name"`
	Topic               string    `json:"topic"`
	AckDeadline         int       `json:"ackDeadlineSeconds"`
	PushEndpoint        string    `json:"pushEndpoint,omitempty"`
	PushSA              string    `json:"pushServiceAccount,omitempty"`
	DeadLetterTopic     string    `json:"deadLetterTopic,omitempty"`
	MaxDeliveryAttempts int       `json:"maxDeliveryAttempts,omitempty"`
	BQTable             string    `json:"bigqueryTable,omitempty"`
	Filter              string    `json:"filter,omitempty"`
	Backlog             []Message `json:"backlog"`
	Acked               int       `json:"ackedCount"`
	DeadLettered        int       `json:"deadLetteredCount"`
	Retention           string    `json:"messageRetentionDuration,omitempty"`
	ExactlyOnce         bool      `json:"enableExactlyOnceDelivery"`
}

type Message struct {
	ID         string            `json:"messageId"`
	Data       string            `json:"data"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Attempts   int               `json:"deliveryAttempt"`
	Published  string            `json:"publishTime"`
}

type Secret struct {
	Name        string            `json:"name"`
	Replication string            `json:"replication"`
	Labels      map[string]string `json:"labels"`
	Versions    []SecretVersion   `json:"versions"`
	IAM         Policy            `json:"iamPolicy"`
	Rotation    string            `json:"rotationPeriod,omitempty"`
	KMSKey      string            `json:"kmsKeyName,omitempty"`
}

type SecretVersion struct {
	ID      int    `json:"id"`
	Data    string `json:"data"`
	State   string `json:"state"`
	Exposed bool   `json:"exposed,omitempty"`
}

type KeyRing struct {
	Name     string                `json:"name"`
	Location string                `json:"location"`
	Keys     map[string]*CryptoKey `json:"keys"`
}

type CryptoKey struct {
	Name           string       `json:"name"`
	Purpose        string       `json:"purpose"`
	RotationPeriod string       `json:"rotationPeriod,omitempty"`
	NextRotation   string       `json:"nextRotationTime,omitempty"`
	Protection     string       `json:"protectionLevel"`
	Versions       []KeyVersion `json:"versions"`
	IAM            Policy       `json:"iamPolicy"`
}

type KeyVersion struct {
	ID    int    `json:"id"`
	State string `json:"state"`
}

type Cluster struct {
	Name           string     `json:"name"`
	Location       string     `json:"location"`
	Autopilot      bool       `json:"autopilot"`
	NodePools      []NodePool `json:"nodePools"`
	WorkloadPool   string     `json:"workloadPool,omitempty"`
	Network        string     `json:"network"`
	Subnet         string     `json:"subnetwork"`
	PrivateNodes   bool       `json:"privateNodes"`
	MasterAuthNets []string   `json:"masterAuthorizedNetworks,omitempty"`
	ReleaseChannel string     `json:"releaseChannel"`
	Status         string     `json:"status"`
	Endpoint       string     `json:"endpoint"`
	K8s            *K8sState  `json:"k8s"`
	ShieldedNodes  bool       `json:"shieldedNodes"`
	BinaryAuthz    bool       `json:"binaryAuthorization"`
}

type NodePool struct {
	Name        string `json:"name"`
	MachineType string `json:"machineType"`
	Count       int    `json:"nodeCount"`
	Autoscaling bool   `json:"autoscaling"`
	Min         int    `json:"minNodeCount"`
	Max         int    `json:"maxNodeCount"`
	SA          string `json:"serviceAccount"`
	Spot        bool   `json:"spot"`
}

type ArtifactRepo struct {
	Name     string              `json:"name"`
	Location string              `json:"location"`
	Format   string              `json:"format"`
	Images   map[string][]string `json:"images"` // image -> tags
	Cleanup  bool                `json:"cleanupPolicies"`
	Scanning bool                `json:"vulnerabilityScanning"`
}

type Build struct {
	ID      string   `json:"id"`
	Status  string   `json:"status"`
	Source  string   `json:"source"`
	Trigger string   `json:"buildTriggerId,omitempty"`
	Images  []string `json:"images"`
	Steps   []string `json:"steps"`
	Log     []string `json:"log"`
	Commit  string   `json:"commitSha,omitempty"`
	SA      string   `json:"serviceAccount,omitempty"`
	Created string   `json:"createTime"`
}

type BuildTrigger struct {
	Name        string `json:"name"`
	Repo        string `json:"repo"`
	Branch      string `json:"branchPattern"`
	BuildConfig string `json:"filename"`
	SA          string `json:"serviceAccount,omitempty"`
	Region      string `json:"region"`
}

type SourceRepo struct {
	Name    string      `json:"name"`
	Commits []GitCommit `json:"commits"`
}

type GitCommit struct {
	SHA     string            `json:"sha"`
	Message string            `json:"message"`
	Branch  string            `json:"branch"`
	Files   map[string]string `json:"files"`
}

type DeliveryPipeline struct {
	Name              string        `json:"name"`
	Region            string        `json:"region"`
	Stages            []DeployStage `json:"stages"`
	RollbackOnFailure bool          `json:"automaticRollback"`
}

type DeployStage struct {
	Target string `json:"targetId"`
	Verify bool   `json:"verify"`
	Canary []int  `json:"canaryPercentages,omitempty"`
}

type DeployTarget struct {
	Name            string `json:"name"`
	Region          string `json:"region"`
	RunService      string `json:"runService,omitempty"`
	Cluster         string `json:"gkeCluster,omitempty"`
	RequireApproval bool   `json:"requireApproval"`
}

type Release struct {
	Name     string    `json:"name"`
	Pipeline string    `json:"deliveryPipeline"`
	Image    string    `json:"image"`
	Rollouts []Rollout `json:"rollouts"`
}

type Rollout struct {
	Target string `json:"target"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

type Dataset struct {
	ID                    string            `json:"datasetId"`
	Location              string            `json:"location"`
	Tables                map[string]*Table `json:"tables"`
	IAM                   Policy            `json:"access"`
	DefaultExpirationDays int               `json:"defaultTableExpirationDays,omitempty"`
}

type Table struct {
	ID                     string   `json:"tableId"`
	Kind                   string   `json:"type"` // TABLE, VIEW, MODEL, MATERIALIZED_VIEW
	Schema                 []Field  `json:"schema"`
	Rows                   int64    `json:"numRows"`
	PartitionField         string   `json:"partitionField,omitempty"`
	PartitionType          string   `json:"partitionType,omitempty"`
	PartitionDays          int      `json:"partitionDays,omitempty"`
	Clustering             []string `json:"clustering,omitempty"`
	Query                  string   `json:"query,omitempty"`
	RequirePartitionFilter bool     `json:"requirePartitionFilter"`
	ModelType              string   `json:"modelType,omitempty"`
}

type Field struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Bytes int    `json:"avgBytes"`
}

type BQJob struct {
	ID             string `json:"jobId"`
	Query          string `json:"query"`
	BytesProcessed int64  `json:"totalBytesProcessed"`
	DryRun         bool   `json:"dryRun"`
	User           string `json:"user"`
	Destination    string `json:"destinationTable,omitempty"`
	Error          string `json:"error,omitempty"`
}

type AIModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Region      string `json:"region"`
	Image       string `json:"containerImageUri"`
	ArtifactURI string `json:"artifactUri"`
}

type AIEndpoint struct {
	ID          string          `json:"id"`
	DisplayName string          `json:"displayName"`
	Region      string          `json:"region"`
	Deployed    []DeployedModel `json:"deployedModels"`
	Predictions int             `json:"predictionCount"`
	Monitoring  bool            `json:"modelMonitoring"`
	Private     bool            `json:"private"`
}

type DeployedModel struct {
	ModelID     string `json:"model"`
	MachineType string `json:"machineType"`
	MinReplicas int    `json:"minReplicaCount"`
	MaxReplicas int    `json:"maxReplicaCount"`
	Traffic     int    `json:"trafficPercent"`
	GPUs        int    `json:"acceleratorCount"`
	SA          string `json:"serviceAccount,omitempty"`
}

type LogMetric struct {
	Name        string `json:"name"`
	Filter      string `json:"filter"`
	Description string `json:"description"`
}

type AlertPolicy struct {
	Name          string           `json:"name"`
	DisplayName   string           `json:"displayName"`
	Conditions    []AlertCondition `json:"conditions"`
	Channels      []string         `json:"notificationChannels"`
	Enabled       bool             `json:"enabled"`
	Firing        bool             `json:"firing"`
	Documentation string           `json:"documentation,omitempty"`
}

type AlertCondition struct {
	DisplayName string  `json:"displayName"`
	Filter      string  `json:"filter"`
	Comparison  string  `json:"comparison"`
	Threshold   float64 `json:"thresholdValue"`
	Duration    string  `json:"duration"`
}

type NotificationChannel struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"displayName"`
	Type        string            `json:"type"`
	Labels      map[string]string `json:"labels"`
}

type UptimeCheck struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	Path    string `json:"path"`
	Port    int    `json:"port"`
	Period  string `json:"period"`
	Passing bool   `json:"passing"`
}

type Budget struct {
	Name        string    `json:"name"`
	Amount      float64   `json:"amount"`
	Thresholds  []float64 `json:"thresholds"`
	PubSubTopic string    `json:"pubsubTopic,omitempty"`
}

type LogSink struct {
	Name        string `json:"name"`
	Destination string `json:"destination"`
	Filter      string `json:"filter"`
	Writer      string `json:"writerIdentity"`
}

type Perimeter struct {
	Name       string   `json:"name"`
	Resources  []string `json:"resources"`
	Restricted []string `json:"restrictedServices"`
	DryRun     bool     `json:"dryRun"`
}

// LogEntry mimics a Cloud Logging entry.
type LogEntry struct {
	Timestamp string            `json:"timestamp"`
	Severity  string            `json:"severity"`
	LogName   string            `json:"logName"`
	Resource  LogResource       `json:"resource"`
	Text      string            `json:"textPayload,omitempty"`
	HTTP      *HTTPRequestLog   `json:"httpRequest,omitempty"`
	Proto     map[string]string `json:"protoPayload,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Project   string            `json:"project,omitempty"`
}

type LogResource struct {
	Type   string            `json:"type"`
	Labels map[string]string `json:"labels"`
}

type HTTPRequestLog struct {
	Method  string `json:"requestMethod"`
	URL     string `json:"requestUrl"`
	Status  int    `json:"status"`
	Latency string `json:"latency"`
}

// Point is one metric sample.
type Point struct {
	T     string  `json:"t"`
	Value float64 `json:"v"`
}

// TrafficSpec declares synthetic user traffic that exercises the scenario so
// that symptoms (logs, metrics, HTTP errors) appear without the student acting.
type TrafficSpec struct {
	Name    string `json:"name" yaml:"name"`
	Target  string `json:"target" yaml:"target"` // URL, host, run:svc, k8s:cluster/svc
	Path    string `json:"path" yaml:"path"`
	RPS     int    `json:"rps" yaml:"rps"`
	Project string `json:"project" yaml:"project"`
}

// New creates an empty world with an organization, a training folder and one
// lab project where the given principal is Owner.
func New(seed int64, projectID, principal string) *State {
	s := &State{
		Seed:     seed,
		Clock:    time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
		Folders:  map[string]*Folder{},
		Projects: map[string]*Project{},
		Metrics:  map[string][]Point{},
		Images:   DefaultImages(),
		Extra:    map[string]string{},
	}
	s.init()
	s.Org = &Organization{ID: "240158832107", DisplayName: "training.gcplab.dev", OrgPolicies: map[string]*OrgPolicy{}}
	s.Folders["folders/771200"] = &Folder{ID: "771200", DisplayName: "lab-sandboxes", Parent: "organizations/240158832107"}
	p := s.NewProject(projectID, "folders/771200")
	p.IAM.Bindings = append(p.IAM.Bindings, Binding{Role: "roles/owner", Members: []string{principal}})
	return s
}

func (s *State) init() {
	if s.ipSeq == nil {
		s.ipSeq = map[string]int{}
	}
	if s.rng == nil {
		s.rng = rand.New(rand.NewSource(s.Seed))
	}
	if s.Images == nil {
		s.Images = DefaultImages()
	}
	if s.Metrics == nil {
		s.Metrics = map[string][]Point{}
	}
	if s.Extra == nil {
		s.Extra = map[string]string{}
	}
}

// Rehydrate restores non-serialized helpers after JSON decoding.
func (s *State) Rehydrate() {
	s.init()
	// Rebuild IP sequence counters from existing allocations.
	for _, p := range s.Projects {
		for _, i := range p.Instances {
			s.bumpIP(i.InternalIP)
		}
	}
}

func (s *State) bumpIP(ip string) {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return
	}
	prefix := strings.Join(parts[:3], ".")
	var last int
	fmt.Sscanf(parts[3], "%d", &last)
	if last > s.ipSeq[prefix] {
		s.ipSeq[prefix] = last
	}
}

// NewProject adds an empty project with the default set of enabled APIs.
func (s *State) NewProject(id, parent string) *Project {
	s.init()
	num := fmt.Sprintf("%d", 100000000000+s.rng.Int63n(899999999999))
	p := &Project{
		ID: id, Number: num, Name: id, Parent: parent, State: "ACTIVE",
		Labels: map[string]string{}, BillingEnabled: true,
		Services: map[string]bool{}, OrgPolicies: map[string]*OrgPolicy{},
		ServiceAccounts: map[string]*ServiceAccount{}, CustomRoles: map[string]*Role{},
		Networks: map[string]*Network{}, Subnets: map[string]*Subnet{}, Firewalls: map[string]*Firewall{},
		Routes: map[string]*Route{}, Routers: map[string]*Router{}, Addresses: map[string]*Address{},
		Instances: map[string]*Instance{}, InstanceTemplates: map[string]*InstanceTemplate{},
		InstanceGroups: map[string]*InstanceGroup{}, Disks: map[string]*Disk{}, Snapshots: map[string]*Snapshot{},
		HealthChecks: map[string]*HealthCheck{}, BackendServices: map[string]*BackendService{},
		URLMaps: map[string]*URLMap{}, TargetProxies: map[string]*TargetProxy{},
		ForwardingRules: map[string]*ForwardingRule{}, SecurityPolicies: map[string]*SecurityPolicy{},
		NEGs: map[string]*NEG{}, VPNGateways: map[string]*VPNGateway{}, DNSZones: map[string]*DNSZone{},
		Buckets: map[string]*Bucket{}, RunServices: map[string]*RunService{}, SQLInstances: map[string]*SQLInstance{},
		Topics: map[string]*Topic{}, Subs: map[string]*Subscription{}, Secrets: map[string]*Secret{},
		KeyRings: map[string]*KeyRing{}, Clusters: map[string]*Cluster{},
		ArtifactRepos: map[string]*ArtifactRepo{}, Triggers: map[string]*BuildTrigger{},
		SourceRepos: map[string]*SourceRepo{}, Pipelines: map[string]*DeliveryPipeline{},
		DeployTargets: map[string]*DeployTarget{}, Datasets: map[string]*Dataset{},
		AIModels: map[string]*AIModel{}, AIEndpoints: map[string]*AIEndpoint{},
		LogMetrics: map[string]*LogMetric{}, AlertPolicies: map[string]*AlertPolicy{},
		Channels: map[string]*NotificationChannel{}, UptimeChecks: map[string]*UptimeCheck{},
		Budgets: map[string]*Budget{}, LogSinks: map[string]*LogSink{}, Perimeters: map[string]*Perimeter{},
	}
	for _, svc := range DefaultEnabledAPIs {
		p.Services[svc] = true
	}
	// Default compute service account, like GCP creates when Compute is enabled.
	dsa := fmt.Sprintf("%s-compute@developer.gserviceaccount.com", num)
	p.ServiceAccounts[dsa] = &ServiceAccount{Email: dsa, DisplayName: "Compute Engine default service account"}
	p.IAM.Bindings = append(p.IAM.Bindings, Binding{Role: "roles/editor", Members: []string{"serviceAccount:" + dsa}})
	s.Projects[id] = p
	return p
}

// DefaultEnabledAPIs are enabled in every new lab project unless the lab
// overrides them (API allowlist per scenario).
var DefaultEnabledAPIs = []string{
	"compute.googleapis.com", "storage.googleapis.com", "iam.googleapis.com",
	"cloudresourcemanager.googleapis.com", "logging.googleapis.com", "monitoring.googleapis.com",
	"run.googleapis.com", "sqladmin.googleapis.com", "pubsub.googleapis.com",
	"secretmanager.googleapis.com", "cloudkms.googleapis.com", "container.googleapis.com",
	"cloudbuild.googleapis.com", "artifactregistry.googleapis.com", "clouddeploy.googleapis.com",
	"bigquery.googleapis.com", "aiplatform.googleapis.com", "dns.googleapis.com",
	"servicenetworking.googleapis.com", "securitycenter.googleapis.com", "sourcerepo.googleapis.com",
	"billingbudgets.googleapis.com", "accesscontextmanager.googleapis.com",
}

// Now returns the simulated clock formatted as RFC3339.
func (s *State) Now() string { return s.Clock.UTC().Format(time.RFC3339) }

// Advance moves the simulated clock forward.
func (s *State) Advance(d time.Duration) { s.Clock = s.Clock.Add(d) }

// Rand exposes the deterministic generator.
func (s *State) Rand() *rand.Rand { s.init(); return s.rng }

// ID generates a deterministic hex identifier.
func (s *State) ID(n int) string {
	s.init()
	const hex = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = hex[s.rng.Intn(16)]
	}
	return string(b)
}

// Log appends a log entry stamped with the simulated clock.
func (s *State) Log(project string, e LogEntry) {
	if e.Timestamp == "" {
		e.Timestamp = s.Now()
	}
	if e.Severity == "" {
		e.Severity = "INFO"
	}
	e.Project = project
	if !strings.HasPrefix(e.LogName, "projects/") {
		e.LogName = "projects/" + project + "/logs/" + e.LogName
	}
	s.Logs = append(s.Logs, e)
	if len(s.Logs) > 5000 {
		s.Logs = s.Logs[len(s.Logs)-5000:]
	}
}

// Audit records an Admin Activity audit log entry.
func (s *State) Audit(project, principal, service, method, resource string) {
	s.Log(project, LogEntry{
		Severity: "NOTICE",
		LogName:  "cloudaudit.googleapis.com%2Factivity",
		Resource: LogResource{Type: "audited_resource", Labels: map[string]string{"service": service, "project_id": project}},
		Proto: map[string]string{
			"@type": "type.googleapis.com/google.cloud.audit.AuditLog", "serviceName": service,
			"methodName": method, "resourceName": resource, "authenticationInfo.principalEmail": strings.TrimPrefix(strings.TrimPrefix(principal, "user:"), "serviceAccount:"),
		},
	})
}

// Metric records a sample on a named time series.
func (s *State) Metric(name string, v float64) {
	s.Metrics[name] = append(s.Metrics[name], Point{T: s.Now(), Value: v})
	if len(s.Metrics[name]) > 240 {
		s.Metrics[name] = s.Metrics[name][len(s.Metrics[name])-240:]
	}
}

// LastMetric returns the latest value of a series.
func (s *State) LastMetric(name string) (float64, bool) {
	pts := s.Metrics[name]
	if len(pts) == 0 {
		return 0, false
	}
	return pts[len(pts)-1].Value, true
}

// SortedKeys returns the keys of a map in order (for deterministic output).
func SortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Project returns a project or nil.
func (s *State) Project(id string) *Project { return s.Projects[id] }

// FindBucket looks a bucket up across all projects (global namespace).
func (s *State) FindBucket(name string) (*Bucket, *Project) {
	name = strings.TrimSuffix(strings.TrimPrefix(name, "gs://"), "/")
	for _, pid := range SortedKeys(s.Projects) {
		p := s.Projects[pid]
		if b, ok := p.Buckets[name]; ok {
			return b, p
		}
	}
	return nil, nil
}

// RegionOf returns the region for a zone ("europe-west1-b" -> "europe-west1").
func RegionOf(zone string) string {
	i := strings.LastIndex(zone, "-")
	if i < 0 {
		return zone
	}
	suffix := zone[i+1:]
	if len(suffix) == 1 {
		return zone[:i]
	}
	return zone
}

// ValidRegions is the set of regions the simulator accepts.
var ValidRegions = map[string]bool{
	"us-central1": true, "us-east1": true, "us-east4": true, "us-west1": true, "us-west2": true,
	"europe-west1": true, "europe-west2": true, "europe-west3": true, "europe-west4": true, "europe-southwest1": true,
	"europe-north1": true, "asia-east1": true, "asia-northeast1": true, "asia-southeast1": true,
	"southamerica-east1": true, "australia-southeast1": true, "northamerica-northeast1": true,
}

// ValidZone checks a zone name.
func ValidZone(z string) bool {
	r := RegionOf(z)
	if !ValidRegions[r] || r == z {
		return false
	}
	suf := z[len(r)+1:]
	return suf == "a" || suf == "b" || suf == "c" || suf == "d" || suf == "f"
}

// MultiRegions for storage/bigquery.
var MultiRegions = map[string]bool{"US": true, "EU": true, "ASIA": true}
var DualRegions = map[string]bool{"EUR4": true, "NAM4": true, "ASIA1": true}
