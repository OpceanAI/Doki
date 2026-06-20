package k8s

import "time"

// Service represents a Kubernetes Service resource.
type Service struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       ServiceSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     ServiceStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// ServiceList is a list of Service resources.
type ServiceList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []Service `json:"items" yaml:"items"`
}

// ServiceSpec defines the desired state of a Service.
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

// ServicePort defines a network port exposed by a Service.
type ServicePort struct {
	Name        string      `json:"name,omitempty" yaml:"name,omitempty"`
	Protocol    string      `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	AppProtocol *string     `json:"appProtocol,omitempty" yaml:"appProtocol,omitempty"`
	Port        int32       `json:"port" yaml:"port"`
	TargetPort  IntOrString `json:"targetPort,omitempty" yaml:"targetPort,omitempty"`
	NodePort    int32       `json:"nodePort,omitempty" yaml:"nodePort,omitempty"`
}

// ServiceStatus represents the observed state of a Service.
type ServiceStatus struct {
	LoadBalancer LoadBalancerStatus `json:"loadBalancer,omitempty" yaml:"loadBalancer,omitempty"`
	Conditions   []Condition        `json:"conditions,omitempty" yaml:"conditions,omitempty"`
}

// LoadBalancerStatus represents the status of a load balancer assigned to a Service.
type LoadBalancerStatus struct {
	Ingress []LoadBalancerIngress `json:"ingress,omitempty" yaml:"ingress,omitempty"`
}

// LoadBalancerIngress represents an ingress point for a load balancer.
type LoadBalancerIngress struct {
	IP       string                   `json:"ip,omitempty" yaml:"ip,omitempty"`
	Hostname string                   `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	IPMode   *string                  `json:"ipMode,omitempty" yaml:"ipMode,omitempty"`
	Ports    []PortStatus             `json:"ports,omitempty" yaml:"ports,omitempty"`
}

// PortStatus represents the status of a port exposed by a load balancer.
type PortStatus struct {
	Port     int32  `json:"port" yaml:"port"`
	Protocol string `json:"protocol" yaml:"protocol"`
	Error    *string `json:"error,omitempty" yaml:"error,omitempty"`
}

// ServiceTypeClusterIP is the Service type for cluster-internal IP access only.
// ServiceTypeNodePort exposes the Service on each node's IP at a static port.
// ServiceTypeLoadBalancer exposes the Service externally using a cloud provider's load balancer.
// ServiceTypeExternalName maps the Service to an external DNS name.
const (
	ServiceTypeClusterIP    = "ClusterIP"
	ServiceTypeNodePort     = "NodePort"
	ServiceTypeLoadBalancer = "LoadBalancer"
	ServiceTypeExternalName = "ExternalName"
)

// ConfigMap represents a Kubernetes ConfigMap resource containing configuration data.
type ConfigMap struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Data       map[string]string `json:"data,omitempty" yaml:"data,omitempty"`
	BinaryData map[string][]byte `json:"binaryData,omitempty" yaml:"binaryData,omitempty"`
	Immutable  *bool             `json:"immutable,omitempty" yaml:"immutable,omitempty"`
}

// ConfigMapList is a list of ConfigMap resources.
type ConfigMapList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []ConfigMap `json:"items" yaml:"items"`
}

// Secret represents a Kubernetes Secret resource containing sensitive data.
type Secret struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Data       map[string][]byte `json:"data,omitempty" yaml:"data,omitempty"`
	StringData map[string]string `json:"stringData,omitempty" yaml:"stringData,omitempty"`
	Type       string            `json:"type,omitempty" yaml:"type,omitempty"`
	Immutable  *bool             `json:"immutable,omitempty" yaml:"immutable,omitempty"`
}

// SecretList is a list of Secret resources.
type SecretList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []Secret `json:"items" yaml:"items"`
}

// SecretTypeOpaque is the default Secret type for arbitrary user-defined data.
// SecretTypeServiceAccountToken stores a ServiceAccount token.
// SecretTypeDockercfg stores a legacy Docker registry credential.
// SecretTypeDockerConfigJSON stores a Docker registry credential in JSON format.
// SecretTypeBasicAuth stores credentials for basic authentication.
// SecretTypeSSHAuth stores credentials for SSH authentication.
// SecretTypeTLS stores TLS certificates and keys.
// SecretTypeBootstrapToken stores bootstrap tokens for node registration.
const (
	SecretTypeOpaque              = "Opaque"
	SecretTypeServiceAccountToken = "kubernetes.io/service-account-token"
	SecretTypeDockercfg           = "kubernetes.io/dockercfg"
	SecretTypeDockerConfigJSON    = "kubernetes.io/dockerconfigjson"
	SecretTypeBasicAuth           = "kubernetes.io/basic-auth"
	SecretTypeSSHAuth             = "kubernetes.io/ssh-auth"
	SecretTypeTLS                 = "kubernetes.io/tls"
	SecretTypeBootstrapToken      = "bootstrap.kubernetes.io/token"
)

