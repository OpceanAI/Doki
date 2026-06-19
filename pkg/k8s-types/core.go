package k8s

import "time"

type Pod struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       PodSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     PodStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type PodList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []Pod `json:"items" yaml:"items"`
}

type PodSpec struct {
	Containers         []Container          `json:"containers" yaml:"containers"`
	InitContainers     []Container          `json:"initContainers,omitempty" yaml:"initContainers,omitempty"`
	Volumes            []Volume             `json:"volumes,omitempty" yaml:"volumes,omitempty"`
	ServiceAccountName string               `json:"serviceAccountName,omitempty" yaml:"serviceAccountName,omitempty"`
	NodeName           string               `json:"nodeName,omitempty" yaml:"nodeName,omitempty"`
	NodeSelector       map[string]string    `json:"nodeSelector,omitempty" yaml:"nodeSelector,omitempty"`
	HostNetwork        bool                 `json:"hostNetwork,omitempty" yaml:"hostNetwork,omitempty"`
	Hostname           string               `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	Subdomain          string               `json:"subdomain,omitempty" yaml:"subdomain,omitempty"`
	DNSPolicy          string               `json:"dnsPolicy,omitempty" yaml:"dnsPolicy,omitempty"`
	RestartPolicy      string               `json:"restartPolicy,omitempty" yaml:"restartPolicy,omitempty"`
	TerminationGracePeriodSeconds *int64    `json:"terminationGracePeriodSeconds,omitempty" yaml:"terminationGracePeriodSeconds,omitempty"`
	Tolerations        []Toleration         `json:"tolerations,omitempty" yaml:"tolerations,omitempty"`
	Affinity           *Affinity            `json:"affinity,omitempty" yaml:"affinity,omitempty"`
	SecurityContext    *PodSecurityContext   `json:"securityContext,omitempty" yaml:"securityContext,omitempty"`
	ImagePullSecrets   []LocalObjectReference `json:"imagePullSecrets,omitempty" yaml:"imagePullSecrets,omitempty"`
	PriorityClassName  string               `json:"priorityClassName,omitempty" yaml:"priorityClassName,omitempty"`
	Priority           *int32               `json:"priority,omitempty" yaml:"priority,omitempty"`
	RuntimeClassName   *string              `json:"runtimeClassName,omitempty" yaml:"runtimeClassName,omitempty"`
	SchedulerName      string               `json:"schedulerName,omitempty" yaml:"schedulerName,omitempty"`
	ActiveDeadlineSeconds *int64            `json:"activeDeadlineSeconds,omitempty" yaml:"activeDeadlineSeconds,omitempty"`
	AutomountServiceAccountToken *bool      `json:"automountServiceAccountToken,omitempty" yaml:"automountServiceAccountToken,omitempty"`
	EnableServiceLinks *bool                `json:"enableServiceLinks,omitempty" yaml:"enableServiceLinks,omitempty"`
	HostAliases        []HostAlias          `json:"hostAliases,omitempty" yaml:"hostAliases,omitempty"`
	TopologySpreadConstraints []TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty" yaml:"topologySpreadConstraints,omitempty"`
}

type Container struct {
	Name            string                 `json:"name" yaml:"name"`
	Image           string                 `json:"image" yaml:"image"`
	Command         []string               `json:"command,omitempty" yaml:"command,omitempty"`
	Args            []string               `json:"args,omitempty" yaml:"args,omitempty"`
	WorkingDir      string                 `json:"workingDir,omitempty" yaml:"workingDir,omitempty"`
	Ports           []ContainerPort        `json:"ports,omitempty" yaml:"ports,omitempty"`
	Env             []EnvVar               `json:"env,omitempty" yaml:"env,omitempty"`
	EnvFrom         []EnvFromSource        `json:"envFrom,omitempty" yaml:"envFrom,omitempty"`
	VolumeMounts    []VolumeMount          `json:"volumeMounts,omitempty" yaml:"volumeMounts,omitempty"`
	Resources       ResourceRequirements   `json:"resources,omitempty" yaml:"resources,omitempty"`
	LivenessProbe   *Probe                 `json:"livenessProbe,omitempty" yaml:"livenessProbe,omitempty"`
	ReadinessProbe  *Probe                 `json:"readinessProbe,omitempty" yaml:"readinessProbe,omitempty"`
	StartupProbe    *Probe                 `json:"startupProbe,omitempty" yaml:"startupProbe,omitempty"`
	Lifecycle       *Lifecycle             `json:"lifecycle,omitempty" yaml:"lifecycle,omitempty"`
	SecurityContext *SecurityContext        `json:"securityContext,omitempty" yaml:"securityContext,omitempty"`
	ImagePullPolicy string                 `json:"imagePullPolicy,omitempty" yaml:"imagePullPolicy,omitempty"`
	Stdin           bool                   `json:"stdin,omitempty" yaml:"stdin,omitempty"`
	StdinOnce       bool                   `json:"stdinOnce,omitempty" yaml:"stdinOnce,omitempty"`
	TTY             bool                   `json:"tty,omitempty" yaml:"tty,omitempty"`
}

