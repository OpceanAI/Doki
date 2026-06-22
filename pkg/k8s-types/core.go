package k8s

import "time"

// Pod represents a Kubernetes-style Pod resource, the smallest deployable
// unit that wraps one or more co-scheduled containers.
type Pod struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       PodSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     PodStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// PodList represents a collection of Pod resources returned in a list
// response.
type PodList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []Pod `json:"items" yaml:"items"`
}

// PodSpec represents the desired state of a Pod: the containers to run,
// volumes, scheduling preferences and security context.
type PodSpec struct {
	Containers                    []Container                `json:"containers" yaml:"containers"`
	InitContainers                []Container                `json:"initContainers,omitempty" yaml:"initContainers,omitempty"`
	Volumes                       []Volume                   `json:"volumes,omitempty" yaml:"volumes,omitempty"`
	ServiceAccountName            string                     `json:"serviceAccountName,omitempty" yaml:"serviceAccountName,omitempty"`
	NodeName                      string                     `json:"nodeName,omitempty" yaml:"nodeName,omitempty"`
	NodeSelector                  map[string]string          `json:"nodeSelector,omitempty" yaml:"nodeSelector,omitempty"`
	HostNetwork                   bool                       `json:"hostNetwork,omitempty" yaml:"hostNetwork,omitempty"`
	Hostname                      string                     `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	Subdomain                     string                     `json:"subdomain,omitempty" yaml:"subdomain,omitempty"`
	DNSPolicy                     string                     `json:"dnsPolicy,omitempty" yaml:"dnsPolicy,omitempty"`
	RestartPolicy                 string                     `json:"restartPolicy,omitempty" yaml:"restartPolicy,omitempty"`
	TerminationGracePeriodSeconds *int64                     `json:"terminationGracePeriodSeconds,omitempty" yaml:"terminationGracePeriodSeconds,omitempty"`
	Tolerations                   []Toleration               `json:"tolerations,omitempty" yaml:"tolerations,omitempty"`
	Affinity                      *Affinity                  `json:"affinity,omitempty" yaml:"affinity,omitempty"`
	SecurityContext               *PodSecurityContext        `json:"securityContext,omitempty" yaml:"securityContext,omitempty"`
	ImagePullSecrets              []LocalObjectReference     `json:"imagePullSecrets,omitempty" yaml:"imagePullSecrets,omitempty"`
	PriorityClassName             string                     `json:"priorityClassName,omitempty" yaml:"priorityClassName,omitempty"`
	Priority                      *int32                     `json:"priority,omitempty" yaml:"priority,omitempty"`
	RuntimeClassName              *string                    `json:"runtimeClassName,omitempty" yaml:"runtimeClassName,omitempty"`
	SchedulerName                 string                     `json:"schedulerName,omitempty" yaml:"schedulerName,omitempty"`
	ActiveDeadlineSeconds         *int64                     `json:"activeDeadlineSeconds,omitempty" yaml:"activeDeadlineSeconds,omitempty"`
	AutomountServiceAccountToken  *bool                      `json:"automountServiceAccountToken,omitempty" yaml:"automountServiceAccountToken,omitempty"`
	EnableServiceLinks            *bool                      `json:"enableServiceLinks,omitempty" yaml:"enableServiceLinks,omitempty"`
	HostAliases                   []HostAlias                `json:"hostAliases,omitempty" yaml:"hostAliases,omitempty"`
	TopologySpreadConstraints     []TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty" yaml:"topologySpreadConstraints,omitempty"`
}

