package k8s

import "time"

type Service struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       ServiceSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     ServiceStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type ServiceList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []Service `json:"items" yaml:"items"`
}

type ServiceSpec struct {
	Type                  string           `json:"type,omitempty" yaml:"type,omitempty"`
	Ports                 []ServicePort    `json:"ports,omitempty" yaml:"ports,omitempty"`
	Selector              map[string]string `json:"selector,omitempty" yaml:"selector,omitempty"`
	ClusterIP             string           `json:"clusterIP,omitempty" yaml:"clusterIP,omitempty"`
	ClusterIPs            []string         `json:"clusterIPs,omitempty" yaml:"clusterIPs,omitempty"`
	ExternalIPs           []string         `json:"externalIPs,omitempty" yaml:"externalIPs,omitempty"`
	LoadBalancerIP        string           `json:"loadBalancerIP,omitempty" yaml:"loadBalancerIP,omitempty"`
	LoadBalancerSourceRanges []string      `json:"loadBalancerSourceRanges,omitempty" yaml:"loadBalancerSourceRanges,omitempty"`
	ExternalName          string           `json:"externalName,omitempty" yaml:"externalName,omitempty"`
	ExternalTrafficPolicy string           `json:"externalTrafficPolicy,omitempty" yaml:"externalTrafficPolicy,omitempty"`
	InternalTrafficPolicy string           `json:"internalTrafficPolicy,omitempty" yaml:"internalTrafficPolicy,omitempty"`
	HealthCheckNodePort   int32            `json:"healthCheckNodePort,omitempty" yaml:"healthCheckNodePort,omitempty"`
	SessionAffinity       string           `json:"sessionAffinity,omitempty" yaml:"sessionAffinity,omitempty"`
	IPFamilies            []string         `json:"ipFamilies,omitempty" yaml:"ipFamilies,omitempty"`
	IPFamilyPolicy        string           `json:"ipFamilyPolicy,omitempty" yaml:"ipFamilyPolicy,omitempty"`
	AllocateLoadBalancerNodePorts *bool    `json:"allocateLoadBalancerNodePorts,omitempty" yaml:"allocateLoadBalancerNodePorts,omitempty"`
	PublishNotReadyAddresses bool          `json:"publishNotReadyAddresses,omitempty" yaml:"publishNotReadyAddresses,omitempty"`
}

type ServicePort struct {
	Name        string      `json:"name,omitempty" yaml:"name,omitempty"`
	Protocol    string      `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	AppProtocol *string     `json:"appProtocol,omitempty" yaml:"appProtocol,omitempty"`
	Port        int32       `json:"port" yaml:"port"`
	TargetPort  IntOrString `json:"targetPort,omitempty" yaml:"targetPort,omitempty"`
	NodePort    int32       `json:"nodePort,omitempty" yaml:"nodePort,omitempty"`
}

type ServiceStatus struct {
	LoadBalancer LoadBalancerStatus `json:"loadBalancer,omitempty" yaml:"loadBalancer,omitempty"`
	Conditions   []Condition        `json:"conditions,omitempty" yaml:"conditions,omitempty"`
}

type LoadBalancerStatus struct {
	Ingress []LoadBalancerIngress `json:"ingress,omitempty" yaml:"ingress,omitempty"`
}

type LoadBalancerIngress struct {
	IP       string                   `json:"ip,omitempty" yaml:"ip,omitempty"`
	Hostname string                   `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	IPMode   *string                  `json:"ipMode,omitempty" yaml:"ipMode,omitempty"`
	Ports    []PortStatus             `json:"ports,omitempty" yaml:"ports,omitempty"`
}

type PortStatus struct {
	Port     int32  `json:"port" yaml:"port"`
	Protocol string `json:"protocol" yaml:"protocol"`
	Error    *string `json:"error,omitempty" yaml:"error,omitempty"`
}

