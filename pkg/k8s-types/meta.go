package k8s

import (
	"encoding/json"
	"time"
)

// TypeMeta represents the identity metadata carried by every Kubernetes-style
// object: the resource Kind and its API Version.
type TypeMeta struct {
	Kind       string `json:"kind" yaml:"kind"`
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
}

// ObjectMeta represents the standard metadata attached to every Kubernetes-style
// object: identity, namespace, labels, owner references and lifecycle timestamps.
type ObjectMeta struct {
	Name              string            `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace         string            `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	UID               string            `json:"uid,omitempty" yaml:"uid,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty" yaml:"resourceVersion,omitempty"`
	Generation        int64             `json:"generation,omitempty" yaml:"generation,omitempty"`
	CreationTimestamp time.Time         `json:"creationTimestamp,omitempty" yaml:"creationTimestamp,omitempty"`
	DeletionTimestamp *time.Time        `json:"deletionTimestamp,omitempty" yaml:"deletionTimestamp,omitempty"`
	Labels            map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	OwnerReferences   []OwnerReference  `json:"ownerReferences,omitempty" yaml:"ownerReferences,omitempty"`
	Finalizers        []string          `json:"finalizers,omitempty" yaml:"finalizers,omitempty"`
}

// ListMeta represents the pagination metadata returned with collection
// responses (resource version and continuation token).
type ListMeta struct {
	ResourceVersion string `json:"resourceVersion,omitempty" yaml:"resourceVersion,omitempty"`
	Continue        string `json:"continue,omitempty" yaml:"continue,omitempty"`
}

// OwnerReference represents a pointer to a parent Kubernetes-style object
// that owns the current resource.
type OwnerReference struct {
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
	Kind       string `json:"kind" yaml:"kind"`
	Name       string `json:"name" yaml:"name"`
	UID        string `json:"uid" yaml:"uid"`
	Controller *bool  `json:"controller,omitempty" yaml:"controller,omitempty"`
}

// LabelSelector represents a label-based selection criterion used to
// match a set of Kubernetes-style resources.
type LabelSelector struct {
	MatchLabels      map[string]string          `json:"matchLabels,omitempty" yaml:"matchLabels,omitempty"`
	MatchExpressions []LabelSelectorRequirement `json:"matchExpressions,omitempty" yaml:"matchExpressions,omitempty"`
}

// LabelSelectorRequirement represents a single label-key/operator/values
// expression that composes a LabelSelector.
type LabelSelectorRequirement struct {
	Key      string   `json:"key" yaml:"key"`
	Operator string   `json:"operator" yaml:"operator"`
	Values   []string `json:"values,omitempty" yaml:"values,omitempty"`
}

// Condition represents a generic status condition attached to a
// Kubernetes-style resource, capturing the last transition and reason.
type Condition struct {
	Type               string    `json:"type" yaml:"type"`
	Status             string    `json:"status" yaml:"status"`
	LastTransitionTime time.Time `json:"lastTransitionTime,omitempty" yaml:"lastTransitionTime,omitempty"`
	Reason             string    `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message            string    `json:"message,omitempty" yaml:"message,omitempty"`
}

// LocalObjectReference represents a reference to another object in the same
// namespace, identified by its name only.
type LocalObjectReference struct {
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
}

// ObjectReference represents a generic reference to any Kubernetes-style
// object, identified by kind, namespace, name and UID.
type ObjectReference struct {
	Kind       string `json:"kind,omitempty" yaml:"kind,omitempty"`
	Namespace  string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Name       string `json:"name,omitempty" yaml:"name,omitempty"`
	UID        string `json:"uid,omitempty" yaml:"uid,omitempty"`
	APIVersion string `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
}

// WatchEvent represents a single change event delivered on a Kubernetes-style
// watch stream, carrying the event type and the affected object payload.
type WatchEvent struct {
	Type   string          `json:"type"`
	Object json.RawMessage `json:"object"`
}