// Container represents a single container running inside a Pod, including
// its image, command, ports, probes, resource requirements and security
// context.
type Container struct {
	Name            string               `json:"name" yaml:"name"`
	Image           string               `json:"image" yaml:"image"`
	Command         []string             `json:"command,omitempty" yaml:"command,omitempty"`
	Args            []string             `json:"args,omitempty" yaml:"args,omitempty"`
	WorkingDir      string               `json:"workingDir,omitempty" yaml:"workingDir,omitempty"`
	Ports           []ContainerPort      `json:"ports,omitempty" yaml:"ports,omitempty"`
	Env             []EnvVar             `json:"env,omitempty" yaml:"env,omitempty"`
	EnvFrom         []EnvFromSource      `json:"envFrom,omitempty" yaml:"envFrom,omitempty"`
	VolumeMounts    []VolumeMount        `json:"volumeMounts,omitempty" yaml:"volumeMounts,omitempty"`
	Resources       ResourceRequirements `json:"resources,omitempty" yaml:"resources,omitempty"`
	LivenessProbe   *Probe               `json:"livenessProbe,omitempty" yaml:"livenessProbe,omitempty"`
	ReadinessProbe  *Probe               `json:"readinessProbe,omitempty" yaml:"readinessProbe,omitempty"`
	StartupProbe    *Probe               `json:"startupProbe,omitempty" yaml:"startupProbe,omitempty"`
	Lifecycle       *Lifecycle           `json:"lifecycle,omitempty" yaml:"lifecycle,omitempty"`
	SecurityContext *SecurityContext     `json:"securityContext,omitempty" yaml:"securityContext,omitempty"`
	ImagePullPolicy string               `json:"imagePullPolicy,omitempty" yaml:"imagePullPolicy,omitempty"`
	Stdin           bool                 `json:"stdin,omitempty" yaml:"stdin,omitempty"`
	StdinOnce       bool                 `json:"stdinOnce,omitempty" yaml:"stdinOnce,omitempty"`
	TTY             bool                 `json:"tty,omitempty" yaml:"tty,omitempty"`
}

// ContainerPort represents a network port exposed by a container inside
// a Pod, with optional host binding and protocol.
type ContainerPort struct {
	Name          string `json:"name,omitempty" yaml:"name,omitempty"`
	ContainerPort int32  `json:"containerPort" yaml:"containerPort"`
	HostPort      int32  `json:"hostPort,omitempty" yaml:"hostPort,omitempty"`
	Protocol      string `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	HostIP        string `json:"hostIP,omitempty" yaml:"hostIP,omitempty"`
}

// EnvVar represents a single environment variable injected into a
// container, either as a literal value or sourced from a referenced object.
type EnvVar struct {
	Name      string        `json:"name" yaml:"name"`
	Value     string        `json:"value,omitempty" yaml:"value,omitempty"`
	ValueFrom *EnvVarSource `json:"valueFrom,omitempty" yaml:"valueFrom,omitempty"`
}

// EnvVarSource represents a source for an environment variable value: a
// pod field, a resource field, or a key in a ConfigMap or Secret.
type EnvVarSource struct {
	FieldRef         *ObjectFieldSelector   `json:"fieldRef,omitempty" yaml:"fieldRef,omitempty"`
	ResourceFieldRef *ResourceFieldSelector `json:"resourceFieldRef,omitempty" yaml:"resourceFieldRef,omitempty"`
	ConfigMapKeyRef  *ConfigMapKeySelector  `json:"configMapKeyRef,omitempty" yaml:"configMapKeyRef,omitempty"`
	SecretKeyRef     *SecretKeySelector     `json:"secretKeyRef,omitempty" yaml:"secretKeyRef,omitempty"`
}

// ObjectFieldSelector represents a field of a referenced Kubernetes
// object (typically the pod itself) used to populate an env value.
type ObjectFieldSelector struct {
	FieldPath  string `json:"fieldPath" yaml:"fieldPath"`
	APIVersion string `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
}

// ResourceFieldSelector represents a container resource (CPU, memory)
// whose value is exposed as an environment variable.
type ResourceFieldSelector struct {
	Resource      string `json:"resource" yaml:"resource"`
	ContainerName string `json:"containerName,omitempty" yaml:"containerName,omitempty"`
	Divisor       string `json:"divisor,omitempty" yaml:"divisor,omitempty"`
}

// ConfigMapKeySelector represents a reference to a specific key in a
// ConfigMap whose value populates an env var.
type ConfigMapKeySelector struct {
	LocalObjectReference `json:",inline" yaml:",inline"`
	Key                  string `json:"key" yaml:"key"`
	Optional             *bool  `json:"optional,omitempty" yaml:"optional,omitempty"`
}

// SecretKeySelector represents a reference to a specific key in a
// Secret whose value populates an env var.
type SecretKeySelector struct {
	LocalObjectReference `json:",inline" yaml:",inline"`
	Key                  string `json:"key" yaml:"key"`
	Optional             *bool  `json:"optional,omitempty" yaml:"optional,omitempty"`
}

// EnvFromSource represents a source of environment variables (a
// ConfigMap or Secret) to import in bulk into a container.
type EnvFromSource struct {
	Prefix       string              `json:"prefix,omitempty" yaml:"prefix,omitempty"`
	ConfigMapRef *ConfigMapEnvSource `json:"configMapRef,omitempty" yaml:"configMapRef,omitempty"`
	SecretRef    *SecretEnvSource    `json:"secretRef,omitempty" yaml:"secretRef,omitempty"`
}