const (
	ServiceTypeClusterIP    = "ClusterIP"
	ServiceTypeNodePort     = "NodePort"
	ServiceTypeLoadBalancer = "LoadBalancer"
	ServiceTypeExternalName = "ExternalName"
)

type ConfigMap struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Data       map[string]string `json:"data,omitempty" yaml:"data,omitempty"`
	BinaryData map[string][]byte `json:"binaryData,omitempty" yaml:"binaryData,omitempty"`
	Immutable  *bool             `json:"immutable,omitempty" yaml:"immutable,omitempty"`
}

type ConfigMapList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []ConfigMap `json:"items" yaml:"items"`
}

type Secret struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Data       map[string][]byte `json:"data,omitempty" yaml:"data,omitempty"`
	StringData map[string]string `json:"stringData,omitempty" yaml:"stringData,omitempty"`
	Type       string            `json:"type,omitempty" yaml:"type,omitempty"`
	Immutable  *bool             `json:"immutable,omitempty" yaml:"immutable,omitempty"`
}

type SecretList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []Secret `json:"items" yaml:"items"`
}

const (
	SecretTypeOpaque                = "Opaque"
	SecretTypeServiceAccountToken   = "kubernetes.io/service-account-token"
	SecretTypeDockercfg             = "kubernetes.io/dockercfg"
	SecretTypeDockerConfigJSON      = "kubernetes.io/dockerconfigjson"
	SecretTypeBasicAuth             = "kubernetes.io/basic-auth"
	SecretTypeSSHAuth               = "kubernetes.io/ssh-auth"
	SecretTypeTLS                   = "kubernetes.io/tls"
	SecretTypeBootstrapToken        = "bootstrap.kubernetes.io/token"
)

type Namespace struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       NamespaceSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     NamespaceStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type NamespaceList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []Namespace `json:"items" yaml:"items"`
}

type NamespaceSpec struct {
	Finalizers []string `json:"finalizers,omitempty" yaml:"finalizers,omitempty"`
}

type NamespaceStatus struct {
	Phase      string      `json:"phase,omitempty" yaml:"phase,omitempty"`
	Conditions []Condition `json:"conditions,omitempty" yaml:"conditions,omitempty"`
}

const (
	NamespaceActive      = "Active"
	NamespaceTerminating = "Terminating"
)

type Node struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       NodeSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     NodeStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type NodeList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []Node `json:"items" yaml:"items"`
}

type NodeSpec struct {
	PodCIDR       string    `json:"podCIDR,omitempty" yaml:"podCIDR,omitempty"`
	PodCIDRs      []string  `json:"podCIDRs,omitempty" yaml:"podCIDRs,omitempty"`
	ProviderID    string    `json:"providerID,omitempty" yaml:"providerID,omitempty"`
	Unschedulable bool      `json:"unschedulable,omitempty" yaml:"unschedulable,omitempty"`
	Taints        []Taint   `json:"taints,omitempty" yaml:"taints,omitempty"`
}

type Taint struct {
	Key       string    `json:"key" yaml:"key"`
	Value     string    `json:"value,omitempty" yaml:"value,omitempty"`
	Effect    string    `json:"effect" yaml:"effect"`
	TimeAdded *time.Time `json:"timeAdded,omitempty" yaml:"timeAdded,omitempty"`
}

type NodeStatus struct {
	Capacity    ResourceList         `json:"capacity,omitempty" yaml:"capacity,omitempty"`
	Allocatable ResourceList         `json:"allocatable,omitempty" yaml:"allocatable,omitempty"`
	Phase       string               `json:"phase,omitempty" yaml:"phase,omitempty"`
	Conditions  []NodeCondition      `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	Addresses   []NodeAddress        `json:"addresses,omitempty" yaml:"addresses,omitempty"`
	NodeInfo    NodeSystemInfo       `json:"nodeInfo,omitempty" yaml:"nodeInfo,omitempty"`
	Images      []ContainerImage     `json:"images,omitempty" yaml:"images,omitempty"`
	DaemonEndpoints NodeDaemonEndpoints `json:"daemonEndpoints,omitempty" yaml:"daemonEndpoints,omitempty"`
}

type NodeCondition struct {
	Type               string    `json:"type" yaml:"type"`
	Status             string    `json:"status" yaml:"status"`
	LastHeartbeatTime  time.Time `json:"lastHeartbeatTime,omitempty" yaml:"lastHeartbeatTime,omitempty"`
	LastTransitionTime time.Time `json:"lastTransitionTime,omitempty" yaml:"lastTransitionTime,omitempty"`
	Reason             string    `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message            string    `json:"message,omitempty" yaml:"message,omitempty"`
}