type ContainerPort struct {
	Name          string `json:"name,omitempty" yaml:"name,omitempty"`
	ContainerPort int32  `json:"containerPort" yaml:"containerPort"`
	HostPort      int32  `json:"hostPort,omitempty" yaml:"hostPort,omitempty"`
	Protocol      string `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	HostIP        string `json:"hostIP,omitempty" yaml:"hostIP,omitempty"`
}

type EnvVar struct {
	Name      string        `json:"name" yaml:"name"`
	Value     string        `json:"value,omitempty" yaml:"value,omitempty"`
	ValueFrom *EnvVarSource `json:"valueFrom,omitempty" yaml:"valueFrom,omitempty"`
}

type EnvVarSource struct {
	FieldRef         *ObjectFieldSelector   `json:"fieldRef,omitempty" yaml:"fieldRef,omitempty"`
	ResourceFieldRef *ResourceFieldSelector `json:"resourceFieldRef,omitempty" yaml:"resourceFieldRef,omitempty"`
	ConfigMapKeyRef  *ConfigMapKeySelector  `json:"configMapKeyRef,omitempty" yaml:"configMapKeyRef,omitempty"`
	SecretKeyRef     *SecretKeySelector     `json:"secretKeyRef,omitempty" yaml:"secretKeyRef,omitempty"`
}

type ObjectFieldSelector struct {
	FieldPath  string `json:"fieldPath" yaml:"fieldPath"`
	APIVersion string `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
}

type ResourceFieldSelector struct {
	Resource   string `json:"resource" yaml:"resource"`
	ContainerName string `json:"containerName,omitempty" yaml:"containerName,omitempty"`
	Divisor    string `json:"divisor,omitempty" yaml:"divisor,omitempty"`
}

type ConfigMapKeySelector struct {
	LocalObjectReference `json:",inline" yaml:",inline"`
	Key                  string `json:"key" yaml:"key"`
	Optional             *bool  `json:"optional,omitempty" yaml:"optional,omitempty"`
}

type SecretKeySelector struct {
	LocalObjectReference `json:",inline" yaml:",inline"`
	Key                  string `json:"key" yaml:"key"`
	Optional             *bool  `json:"optional,omitempty" yaml:"optional,omitempty"`
}

type EnvFromSource struct {
	Prefix       string              `json:"prefix,omitempty" yaml:"prefix,omitempty"`
	ConfigMapRef *ConfigMapEnvSource `json:"configMapRef,omitempty" yaml:"configMapRef,omitempty"`
	SecretRef    *SecretEnvSource    `json:"secretRef,omitempty" yaml:"secretRef,omitempty"`
}

type ConfigMapEnvSource struct {
	LocalObjectReference `json:",inline" yaml:",inline"`
	Optional             *bool `json:"optional,omitempty" yaml:"optional,omitempty"`
}

type SecretEnvSource struct {
	LocalObjectReference `json:",inline" yaml:",inline"`
	Optional             *bool `json:"optional,omitempty" yaml:"optional,omitempty"`
}

type VolumeMount struct {
	Name             string `json:"name" yaml:"name"`
	ReadOnly         bool   `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
	MountPath        string `json:"mountPath" yaml:"mountPath"`
	SubPath          string `json:"subPath,omitempty" yaml:"subPath,omitempty"`
	SubPathExpr      string `json:"subPathExpr,omitempty" yaml:"subPathExpr,omitempty"`
	MountPropagation *string `json:"mountPropagation,omitempty" yaml:"mountPropagation,omitempty"`
}

type Volume struct {
	Name                  string `json:"name" yaml:"name"`
	VolumeSource          `json:",inline" yaml:",inline"`
}

type VolumeSource struct {
	HostPath              *HostPathVolumeSource              `json:"hostPath,omitempty" yaml:"hostPath,omitempty"`
	EmptyDir              *EmptyDirVolumeSource              `json:"emptyDir,omitempty" yaml:"emptyDir,omitempty"`
	ConfigMap             *ConfigMapVolumeSource             `json:"configMap,omitempty" yaml:"configMap,omitempty"`
	Secret                *SecretVolumeSource                `json:"secret,omitempty" yaml:"secret,omitempty"`
	PersistentVolumeClaim *PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty" yaml:"persistentVolumeClaim,omitempty"`
	Projected             *ProjectedVolumeSource             `json:"projected,omitempty" yaml:"projected,omitempty"`
	DownwardAPI           *DownwardAPIVolumeSource           `json:"downwardAPI,omitempty" yaml:"downwardAPI,omitempty"`
}