// ConfigMapEnvSource represents a reference to a ConfigMap whose keys
// are imported as environment variables.
type ConfigMapEnvSource struct {
	LocalObjectReference `json:",inline" yaml:",inline"`
	Optional             *bool `json:"optional,omitempty" yaml:"optional,omitempty"`
}

// SecretEnvSource represents a reference to a Secret whose keys are
// imported as environment variables.
type SecretEnvSource struct {
	LocalObjectReference `json:",inline" yaml:",inline"`
	Optional             *bool `json:"optional,omitempty" yaml:"optional,omitempty"`
}

// VolumeMount represents a mount of a pod volume into a container at
// a specific path, with optional read-only and propagation settings.
type VolumeMount struct {
	Name             string  `json:"name" yaml:"name"`
	ReadOnly         bool    `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
	MountPath        string  `json:"mountPath" yaml:"mountPath"`
	SubPath          string  `json:"subPath,omitempty" yaml:"subPath,omitempty"`
	SubPathExpr      string  `json:"subPathExpr,omitempty" yaml:"subPathExpr,omitempty"`
	MountPropagation *string `json:"mountPropagation,omitempty" yaml:"mountPropagation,omitempty"`
}

// Volume represents a named volume that is mounted into containers
// within a pod, sourced from one of the supported volume source types.
type Volume struct {
	Name         string `json:"name" yaml:"name"`
	VolumeSource `json:",inline" yaml:",inline"`
}

// VolumeSource represents the kind of backing storage a Volume uses:
// hostPath, emptyDir, ConfigMap, Secret, PVC, projected or downwardAPI.
type VolumeSource struct {
	HostPath              *HostPathVolumeSource              `json:"hostPath,omitempty" yaml:"hostPath,omitempty"`
	EmptyDir              *EmptyDirVolumeSource              `json:"emptyDir,omitempty" yaml:"emptyDir,omitempty"`
	ConfigMap             *ConfigMapVolumeSource             `json:"configMap,omitempty" yaml:"configMap,omitempty"`
	Secret                *SecretVolumeSource                `json:"secret,omitempty" yaml:"secret,omitempty"`
	PersistentVolumeClaim *PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty" yaml:"persistentVolumeClaim,omitempty"`
	Projected             *ProjectedVolumeSource             `json:"projected,omitempty" yaml:"projected,omitempty"`
	DownwardAPI           *DownwardAPIVolumeSource           `json:"downwardAPI,omitempty" yaml:"downwardAPI,omitempty"`
}

// HostPathVolumeSource represents a host filesystem path mounted into
// a pod, with an optional type qualifier.
type HostPathVolumeSource struct {
	Path string  `json:"path" yaml:"path"`
	Type *string `json:"type,omitempty" yaml:"type,omitempty"`
}

// EmptyDirVolumeSource represents a temporary empty directory that
// lives for the duration of a pod, optionally backed by a medium.
type EmptyDirVolumeSource struct {
	Medium    string `json:"medium,omitempty" yaml:"medium,omitempty"`
	SizeLimit string `json:"sizeLimit,omitempty" yaml:"sizeLimit,omitempty"`
}

// ConfigMapVolumeSource represents a ConfigMap projected into a pod
// as a volume, optionally with per-key paths and file modes.
type ConfigMapVolumeSource struct {
	LocalObjectReference `json:",inline" yaml:",inline"`
	Items                []KeyToPath `json:"items,omitempty" yaml:"items,omitempty"`
	DefaultMode          *int32      `json:"defaultMode,omitempty" yaml:"defaultMode,omitempty"`
	Optional             *bool       `json:"optional,omitempty" yaml:"optional,omitempty"`
}

// SecretVolumeSource represents a Secret projected into a pod as a
// volume, optionally with per-key paths and file modes.
type SecretVolumeSource struct {
	SecretName  string      `json:"secretName,omitempty" yaml:"secretName,omitempty"`
	Items       []KeyToPath `json:"items,omitempty" yaml:"items,omitempty"`
	DefaultMode *int32      `json:"defaultMode,omitempty" yaml:"defaultMode,omitempty"`
	Optional    *bool       `json:"optional,omitempty" yaml:"optional,omitempty"`
}