// Namespace represents a Kubernetes Namespace resource.
type Namespace struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       NamespaceSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     NamespaceStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// NamespaceList is a list of Namespace resources.
type NamespaceList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []Namespace `json:"items" yaml:"items"`
}

// NamespaceSpec defines the desired state of a Namespace.
type NamespaceSpec struct {
	Finalizers []string `json:"finalizers,omitempty" yaml:"finalizers,omitempty"`
}

// NamespaceStatus represents the observed state of a Namespace.
type NamespaceStatus struct {
	Phase      string      `json:"phase,omitempty" yaml:"phase,omitempty"`
	Conditions []Condition `json:"conditions,omitempty" yaml:"conditions,omitempty"`
}

// NamespaceActive indicates the Namespace is active.
// NamespaceTerminating indicates the Namespace is being terminated.
const (
	NamespaceActive      = "Active"
	NamespaceTerminating = "Terminating"
)

// Node represents a Kubernetes Node resource.
type Node struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       NodeSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     NodeStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// NodeList is a list of Node resources.
type NodeList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []Node `json:"items" yaml:"items"`
}

// NodeSpec defines the desired state of a Node.
type NodeSpec struct {
	PodCIDR       string    `json:"podCIDR,omitempty" yaml:"podCIDR,omitempty"`
	PodCIDRs      []string  `json:"podCIDRs,omitempty" yaml:"podCIDRs,omitempty"`
	ProviderID    string    `json:"providerID,omitempty" yaml:"providerID,omitempty"`
	Unschedulable bool      `json:"unschedulable,omitempty" yaml:"unschedulable,omitempty"`
	Taints        []Taint   `json:"taints,omitempty" yaml:"taints,omitempty"`
}

// Taint represents a node taint that repels pods from being scheduled onto a Node.
type Taint struct {
	Key       string    `json:"key" yaml:"key"`
	Value     string    `json:"value,omitempty" yaml:"value,omitempty"`
	Effect    string    `json:"effect" yaml:"effect"`
	TimeAdded *time.Time `json:"timeAdded,omitempty" yaml:"timeAdded,omitempty"`
}

// NodeStatus represents the observed state of a Node.
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

// NodeCondition represents a condition observed on a Node.
type NodeCondition struct {
	Type               string    `json:"type" yaml:"type"`
	Status             string    `json:"status" yaml:"status"`
	LastHeartbeatTime  time.Time `json:"lastHeartbeatTime,omitempty" yaml:"lastHeartbeatTime,omitempty"`
	LastTransitionTime time.Time `json:"lastTransitionTime,omitempty" yaml:"lastTransitionTime,omitempty"`
	Reason             string    `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message            string    `json:"message,omitempty" yaml:"message,omitempty"`
}

// NodeAddress represents a network address of a Node.
type NodeAddress struct {
	Type    string `json:"type" yaml:"type"`
	Address string `json:"address" yaml:"address"`
}

// NodeSystemInfo represents system information about a Node.
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

// ContainerImage represents a container image available on a Node.
type ContainerImage struct {
	Names     []string `json:"names,omitempty" yaml:"names,omitempty"`
	SizeBytes int64    `json:"sizeBytes,omitempty" yaml:"sizeBytes,omitempty"`
}

// NodeDaemonEndpoints represents the daemon endpoints exposed by a Node.
type NodeDaemonEndpoints struct {
	KubeletEndpoint DaemonEndpoint `json:"kubeletEndpoint,omitempty" yaml:"kubeletEndpoint,omitempty"`
}

// DaemonEndpoint represents a TCP endpoint of a daemon running on a Node.
type DaemonEndpoint struct {
	Port int32 `json:"port" yaml:"port"`
}

// ServiceAccount represents a Kubernetes ServiceAccount resource.
type ServiceAccount struct {
	TypeMeta                   `json:",inline" yaml:",inline"`
	ObjectMeta                 `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Secrets                    []ObjectReference `json:"secrets,omitempty" yaml:"secrets,omitempty"`
	ImagePullSecrets           []LocalObjectReference `json:"imagePullSecrets,omitempty" yaml:"imagePullSecrets,omitempty"`
	AutomountServiceAccountToken *bool `json:"automountServiceAccountToken,omitempty" yaml:"automountServiceAccountToken,omitempty"`
}