type NodeAddress struct {
	Type    string `json:"type" yaml:"type"`
	Address string `json:"address" yaml:"address"`
}

type NodeSystemInfo struct {
	MachineID               string `json:"machineID" yaml:"machineID"`
	SystemUUID              string `json:"systemUUID" yaml:"systemUUID"`
	BootID                  string `json:"bootID" yaml:"bootID"`
	KernelVersion           string `json:"kernelVersion" yaml:"kernelVersion"`
	OSImage                 string `json:"osImage" yaml:"osImage"`
	ContainerRuntimeVersion string `json:"containerRuntimeVersion" yaml:"containerRuntimeVersion"`
	KubeletVersion          string `json:"kubeletVersion" yaml:"kubeletVersion"`
	KubeProxyVersion        string `json:"kubeProxyVersion" yaml:"kubeProxyVersion"`
	OperatingSystem         string `json:"operatingSystem" yaml:"operatingSystem"`
	Architecture            string `json:"architecture" yaml:"architecture"`
}

type ContainerImage struct {
	Names     []string `json:"names,omitempty" yaml:"names,omitempty"`
	SizeBytes int64    `json:"sizeBytes,omitempty" yaml:"sizeBytes,omitempty"`
}

type NodeDaemonEndpoints struct {
	KubeletEndpoint DaemonEndpoint `json:"kubeletEndpoint,omitempty" yaml:"kubeletEndpoint,omitempty"`
}

type DaemonEndpoint struct {
	Port int32 `json:"port" yaml:"port"`
}

type ServiceAccount struct {
	TypeMeta                   `json:",inline" yaml:",inline"`
	ObjectMeta                 `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Secrets                    []ObjectReference `json:"secrets,omitempty" yaml:"secrets,omitempty"`
	ImagePullSecrets           []LocalObjectReference `json:"imagePullSecrets,omitempty" yaml:"imagePullSecrets,omitempty"`
	AutomountServiceAccountToken *bool `json:"automountServiceAccountToken,omitempty" yaml:"automountServiceAccountToken,omitempty"`
}

type ServiceAccountList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []ServiceAccount `json:"items" yaml:"items"`
}

type Endpoints struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Subsets    []EndpointSubset `json:"subsets,omitempty" yaml:"subsets,omitempty"`
}

type EndpointSubset struct {
	Addresses         []EndpointAddress `json:"addresses,omitempty" yaml:"addresses,omitempty"`
	NotReadyAddresses []EndpointAddress `json:"notReadyAddresses,omitempty" yaml:"notReadyAddresses,omitempty"`
	Ports             []EndpointPort    `json:"ports,omitempty" yaml:"ports,omitempty"`
}

type EndpointAddress struct {
	IP       string          `json:"ip" yaml:"ip"`
	Hostname string          `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	NodeName *string         `json:"nodeName,omitempty" yaml:"nodeName,omitempty"`
	TargetRef *ObjectReference `json:"targetRef,omitempty" yaml:"targetRef,omitempty"`
}