type HostPathVolumeSource struct {
	Path string  `json:"path" yaml:"path"`
	Type *string `json:"type,omitempty" yaml:"type,omitempty"`
}

type EmptyDirVolumeSource struct {
	Medium    string `json:"medium,omitempty" yaml:"medium,omitempty"`
	SizeLimit string `json:"sizeLimit,omitempty" yaml:"sizeLimit,omitempty"`
}

type ConfigMapVolumeSource struct {
	LocalObjectReference `json:",inline" yaml:",inline"`
	Items                []KeyToPath `json:"items,omitempty" yaml:"items,omitempty"`
	DefaultMode          *int32      `json:"defaultMode,omitempty" yaml:"defaultMode,omitempty"`
	Optional             *bool       `json:"optional,omitempty" yaml:"optional,omitempty"`
}

type SecretVolumeSource struct {
	SecretName  string      `json:"secretName,omitempty" yaml:"secretName,omitempty"`
	Items       []KeyToPath `json:"items,omitempty" yaml:"items,omitempty"`
	DefaultMode *int32      `json:"defaultMode,omitempty" yaml:"defaultMode,omitempty"`
	Optional    *bool       `json:"optional,omitempty" yaml:"optional,omitempty"`
}

type KeyToPath struct {
	Key  string `json:"key" yaml:"key"`
	Path string `json:"path" yaml:"path"`
	Mode *int32 `json:"mode,omitempty" yaml:"mode,omitempty"`
}

type PersistentVolumeClaimVolumeSource struct {
	ClaimName string `json:"claimName" yaml:"claimName"`
	ReadOnly  bool   `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
}

type ProjectedVolumeSource struct {
	Sources     []VolumeProjection `json:"sources,omitempty" yaml:"sources,omitempty"`
	DefaultMode *int32             `json:"defaultMode,omitempty" yaml:"defaultMode,omitempty"`
}

type VolumeProjection struct {
	Secret      *SecretProjection      `json:"secret,omitempty" yaml:"secret,omitempty"`
	ConfigMap   *ConfigMapProjection   `json:"configMap,omitempty" yaml:"configMap,omitempty"`
	DownwardAPI *DownwardAPIProjection `json:"downwardAPI,omitempty" yaml:"downwardAPI,omitempty"`
}

type SecretProjection struct {
	LocalObjectReference `json:",inline" yaml:",inline"`
	Items                []KeyToPath `json:"items,omitempty" yaml:"items,omitempty"`
	Optional             *bool       `json:"optional,omitempty" yaml:"optional,omitempty"`
}

type ConfigMapProjection struct {
	LocalObjectReference `json:",inline" yaml:",inline"`
	Items                []KeyToPath `json:"items,omitempty" yaml:"items,omitempty"`
	Optional             *bool       `json:"optional,omitempty" yaml:"optional,omitempty"`
}

type DownwardAPIProjection struct {
	Items []DownwardAPIVolumeFile `json:"items,omitempty" yaml:"items,omitempty"`
}

type DownwardAPIVolumeSource struct {
	Items       []DownwardAPIVolumeFile `json:"items,omitempty" yaml:"items,omitempty"`
	DefaultMode *int32                  `json:"defaultMode,omitempty" yaml:"defaultMode,omitempty"`
}

type DownwardAPIVolumeFile struct {
	Path             string                `json:"path" yaml:"path"`
	FieldRef         *ObjectFieldSelector  `json:"fieldRef,omitempty" yaml:"fieldRef,omitempty"`
	ResourceFieldRef *ResourceFieldSelector `json:"resourceFieldRef,omitempty" yaml:"resourceFieldRef,omitempty"`
	Mode             *int32                `json:"mode,omitempty" yaml:"mode,omitempty"`
}

type ResourceRequirements struct {
	Limits   ResourceList `json:"limits,omitempty" yaml:"limits,omitempty"`
	Requests ResourceList `json:"requests,omitempty" yaml:"requests,omitempty"`
}

type ResourceList map[string]string

type Probe struct {
	ProbeHandler        `json:",inline" yaml:",inline"`
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty" yaml:"initialDelaySeconds,omitempty"`
	TimeoutSeconds      int32 `json:"timeoutSeconds,omitempty" yaml:"timeoutSeconds,omitempty"`
	PeriodSeconds       int32 `json:"periodSeconds,omitempty" yaml:"periodSeconds,omitempty"`
	SuccessThreshold    int32 `json:"successThreshold,omitempty" yaml:"successThreshold,omitempty"`
	FailureThreshold    int32 `json:"failureThreshold,omitempty" yaml:"failureThreshold,omitempty"`
}