// KeyToPath represents a mapping from a key in a ConfigMap or Secret
// to a specific file path inside a volume.
type KeyToPath struct {
	Key  string `json:"key" yaml:"key"`
	Path string `json:"path" yaml:"path"`
	Mode *int32 `json:"mode,omitempty" yaml:"mode,omitempty"`
}

// PersistentVolumeClaimVolumeSource represents a reference to a
// PersistentVolumeClaim that is mounted as a volume in a pod.
type PersistentVolumeClaimVolumeSource struct {
	ClaimName string `json:"claimName" yaml:"claimName"`
	ReadOnly  bool   `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
}

// ProjectedVolumeSource represents a list of volume sources that are
// projected into the same directory in a pod.
type ProjectedVolumeSource struct {
	Sources     []VolumeProjection `json:"sources,omitempty" yaml:"sources,omitempty"`
	DefaultMode *int32             `json:"defaultMode,omitempty" yaml:"defaultMode,omitempty"`
}

// VolumeProjection represents a single source contributing to a
// projected volume: a Secret, a ConfigMap or a DownwardAPI entry.
type VolumeProjection struct {
	Secret      *SecretProjection      `json:"secret,omitempty" yaml:"secret,omitempty"`
	ConfigMap   *ConfigMapProjection   `json:"configMap,omitempty" yaml:"configMap,omitempty"`
	DownwardAPI *DownwardAPIProjection `json:"downwardAPI,omitempty" yaml:"downwardAPI,omitempty"`
}

// SecretProjection represents a Secret to project into a projected
// volume, with optional per-key paths.
type SecretProjection struct {
	LocalObjectReference `json:",inline" yaml:",inline"`
	Items                []KeyToPath `json:"items,omitempty" yaml:"items,omitempty"`
	Optional             *bool       `json:"optional,omitempty" yaml:"optional,omitempty"`
}

// ConfigMapProjection represents a ConfigMap to project into a
// projected volume, with optional per-key paths.
type ConfigMapProjection struct {
	LocalObjectReference `json:",inline" yaml:",inline"`
	Items                []KeyToPath `json:"items,omitempty" yaml:"items,omitempty"`
	Optional             *bool       `json:"optional,omitempty" yaml:"optional,omitempty"`
}

// DownwardAPIProjection represents a list of pod or container fields
// to expose in a projected volume via the Downward API.
type DownwardAPIProjection struct {
	Items []DownwardAPIVolumeFile `json:"items,omitempty" yaml:"items,omitempty"`
}

// DownwardAPIVolumeSource represents the source of a volume populated
// with pod- or container-scoped metadata via the Downward API.
type DownwardAPIVolumeSource struct {
	Items       []DownwardAPIVolumeFile `json:"items,omitempty" yaml:"items,omitempty"`
	DefaultMode *int32                  `json:"defaultMode,omitempty" yaml:"defaultMode,omitempty"`
}

// DownwardAPIVolumeFile represents a single file written into a
// DownwardAPI volume, sourced from a pod field or a container resource.
type DownwardAPIVolumeFile struct {
	Path             string                 `json:"path" yaml:"path"`
	FieldRef         *ObjectFieldSelector   `json:"fieldRef,omitempty" yaml:"fieldRef,omitempty"`
	ResourceFieldRef *ResourceFieldSelector `json:"resourceFieldRef,omitempty" yaml:"resourceFieldRef,omitempty"`
	Mode             *int32                 `json:"mode,omitempty" yaml:"mode,omitempty"`
}

// ResourceRequirements represents the compute resource limits and
// requests (CPU, memory, etc.) requested for a container.
type ResourceRequirements struct {
	Limits   ResourceList `json:"limits,omitempty" yaml:"limits,omitempty"`
	Requests ResourceList `json:"requests,omitempty" yaml:"requests,omitempty"`
}

// ResourceList represents a map of resource names to quantity strings
// (e.g. "cpu": "100m", "memory": "256Mi").
type ResourceList map[string]string

// Probe represents a health check (liveness, readiness or startup) for
// a container, including its handler and timing parameters.
type Probe struct {
	ProbeHandler        `json:",inline" yaml:",inline"`
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty" yaml:"initialDelaySeconds,omitempty"`
	TimeoutSeconds      int32 `json:"timeoutSeconds,omitempty" yaml:"timeoutSeconds,omitempty"`
	PeriodSeconds       int32 `json:"periodSeconds,omitempty" yaml:"periodSeconds,omitempty"`
	SuccessThreshold    int32 `json:"successThreshold,omitempty" yaml:"successThreshold,omitempty"`
	FailureThreshold    int32 `json:"failureThreshold,omitempty" yaml:"failureThreshold,omitempty"`
}