type EndpointPort struct {
	Name     string `json:"name,omitempty" yaml:"name,omitempty"`
	Port     int32  `json:"port" yaml:"port"`
	Protocol string `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	AppProtocol *string `json:"appProtocol,omitempty" yaml:"appProtocol,omitempty"`
}

type Event struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	InvolvedObject ObjectReference `json:"involvedObject" yaml:"involvedObject"`
	Reason         string          `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message        string          `json:"message,omitempty" yaml:"message,omitempty"`
	Source         EventSource     `json:"source,omitempty" yaml:"source,omitempty"`
	FirstTimestamp time.Time       `json:"firstTimestamp,omitempty" yaml:"firstTimestamp,omitempty"`
	LastTimestamp  time.Time       `json:"lastTimestamp,omitempty" yaml:"lastTimestamp,omitempty"`
	Count          int32           `json:"count,omitempty" yaml:"count,omitempty"`
	Type           string          `json:"type,omitempty" yaml:"type,omitempty"`
	Action         string          `json:"action,omitempty" yaml:"action,omitempty"`
	ReportingController string     `json:"reportingController,omitempty" yaml:"reportingController,omitempty"`
	ReportingInstance   string     `json:"reportingInstance,omitempty" yaml:"reportingInstance,omitempty"`
}

type EventSource struct {
	Component string `json:"component,omitempty" yaml:"component,omitempty"`
	Host      string `json:"host,omitempty" yaml:"host,omitempty"`
}

type PersistentVolumeClaim struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       PersistentVolumeClaimSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     PersistentVolumeClaimStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type PersistentVolumeClaimList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []PersistentVolumeClaim `json:"items" yaml:"items"`
}

type PersistentVolumeClaimSpec struct {
	AccessModes      []string             `json:"accessModes,omitempty" yaml:"accessModes,omitempty"`
	Selector         *LabelSelector       `json:"selector,omitempty" yaml:"selector,omitempty"`
	Resources        ResourceRequirements `json:"resources,omitempty" yaml:"resources,omitempty"`
	VolumeName       string               `json:"volumeName,omitempty" yaml:"volumeName,omitempty"`
	StorageClassName *string              `json:"storageClassName,omitempty" yaml:"storageClassName,omitempty"`
	VolumeMode       *string              `json:"volumeMode,omitempty" yaml:"volumeMode,omitempty"`
}

type PersistentVolumeClaimStatus struct {
	Phase       string       `json:"phase,omitempty" yaml:"phase,omitempty"`
	AccessModes []string     `json:"accessModes,omitempty" yaml:"accessModes,omitempty"`
	Capacity    ResourceList `json:"capacity,omitempty" yaml:"capacity,omitempty"`
	Conditions  []Condition  `json:"conditions,omitempty" yaml:"conditions,omitempty"`
}

type PersistentVolume struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       PersistentVolumeSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     PersistentVolumeStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type PersistentVolumeList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []PersistentVolume `json:"items" yaml:"items"`
}

type PersistentVolumeSpec struct {
	Capacity                      ResourceList                  `json:"capacity,omitempty" yaml:"capacity,omitempty"`
	AccessModes                   []string                      `json:"accessModes,omitempty" yaml:"accessModes,omitempty"`
	PersistentVolumeReclaimPolicy string                        `json:"persistentVolumeReclaimPolicy,omitempty" yaml:"persistentVolumeReclaimPolicy,omitempty"`
	StorageClassName              string                        `json:"storageClassName,omitempty" yaml:"storageClassName,omitempty"`
	VolumeMode                    *string                       `json:"volumeMode,omitempty" yaml:"volumeMode,omitempty"`
	ClaimRef                      *ObjectReference              `json:"claimRef,omitempty" yaml:"claimRef,omitempty"`
	HostPath                      *HostPathVolumeSource         `json:"hostPath,omitempty" yaml:"hostPath,omitempty"`
	NFS                           *NFSVolumeSource              `json:"nfs,omitempty" yaml:"nfs,omitempty"`
}

type PersistentVolumeStatus struct {
	Phase   string `json:"phase,omitempty" yaml:"phase,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
	Reason  string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type NFSVolumeSource struct {
	Server   string `json:"server" yaml:"server"`
	Path     string `json:"path" yaml:"path"`
	ReadOnly bool   `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
}