// ServiceAccountList is a list of ServiceAccount resources.
type ServiceAccountList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []ServiceAccount `json:"items" yaml:"items"`
}

// Endpoints represents a Kubernetes Endpoints resource.
type Endpoints struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Subsets    []EndpointSubset `json:"subsets,omitempty" yaml:"subsets,omitempty"`
}

// EndpointSubset represents a group of addresses with a common set of ports.
type EndpointSubset struct {
	Addresses         []EndpointAddress `json:"addresses,omitempty" yaml:"addresses,omitempty"`
	NotReadyAddresses []EndpointAddress `json:"notReadyAddresses,omitempty" yaml:"notReadyAddresses,omitempty"`
	Ports             []EndpointPort    `json:"ports,omitempty" yaml:"ports,omitempty"`
}

// EndpointAddress represents a single address in an EndpointSubset.
type EndpointAddress struct {
	IP       string          `json:"ip" yaml:"ip"`
	Hostname string          `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	NodeName *string         `json:"nodeName,omitempty" yaml:"nodeName,omitempty"`
	TargetRef *ObjectReference `json:"targetRef,omitempty" yaml:"targetRef,omitempty"`
}

// EndpointPort represents a port in an EndpointSubset.
type EndpointPort struct {
	Name     string `json:"name,omitempty" yaml:"name,omitempty"`
	Port     int32  `json:"port" yaml:"port"`
	Protocol string `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	AppProtocol *string `json:"appProtocol,omitempty" yaml:"appProtocol,omitempty"`
}

// Event represents a Kubernetes Event resource.
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

// EventSource represents the source of a Kubernetes Event.
type EventSource struct {
	Component string `json:"component,omitempty" yaml:"component,omitempty"`
	Host      string `json:"host,omitempty" yaml:"host,omitempty"`
}

// PersistentVolumeClaim represents a Kubernetes PersistentVolumeClaim resource.
type PersistentVolumeClaim struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       PersistentVolumeClaimSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     PersistentVolumeClaimStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// PersistentVolumeClaimList is a list of PersistentVolumeClaim resources.
type PersistentVolumeClaimList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []PersistentVolumeClaim `json:"items" yaml:"items"`
}

// PersistentVolumeClaimSpec defines the desired state of a PersistentVolumeClaim.
type PersistentVolumeClaimSpec struct {
	AccessModes      []string             `json:"accessModes,omitempty" yaml:"accessModes,omitempty"`
	Selector         *LabelSelector       `json:"selector,omitempty" yaml:"selector,omitempty"`
	Resources        ResourceRequirements `json:"resources,omitempty" yaml:"resources,omitempty"`
	VolumeName       string               `json:"volumeName,omitempty" yaml:"volumeName,omitempty"`
	StorageClassName *string              `json:"storageClassName,omitempty" yaml:"storageClassName,omitempty"`
	VolumeMode       *string              `json:"volumeMode,omitempty" yaml:"volumeMode,omitempty"`
}

// PersistentVolumeClaimStatus represents the observed state of a PersistentVolumeClaim.
type PersistentVolumeClaimStatus struct {
	Phase       string       `json:"phase,omitempty" yaml:"phase,omitempty"`
	AccessModes []string     `json:"accessModes,omitempty" yaml:"accessModes,omitempty"`
	Capacity    ResourceList `json:"capacity,omitempty" yaml:"capacity,omitempty"`
	Conditions  []Condition  `json:"conditions,omitempty" yaml:"conditions,omitempty"`
}

// PersistentVolume represents a Kubernetes PersistentVolume resource.
type PersistentVolume struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       PersistentVolumeSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     PersistentVolumeStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// PersistentVolumeList is a list of PersistentVolume resources.
type PersistentVolumeList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []PersistentVolume `json:"items" yaml:"items"`
}

// PersistentVolumeSpec defines the desired state of a PersistentVolume.
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

// PersistentVolumeStatus represents the observed state of a PersistentVolume.
type PersistentVolumeStatus struct {
	Phase   string `json:"phase,omitempty" yaml:"phase,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
	Reason  string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// NFSVolumeSource represents an NFS mount that backs a volume.
type NFSVolumeSource struct {
	Server   string `json:"server" yaml:"server"`
	Path     string `json:"path" yaml:"path"`
	ReadOnly bool   `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
}