// ProbeHandler represents the action executed by a Probe: exec, HTTP
// GET, TCP socket, or gRPC health check.
type ProbeHandler struct {
	Exec      *ExecAction      `json:"exec,omitempty" yaml:"exec,omitempty"`
	HTTPGet   *HTTPGetAction   `json:"httpGet,omitempty" yaml:"httpGet,omitempty"`
	TCPSocket *TCPSocketAction `json:"tcpSocket,omitempty" yaml:"tcpSocket,omitempty"`
	GRPC      *GRPCAction      `json:"grpc,omitempty" yaml:"grpc,omitempty"`
}

// ExecAction represents a command executed inside a container, used
// for probes and lifecycle hooks.
type ExecAction struct {
	Command []string `json:"command,omitempty" yaml:"command,omitempty"`
}

// HTTPGetAction represents an HTTP GET probe or hook against a path
// and port inside (or outside) the container.
type HTTPGetAction struct {
	Path        string       `json:"path,omitempty" yaml:"path,omitempty"`
	Port        IntOrString  `json:"port" yaml:"port"`
	Host        string       `json:"host,omitempty" yaml:"host,omitempty"`
	Scheme      string       `json:"scheme,omitempty" yaml:"scheme,omitempty"`
	HTTPHeaders []HTTPHeader `json:"httpHeaders,omitempty" yaml:"httpHeaders,omitempty"`
}

// HTTPHeader represents a custom header attached to an HTTP probe or
// lifecycle HTTP request.
type HTTPHeader struct {
	Name  string `json:"name" yaml:"name"`
	Value string `json:"value" yaml:"value"`
}

// TCPSocketAction represents a TCP socket probe or hook against a
// port inside (or outside) the container.
type TCPSocketAction struct {
	Port IntOrString `json:"port" yaml:"port"`
	Host string      `json:"host,omitempty" yaml:"host,omitempty"`
}

// GRPCAction represents a gRPC health-check probe against a service
// exposed by a container.
type GRPCAction struct {
	Port    int32   `json:"port" yaml:"port"`
	Service *string `json:"service,omitempty" yaml:"service,omitempty"`
}

// IntOrString represents a value that can be encoded either as an
// integer or a string (for fields like ports or quantities).
type IntOrString struct {
	Type   int    `json:"type"`
	IntVal int32  `json:"intVal,omitempty"`
	StrVal string `json:"strVal,omitempty"`
}

// Lifecycle represents the lifecycle hooks (postStart, preStop) that
// a container can register with the runtime.
type Lifecycle struct {
	PostStart *LifecycleHandler `json:"postStart,omitempty" yaml:"postStart,omitempty"`
	PreStop   *LifecycleHandler `json:"preStop,omitempty" yaml:"preStop,omitempty"`
}

// LifecycleHandler represents a single lifecycle hook action, using
// one of exec, HTTP GET or TCP socket invocation modes.
type LifecycleHandler struct {
	Exec      *ExecAction      `json:"exec,omitempty" yaml:"exec,omitempty"`
	HTTPGet   *HTTPGetAction   `json:"httpGet,omitempty" yaml:"httpGet,omitempty"`
	TCPSocket *TCPSocketAction `json:"tcpSocket,omitempty" yaml:"tcpSocket,omitempty"`
}

// Toleration represents a pod's tolerance for a node taint, allowing
// the pod to be scheduled on (or kept running on) tainted nodes.
type Toleration struct {
	Key               string `json:"key,omitempty" yaml:"key,omitempty"`
	Operator          string `json:"operator,omitempty" yaml:"operator,omitempty"`
	Value             string `json:"value,omitempty" yaml:"value,omitempty"`
	Effect            string `json:"effect,omitempty" yaml:"effect,omitempty"`
	TolerationSeconds *int64 `json:"tolerationSeconds,omitempty" yaml:"tolerationSeconds,omitempty"`
}

// Affinity represents the scheduling affinity rules for a pod, broken
// down into node, pod and pod-anti affinity.
type Affinity struct {
	NodeAffinity    *NodeAffinity    `json:"nodeAffinity,omitempty" yaml:"nodeAffinity,omitempty"`
	PodAffinity     *PodAffinity     `json:"podAffinity,omitempty" yaml:"podAffinity,omitempty"`
	PodAntiAffinity *PodAntiAffinity `json:"podAntiAffinity,omitempty" yaml:"podAntiAffinity,omitempty"`
}