type ProbeHandler struct {
	Exec      *ExecAction      `json:"exec,omitempty" yaml:"exec,omitempty"`
	HTTPGet   *HTTPGetAction   `json:"httpGet,omitempty" yaml:"httpGet,omitempty"`
	TCPSocket *TCPSocketAction `json:"tcpSocket,omitempty" yaml:"tcpSocket,omitempty"`
	GRPC      *GRPCAction      `json:"grpc,omitempty" yaml:"grpc,omitempty"`
}

type ExecAction struct {
	Command []string `json:"command,omitempty" yaml:"command,omitempty"`
}

type HTTPGetAction struct {
	Path        string        `json:"path,omitempty" yaml:"path,omitempty"`
	Port        IntOrString   `json:"port" yaml:"port"`
	Host        string        `json:"host,omitempty" yaml:"host,omitempty"`
	Scheme      string        `json:"scheme,omitempty" yaml:"scheme,omitempty"`
	HTTPHeaders []HTTPHeader  `json:"httpHeaders,omitempty" yaml:"httpHeaders,omitempty"`
}

type HTTPHeader struct {
	Name  string `json:"name" yaml:"name"`
	Value string `json:"value" yaml:"value"`
}

type TCPSocketAction struct {
	Port IntOrString `json:"port" yaml:"port"`
	Host string      `json:"host,omitempty" yaml:"host,omitempty"`
}

type GRPCAction struct {
	Port    int32   `json:"port" yaml:"port"`
	Service *string `json:"service,omitempty" yaml:"service,omitempty"`
}

type IntOrString struct {
	Type   int    `json:"type"`
	IntVal int32  `json:"intVal,omitempty"`
	StrVal string `json:"strVal,omitempty"`
}

type Lifecycle struct {
	PostStart *LifecycleHandler `json:"postStart,omitempty" yaml:"postStart,omitempty"`
	PreStop   *LifecycleHandler `json:"preStop,omitempty" yaml:"preStop,omitempty"`
}

type LifecycleHandler struct {
	Exec      *ExecAction      `json:"exec,omitempty" yaml:"exec,omitempty"`
	HTTPGet   *HTTPGetAction   `json:"httpGet,omitempty" yaml:"httpGet,omitempty"`
	TCPSocket *TCPSocketAction `json:"tcpSocket,omitempty" yaml:"tcpSocket,omitempty"`
}

type Toleration struct {
	Key               string `json:"key,omitempty" yaml:"key,omitempty"`
	Operator          string `json:"operator,omitempty" yaml:"operator,omitempty"`
	Value             string `json:"value,omitempty" yaml:"value,omitempty"`
	Effect            string `json:"effect,omitempty" yaml:"effect,omitempty"`
	TolerationSeconds *int64 `json:"tolerationSeconds,omitempty" yaml:"tolerationSeconds,omitempty"`
}

type Affinity struct {
	NodeAffinity    *NodeAffinity    `json:"nodeAffinity,omitempty" yaml:"nodeAffinity,omitempty"`
	PodAffinity     *PodAffinity     `json:"podAffinity,omitempty" yaml:"podAffinity,omitempty"`
	PodAntiAffinity *PodAntiAffinity `json:"podAntiAffinity,omitempty" yaml:"podAntiAffinity,omitempty"`
}

type NodeAffinity struct {
	RequiredDuringSchedulingIgnoredDuringExecution  *NodeSelector             `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty" yaml:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`
	PreferredDuringSchedulingIgnoredDuringExecution []PreferredSchedulingTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty" yaml:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

type NodeSelector struct {
	NodeSelectorTerms []NodeSelectorTerm `json:"nodeSelectorTerms" yaml:"nodeSelectorTerms"`
}

type NodeSelectorTerm struct {
	MatchExpressions []NodeSelectorRequirement `json:"matchExpressions,omitempty" yaml:"matchExpressions,omitempty"`
	MatchFields      []NodeSelectorRequirement `json:"matchFields,omitempty" yaml:"matchFields,omitempty"`
}

type NodeSelectorRequirement struct {
	Key      string   `json:"key" yaml:"key"`
	Operator string   `json:"operator" yaml:"operator"`
	Values   []string `json:"values,omitempty" yaml:"values,omitempty"`
}

type PreferredSchedulingTerm struct {
	Weight     int32            `json:"weight" yaml:"weight"`
	Preference NodeSelectorTerm `json:"preference" yaml:"preference"`
}

type PodAffinity struct {
	RequiredDuringSchedulingIgnoredDuringExecution  []PodAffinityTerm         `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty" yaml:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`
	PreferredDuringSchedulingIgnoredDuringExecution []WeightedPodAffinityTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty" yaml:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

type PodAntiAffinity struct {
	RequiredDuringSchedulingIgnoredDuringExecution  []PodAffinityTerm         `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty" yaml:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`
	PreferredDuringSchedulingIgnoredDuringExecution []WeightedPodAffinityTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty" yaml:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

type PodAffinityTerm struct {
	LabelSelector *LabelSelector `json:"labelSelector,omitempty" yaml:"labelSelector,omitempty"`
	TopologyKey   string         `json:"topologyKey" yaml:"topologyKey"`
	Namespaces    []string       `json:"namespaces,omitempty" yaml:"namespaces,omitempty"`
}

type WeightedPodAffinityTerm struct {
	Weight          int32           `json:"weight" yaml:"weight"`
	PodAffinityTerm PodAffinityTerm `json:"podAffinityTerm" yaml:"podAffinityTerm"`
}

type PodSecurityContext struct {
	RunAsUser    *int64 `json:"runAsUser,omitempty" yaml:"runAsUser,omitempty"`
	RunAsGroup   *int64 `json:"runAsGroup,omitempty" yaml:"runAsGroup,omitempty"`
	RunAsNonRoot *bool  `json:"runAsNonRoot,omitempty" yaml:"runAsNonRoot,omitempty"`
	FSGroup      *int64 `json:"fsGroup,omitempty" yaml:"fsGroup,omitempty"`
	SupplementalGroups []int64 `json:"supplementalGroups,omitempty" yaml:"supplementalGroups,omitempty"`
	Sysctls      []Sysctl `json:"sysctls,omitempty" yaml:"sysctls,omitempty"`
}

type SecurityContext struct {
	RunAsUser                *int64 `json:"runAsUser,omitempty" yaml:"runAsUser,omitempty"`
	RunAsGroup               *int64 `json:"runAsGroup,omitempty" yaml:"runAsGroup,omitempty"`
	RunAsNonRoot             *bool  `json:"runAsNonRoot,omitempty" yaml:"runAsNonRoot,omitempty"`
	ReadOnlyRootFilesystem   *bool  `json:"readOnlyRootFilesystem,omitempty" yaml:"readOnlyRootFilesystem,omitempty"`
	Privileged               *bool  `json:"privileged,omitempty" yaml:"privileged,omitempty"`
	AllowPrivilegeEscalation *bool  `json:"allowPrivilegeEscalation,omitempty" yaml:"allowPrivilegeEscalation,omitempty"`
	Capabilities             *Capabilities `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	SELinuxOptions           *SELinuxOptions `json:"seLinuxOptions,omitempty" yaml:"seLinuxOptions,omitempty"`
	SeccompProfile           *SeccompProfile `json:"seccompProfile,omitempty" yaml:"seccompProfile,omitempty"`
	ProcMount                *string `json:"procMount,omitempty" yaml:"procMount,omitempty"`
}

type Capabilities struct {
	Add  []string `json:"add,omitempty" yaml:"add,omitempty"`
	Drop []string `json:"drop,omitempty" yaml:"drop,omitempty"`
}

type SELinuxOptions struct {
	User  string `json:"user,omitempty" yaml:"user,omitempty"`
	Role  string `json:"role,omitempty" yaml:"role,omitempty"`
	Type  string `json:"type,omitempty" yaml:"type,omitempty"`
	Level string `json:"level,omitempty" yaml:"level,omitempty"`
}

type SeccompProfile struct {
	Type             string `json:"type" yaml:"type"`
	LocalhostProfile *string `json:"localhostProfile,omitempty" yaml:"localhostProfile,omitempty"`
}

type Sysctl struct {
	Name  string `json:"name" yaml:"name"`
	Value string `json:"value" yaml:"value"`
}

type HostAlias struct {
	IP        string   `json:"ip" yaml:"ip"`
	Hostnames []string `json:"hostnames,omitempty" yaml:"hostnames,omitempty"`
}

type TopologySpreadConstraint struct {
	MaxSkew           int32          `json:"maxSkew" yaml:"maxSkew"`
	TopologyKey       string         `json:"topologyKey" yaml:"topologyKey"`
	WhenUnsatisfiable string         `json:"whenUnsatisfiable" yaml:"whenUnsatisfiable"`
	LabelSelector     *LabelSelector `json:"labelSelector,omitempty" yaml:"labelSelector,omitempty"`
	MinDomains        *int32         `json:"minDomains,omitempty" yaml:"minDomains,omitempty"`
}

type PodStatus struct {
	Phase             string            `json:"phase,omitempty" yaml:"phase,omitempty"`
	Conditions        []PodCondition    `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	Message           string            `json:"message,omitempty" yaml:"message,omitempty"`
	Reason            string            `json:"reason,omitempty" yaml:"reason,omitempty"`
	HostIP            string            `json:"hostIP,omitempty" yaml:"hostIP,omitempty"`
	HostIPs           []HostIP          `json:"hostIPs,omitempty" yaml:"hostIPs,omitempty"`
	PodIP             string            `json:"podIP,omitempty" yaml:"podIP,omitempty"`
	PodIPs            []PodIP           `json:"podIPs,omitempty" yaml:"podIPs,omitempty"`
	StartTime         *time.Time        `json:"startTime,omitempty" yaml:"startTime,omitempty"`
	InitContainerStatuses []ContainerStatus `json:"initContainerStatuses,omitempty" yaml:"initContainerStatuses,omitempty"`
	ContainerStatuses []ContainerStatus `json:"containerStatuses,omitempty" yaml:"containerStatuses,omitempty"`
	QOSClass          string            `json:"qosClass,omitempty" yaml:"qosClass,omitempty"`
	NominatedNodeName string            `json:"nominatedNodeName,omitempty" yaml:"nominatedNodeName,omitempty"`
}

type PodCondition struct {
	Type               string    `json:"type" yaml:"type"`
	Status             string    `json:"status" yaml:"status"`
	LastProbeTime      time.Time `json:"lastProbeTime,omitempty" yaml:"lastProbeTime,omitempty"`
	LastTransitionTime time.Time `json:"lastTransitionTime,omitempty" yaml:"lastTransitionTime,omitempty"`
	Reason             string    `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message            string    `json:"message,omitempty" yaml:"message,omitempty"`
}

type HostIP struct {
	IP string `json:"ip" yaml:"ip"`
}

type PodIP struct {
	IP string `json:"ip" yaml:"ip"`
}

type ContainerStatus struct {
	Name                 string          `json:"name" yaml:"name"`
	State                ContainerState  `json:"state,omitempty" yaml:"state,omitempty"`
	LastTerminationState ContainerState  `json:"lastState,omitempty" yaml:"lastState,omitempty"`
	Ready                bool            `json:"ready" yaml:"ready"`
	RestartCount         int32           `json:"restartCount" yaml:"restartCount"`
	Image                string          `json:"image" yaml:"image"`
	ImageID              string          `json:"imageID" yaml:"imageID"`
	ContainerID          string          `json:"containerID,omitempty" yaml:"containerID,omitempty"`
	Started              *bool           `json:"started,omitempty" yaml:"started,omitempty"`
	AllocatedResources   ResourceList    `json:"allocatedResources,omitempty" yaml:"allocatedResources,omitempty"`
}

type ContainerState struct {
	Waiting    *ContainerStateWaiting    `json:"waiting,omitempty" yaml:"waiting,omitempty"`
	Running    *ContainerStateRunning    `json:"running,omitempty" yaml:"running,omitempty"`
	Terminated *ContainerStateTerminated `json:"terminated,omitempty" yaml:"terminated,omitempty"`
}

type ContainerStateWaiting struct {
	Reason  string `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

type ContainerStateRunning struct {
	StartedAt time.Time `json:"startedAt,omitempty" yaml:"startedAt,omitempty"`
}

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
	PodPending   = "Pending"
	PodRunning   = "Running"
	PodSucceeded = "Succeeded"
	PodFailed    = "Failed"
	PodUnknown   = "Unknown"
)