// NodeAffinity represents the node affinity rules applied to a pod
// during scheduling.
type NodeAffinity struct {
	RequiredDuringSchedulingIgnoredDuringExecution  *NodeSelector             `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty" yaml:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`
	PreferredDuringSchedulingIgnoredDuringExecution []PreferredSchedulingTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty" yaml:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

// NodeSelector represents a list of node selector terms, any of which
// must be satisfied for a pod to be scheduled on a node.
type NodeSelector struct {
	NodeSelectorTerms []NodeSelectorTerm `json:"nodeSelectorTerms" yaml:"nodeSelectorTerms"`
}

// NodeSelectorTerm represents a single term in a NodeSelector, with
// a set of match expressions and field match requirements.
type NodeSelectorTerm struct {
	MatchExpressions []NodeSelectorRequirement `json:"matchExpressions,omitempty" yaml:"matchExpressions,omitempty"`
	MatchFields      []NodeSelectorRequirement `json:"matchFields,omitempty" yaml:"matchFields,omitempty"`
}

// NodeSelectorRequirement represents a single key/operator/values
// rule used to match a node label or field.
type NodeSelectorRequirement struct {
	Key      string   `json:"key" yaml:"key"`
	Operator string   `json:"operator" yaml:"operator"`
	Values   []string `json:"values,omitempty" yaml:"values,omitempty"`
}

// PreferredSchedulingTerm represents a node selection preference with
// a weight, used by NodeAffinity's preferred-during-scheduling rules.
type PreferredSchedulingTerm struct {
	Weight     int32            `json:"weight" yaml:"weight"`
	Preference NodeSelectorTerm `json:"preference" yaml:"preference"`
}

// PodAffinity represents the pod affinity rules applied to a pod
// during scheduling, indicating which pods it should be co-located with.
type PodAffinity struct {
	RequiredDuringSchedulingIgnoredDuringExecution  []PodAffinityTerm         `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty" yaml:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`
	PreferredDuringSchedulingIgnoredDuringExecution []WeightedPodAffinityTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty" yaml:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

// PodAntiAffinity represents the pod anti-affinity rules applied to a
// pod during scheduling, indicating which pods it should avoid.
type PodAntiAffinity struct {
	RequiredDuringSchedulingIgnoredDuringExecution  []PodAffinityTerm         `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty" yaml:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`
	PreferredDuringSchedulingIgnoredDuringExecution []WeightedPodAffinityTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty" yaml:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

// PodAffinityTerm represents a single pod affinity or anti-affinity
// rule, including its label selector and topology key.
type PodAffinityTerm struct {
	LabelSelector *LabelSelector `json:"labelSelector,omitempty" yaml:"labelSelector,omitempty"`
	TopologyKey   string         `json:"topologyKey" yaml:"topologyKey"`
	Namespaces    []string       `json:"namespaces,omitempty" yaml:"namespaces,omitempty"`
}

// WeightedPodAffinityTerm represents a preferred pod affinity rule
// with an associated scheduling weight.
type WeightedPodAffinityTerm struct {
	Weight          int32           `json:"weight" yaml:"weight"`
	PodAffinityTerm PodAffinityTerm `json:"podAffinityTerm" yaml:"podAffinityTerm"`
}

// PodSecurityContext represents security settings applied at the pod
// level (UID, GID, fs group, sysctls, etc.).
type PodSecurityContext struct {
	RunAsUser          *int64   `json:"runAsUser,omitempty" yaml:"runAsUser,omitempty"`
	RunAsGroup         *int64   `json:"runAsGroup,omitempty" yaml:"runAsGroup,omitempty"`
	RunAsNonRoot       *bool    `json:"runAsNonRoot,omitempty" yaml:"runAsNonRoot,omitempty"`
	FSGroup            *int64   `json:"fsGroup,omitempty" yaml:"fsGroup,omitempty"`
	SupplementalGroups []int64  `json:"supplementalGroups,omitempty" yaml:"supplementalGroups,omitempty"`
	Sysctls            []Sysctl `json:"sysctls,omitempty" yaml:"sysctls,omitempty"`
}

// SecurityContext represents security settings applied at the
// container level (UID, GID, capabilities, seccomp, etc.).
type SecurityContext struct {
	RunAsUser                *int64          `json:"runAsUser,omitempty" yaml:"runAsUser,omitempty"`
	RunAsGroup               *int64          `json:"runAsGroup,omitempty" yaml:"runAsGroup,omitempty"`
	RunAsNonRoot             *bool           `json:"runAsNonRoot,omitempty" yaml:"runAsNonRoot,omitempty"`
	ReadOnlyRootFilesystem   *bool           `json:"readOnlyRootFilesystem,omitempty" yaml:"readOnlyRootFilesystem,omitempty"`
	Privileged               *bool           `json:"privileged,omitempty" yaml:"privileged,omitempty"`
	AllowPrivilegeEscalation *bool           `json:"allowPrivilegeEscalation,omitempty" yaml:"allowPrivilegeEscalation,omitempty"`
	Capabilities             *Capabilities   `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	SELinuxOptions           *SELinuxOptions `json:"seLinuxOptions,omitempty" yaml:"seLinuxOptions,omitempty"`
	SeccompProfile           *SeccompProfile `json:"seccompProfile,omitempty" yaml:"seccompProfile,omitempty"`
	ProcMount                *string         `json:"procMount,omitempty" yaml:"procMount,omitempty"`
}

// Capabilities represents the set of Linux capabilities to add or
// drop for a container.
type Capabilities struct {
	Add  []string `json:"add,omitempty" yaml:"add,omitempty"`
	Drop []string `json:"drop,omitempty" yaml:"drop,omitempty"`
}

// SELinuxOptions represents the SELinux labels applied to a container.
type SELinuxOptions struct {
	User  string `json:"user,omitempty" yaml:"user,omitempty"`
	Role  string `json:"role,omitempty" yaml:"role,omitempty"`
	Type  string `json:"type,omitempty" yaml:"type,omitempty"`
	Level string `json:"level,omitempty" yaml:"level,omitempty"`
}

// SeccompProfile represents a seccomp profile applied to a container,
// either a runtime default or a localhost-hosted profile.
type SeccompProfile struct {
	Type             string  `json:"type" yaml:"type"`
	LocalhostProfile *string `json:"localhostProfile,omitempty" yaml:"localhostProfile,omitempty"`
}

// Sysctl represents a single kernel sysctl setting applied via the
// pod security context.
type Sysctl struct {
	Name  string `json:"name" yaml:"name"`
	Value string `json:"value" yaml:"value"`
}

// HostAlias represents an entry added to a pod's /etc/hosts mapping
// an IP to one or more hostnames.
type HostAlias struct {
	IP        string   `json:"ip" yaml:"ip"`
	Hostnames []string `json:"hostnames,omitempty" yaml:"hostnames,omitempty"`
}

// TopologySpreadConstraint represents a constraint that tries to
// spread pods across failure domains such as nodes, racks or zones.
type TopologySpreadConstraint struct {
	MaxSkew           int32          `json:"maxSkew" yaml:"maxSkew"`
	TopologyKey       string         `json:"topologyKey" yaml:"topologyKey"`
	WhenUnsatisfiable string         `json:"whenUnsatisfiable" yaml:"whenUnsatisfiable"`
	LabelSelector     *LabelSelector `json:"labelSelector,omitempty" yaml:"labelSelector,omitempty"`
	MinDomains        *int32         `json:"minDomains,omitempty" yaml:"minDomains,omitempty"`
}

// PodStatus represents the observed state of a Pod: its phase,
// conditions, IP addresses and per-container statuses.
type PodStatus struct {
	Phase                 string            `json:"phase,omitempty" yaml:"phase,omitempty"`
	Conditions            []PodCondition    `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	Message               string            `json:"message,omitempty" yaml:"message,omitempty"`
	Reason                string            `json:"reason,omitempty" yaml:"reason,omitempty"`
	HostIP                string            `json:"hostIP,omitempty" yaml:"hostIP,omitempty"`
	HostIPs               []HostIP          `json:"hostIPs,omitempty" yaml:"hostIPs,omitempty"`
	PodIP                 string            `json:"podIP,omitempty" yaml:"podIP,omitempty"`
	PodIPs                []PodIP           `json:"podIPs,omitempty" yaml:"podIPs,omitempty"`
	StartTime             *time.Time        `json:"startTime,omitempty" yaml:"startTime,omitempty"`
	InitContainerStatuses []ContainerStatus `json:"initContainerStatuses,omitempty" yaml:"initContainerStatuses,omitempty"`
	ContainerStatuses     []ContainerStatus `json:"containerStatuses,omitempty" yaml:"containerStatuses,omitempty"`
	QOSClass              string            `json:"qosClass,omitempty" yaml:"qosClass,omitempty"`
	NominatedNodeName     string            `json:"nominatedNodeName,omitempty" yaml:"nominatedNodeName,omitempty"`
}

// PodCondition represents a single condition attached to a Pod's
// status, describing a transition the pod has gone through.
type PodCondition struct {
	Type               string    `json:"type" yaml:"type"`
	Status             string    `json:"status" yaml:"status"`
	LastProbeTime      time.Time `json:"lastProbeTime,omitempty" yaml:"lastProbeTime,omitempty"`
	LastTransitionTime time.Time `json:"lastTransitionTime,omitempty" yaml:"lastTransitionTime,omitempty"`
	Reason             string    `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message            string    `json:"message,omitempty" yaml:"message,omitempty"`
}

// HostIP represents the IP address of the host on which a pod is
// running, as exposed in PodStatus.
type HostIP struct {
	IP string `json:"ip" yaml:"ip"`
}

// PodIP represents a single IP address assigned to a pod, as exposed
// in PodStatus (one entry per IP family).
type PodIP struct {
	IP string `json:"ip" yaml:"ip"`
}

// ContainerStatus represents the observed state of a single container
// inside a pod, including its current and last-termination state.
type ContainerStatus struct {
	Name                 string         `json:"name" yaml:"name"`
	State                ContainerState `json:"state,omitempty" yaml:"state,omitempty"`
	LastTerminationState ContainerState `json:"lastState,omitempty" yaml:"lastState,omitempty"`
	Ready                bool           `json:"ready" yaml:"ready"`
	RestartCount         int32          `json:"restartCount" yaml:"restartCount"`
	Image                string         `json:"image" yaml:"image"`
	ImageID              string         `json:"imageID" yaml:"imageID"`
	ContainerID          string         `json:"containerID,omitempty" yaml:"containerID,omitempty"`
	Started              *bool          `json:"started,omitempty" yaml:"started,omitempty"`
	AllocatedResources   ResourceList   `json:"allocatedResources,omitempty" yaml:"allocatedResources,omitempty"`
}

// ContainerState represents the lifecycle state of a container:
// waiting, running or terminated.
type ContainerState struct {
	Waiting    *ContainerStateWaiting    `json:"waiting,omitempty" yaml:"waiting,omitempty"`
	Running    *ContainerStateRunning    `json:"running,omitempty" yaml:"running,omitempty"`
	Terminated *ContainerStateTerminated `json:"terminated,omitempty" yaml:"terminated,omitempty"`
}

// ContainerStateWaiting represents a container that is in the waiting
// state, including the reason and a human-readable message.
type ContainerStateWaiting struct {
	Reason  string `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

// ContainerStateRunning represents a container that is currently
// running, exposing its start time.
type ContainerStateRunning struct {
	StartedAt time.Time `json:"startedAt,omitempty" yaml:"startedAt,omitempty"`
}

// ContainerStateTerminated represents a container that has terminated,
// including its exit code, signal and timing information.
type ContainerStateTerminated struct {
	ExitCode    int32     `json:"exitCode" yaml:"exitCode"`
	Signal      int32     `json:"signal,omitempty" yaml:"signal,omitempty"`
	Reason      string    `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message     string    `json:"message,omitempty" yaml:"message,omitempty"`
	StartedAt   time.Time `json:"startedAt,omitempty" yaml:"startedAt,omitempty"`
	FinishedAt  time.Time `json:"finishedAt,omitempty" yaml:"finishedAt,omitempty"`
	ContainerID string    `json:"containerID,omitempty" yaml:"containerID,omitempty"`
}

const (
	// PodPending indicates the pod has been accepted by the system
	// but one or more containers have not yet started.
	PodPending = "Pending"
	// PodRunning indicates the pod has been bound to a node and all
	// of its containers have been created.
	PodRunning = "Running"
	// PodSucceeded indicates that all containers in the pod have
	// voluntarily terminated with a zero exit code.
	PodSucceeded = "Succeeded"
	// PodFailed indicates that at least one container has terminated
	// in a non-zero state or been terminated by the system.
	PodFailed = "Failed"
	// PodUnknown indicates the state of the pod could not be obtained.
	PodUnknown = "Unknown"
)
