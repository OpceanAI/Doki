// Package k8s contains a partial set of Kubernetes-style API types used by
// Doki to talk to the embedded kubelet and to render pod specs.
package k8s

import "time"

// Deployment represents a Kubernetes-style Deployment resource that
// manages a replicated set of pods via a pod template.
type Deployment struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       DeploymentSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     DeploymentStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// DeploymentList represents a collection of Deployment resources returned
// in a list response.
type DeploymentList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []Deployment `json:"items" yaml:"items"`
}

// DeploymentSpec represents the desired state of a Deployment: replica
// count, label selector, pod template and rollout strategy.
type DeploymentSpec struct {
	Replicas                *int32             `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	Selector                *LabelSelector     `json:"selector" yaml:"selector"`
	Template                PodTemplateSpec    `json:"template" yaml:"template"`
	Strategy                DeploymentStrategy `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	MinReadySeconds         int32              `json:"minReadySeconds,omitempty" yaml:"minReadySeconds,omitempty"`
	RevisionHistoryLimit    *int32             `json:"revisionHistoryLimit,omitempty" yaml:"revisionHistoryLimit,omitempty"`
	Paused                  bool               `json:"paused,omitempty" yaml:"paused,omitempty"`
	ProgressDeadlineSeconds *int32             `json:"progressDeadlineSeconds,omitempty" yaml:"progressDeadlineSeconds,omitempty"`
}

// PodTemplateSpec represents the template used by controllers to create
// new pods, including the pod metadata and pod spec.
type PodTemplateSpec struct {
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       PodSpec `json:"spec,omitempty" yaml:"spec,omitempty"`
}

// DeploymentStrategy represents the strategy used by a Deployment to
// replace existing pods with new ones (recreate or rolling update).
type DeploymentStrategy struct {
	Type          string                   `json:"type,omitempty" yaml:"type,omitempty"`
	RollingUpdate *RollingUpdateDeployment `json:"rollingUpdate,omitempty" yaml:"rollingUpdate,omitempty"`
}

// RollingUpdateDeployment represents the parameters that control the
// rolling-update strategy of a Deployment (max surge / max unavailable).
type RollingUpdateDeployment struct {
	MaxUnavailable *IntOrString `json:"maxUnavailable,omitempty" yaml:"maxUnavailable,omitempty"`
	MaxSurge       *IntOrString `json:"maxSurge,omitempty" yaml:"maxSurge,omitempty"`
}

// DeploymentStatus represents the observed state of a Deployment,
// including the counts of replicas in each lifecycle phase.
type DeploymentStatus struct {
	ObservedGeneration  int64                 `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
	Replicas            int32                 `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	UpdatedReplicas     int32                 `json:"updatedReplicas,omitempty" yaml:"updatedReplicas,omitempty"`
	ReadyReplicas       int32                 `json:"readyReplicas,omitempty" yaml:"readyReplicas,omitempty"`
	AvailableReplicas   int32                 `json:"availableReplicas,omitempty" yaml:"availableReplicas,omitempty"`
	UnavailableReplicas int32                 `json:"unavailableReplicas,omitempty" yaml:"unavailableReplicas,omitempty"`
	Conditions          []DeploymentCondition `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	CollisionCount      *int32                `json:"collisionCount,omitempty" yaml:"collisionCount,omitempty"`
}

// DeploymentCondition represents a single condition entry in a
// Deployment's status subresource.
type DeploymentCondition struct {
	Type               string    `json:"type" yaml:"type"`
	Status             string    `json:"status" yaml:"status"`
	LastUpdateTime     time.Time `json:"lastUpdateTime,omitempty" yaml:"lastUpdateTime,omitempty"`
	LastTransitionTime time.Time `json:"lastTransitionTime,omitempty" yaml:"lastTransitionTime,omitempty"`
	Reason             string    `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message            string    `json:"message,omitempty" yaml:"message,omitempty"`
}

const (
	// DeploymentRecreate is the Deployment strategy type that kills all
	// existing pods before creating new ones.
	DeploymentRecreate = "Recreate"
	// DeploymentRollingUpdate is the Deployment strategy type that
	// replaces pods incrementally.
	DeploymentRollingUpdate = "RollingUpdate"
)

// ReplicaSet represents a Kubernetes-style ReplicaSet resource that
// maintains a stable set of replica pods at any given time.
type ReplicaSet struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       ReplicaSetSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     ReplicaSetStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// ReplicaSetList represents a collection of ReplicaSet resources returned
// in a list response.
type ReplicaSetList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []ReplicaSet `json:"items" yaml:"items"`
}

// ReplicaSetSpec represents the desired state of a ReplicaSet: the
// number of replicas, the label selector and the pod template.
type ReplicaSetSpec struct {
	Replicas        *int32          `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	MinReadySeconds int32           `json:"minReadySeconds,omitempty" yaml:"minReadySeconds,omitempty"`
	Selector        *LabelSelector  `json:"selector" yaml:"selector"`
	Template        PodTemplateSpec `json:"template,omitempty" yaml:"template,omitempty"`
}

// ReplicaSetStatus represents the observed state of a ReplicaSet,
// including the number of ready, fully labeled and available replicas.
type ReplicaSetStatus struct {
	Replicas             int32                 `json:"replicas" yaml:"replicas"`
	FullyLabeledReplicas int32                 `json:"fullyLabeledReplicas,omitempty" yaml:"fullyLabeledReplicas,omitempty"`
	ReadyReplicas        int32                 `json:"readyReplicas,omitempty" yaml:"readyReplicas,omitempty"`
	AvailableReplicas    int32                 `json:"availableReplicas,omitempty" yaml:"availableReplicas,omitempty"`
	ObservedGeneration   int64                 `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
	Conditions           []ReplicaSetCondition `json:"conditions,omitempty" yaml:"conditions,omitempty"`
}

// ReplicaSetCondition represents a single condition entry in a
// ReplicaSet's status subresource.
type ReplicaSetCondition struct {
	Type               string    `json:"type" yaml:"type"`
	Status             string    `json:"status" yaml:"status"`
	LastTransitionTime time.Time `json:"lastTransitionTime,omitempty" yaml:"lastTransitionTime,omitempty"`
	Reason             string    `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message            string    `json:"message,omitempty" yaml:"message,omitempty"`
}

// StatefulSet represents a Kubernetes-style StatefulSet resource that
// manages a set of pods with stable, persistent identity and storage.
type StatefulSet struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       StatefulSetSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     StatefulSetStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// StatefulSetList represents a collection of StatefulSet resources
// returned in a list response.
type StatefulSetList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []StatefulSet `json:"items" yaml:"items"`
}

// StatefulSetSpec represents the desired state of a StatefulSet,
// including its service name, pod management policy and update strategy.
type StatefulSetSpec struct {
	Replicas             *int32                    `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	Selector             *LabelSelector            `json:"selector" yaml:"selector"`
	Template             PodTemplateSpec           `json:"template" yaml:"template"`
	VolumeClaimTemplates []PersistentVolumeClaim   `json:"volumeClaimTemplates,omitempty" yaml:"volumeClaimTemplates,omitempty"`
	ServiceName          string                    `json:"serviceName" yaml:"serviceName"`
	PodManagementPolicy  string                    `json:"podManagementPolicy,omitempty" yaml:"podManagementPolicy,omitempty"`
	UpdateStrategy       StatefulSetUpdateStrategy `json:"updateStrategy,omitempty" yaml:"updateStrategy,omitempty"`
	RevisionHistoryLimit *int32                    `json:"revisionHistoryLimit,omitempty" yaml:"revisionHistoryLimit,omitempty"`
	MinReadySeconds      int32                     `json:"minReadySeconds,omitempty" yaml:"minReadySeconds,omitempty"`
	Ordinals             StatefulSetOrdinals       `json:"ordinals,omitempty" yaml:"ordinals,omitempty"`
}

// StatefulSetUpdateStrategy represents the strategy a StatefulSet uses
// when updating existing pods (rolling update or on delete).
type StatefulSetUpdateStrategy struct {
	Type          string                            `json:"type,omitempty" yaml:"type,omitempty"`
	RollingUpdate *RollingUpdateStatefulSetStrategy `json:"rollingUpdate,omitempty" yaml:"rollingUpdate,omitempty"`
}

// RollingUpdateStatefulSetStrategy represents the parameters controlling
// the rolling update of pods in a StatefulSet.
type RollingUpdateStatefulSetStrategy struct {
	MaxUnavailable *IntOrString `json:"maxUnavailable,omitempty" yaml:"maxUnavailable,omitempty"`
	Partition      *int32       `json:"partition,omitempty" yaml:"partition,omitempty"`
}

// StatefulSetOrdinals represents the ordinal numbering policy used by
// a StatefulSet to assign indices to its replicas.
type StatefulSetOrdinals struct {
	Start int32 `json:"start,omitempty" yaml:"start,omitempty"`
}

// StatefulSetStatus represents the observed state of a StatefulSet,
// including replica counts and current/update revisions.
type StatefulSetStatus struct {
	ObservedGeneration int64       `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
	Replicas           int32       `json:"replicas" yaml:"replicas"`
	ReadyReplicas      int32       `json:"readyReplicas,omitempty" yaml:"readyReplicas,omitempty"`
	CurrentReplicas    int32       `json:"currentReplicas,omitempty" yaml:"currentReplicas,omitempty"`
	UpdatedReplicas    int32       `json:"updatedReplicas,omitempty" yaml:"updatedReplicas,omitempty"`
	CurrentRevision    string      `json:"currentRevision,omitempty" yaml:"currentRevision,omitempty"`
	UpdateRevision     string      `json:"updateRevision,omitempty" yaml:"updateRevision,omitempty"`
	CollisionCount     *int32      `json:"collisionCount,omitempty" yaml:"collisionCount,omitempty"`
	Conditions         []Condition `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	AvailableReplicas  int32       `json:"availableReplicas,omitempty" yaml:"availableReplicas,omitempty"`
}

// DaemonSet represents a Kubernetes-style DaemonSet resource that
// ensures a copy of a pod runs on all (or selected) nodes.
type DaemonSet struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       DaemonSetSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     DaemonSetStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// DaemonSetList represents a collection of DaemonSet resources returned
// in a list response.
type DaemonSetList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []DaemonSet `json:"items" yaml:"items"`
}

// DaemonSetSpec represents the desired state of a DaemonSet, including
// the pod template, node selector and update strategy.
type DaemonSetSpec struct {
	Selector             *LabelSelector          `json:"selector" yaml:"selector"`
	Template             PodTemplateSpec         `json:"template" yaml:"template"`
	UpdateStrategy       DaemonSetUpdateStrategy `json:"updateStrategy,omitempty" yaml:"updateStrategy,omitempty"`
	MinReadySeconds      int32                   `json:"minReadySeconds,omitempty" yaml:"minReadySeconds,omitempty"`
	RevisionHistoryLimit *int32                  `json:"revisionHistoryLimit,omitempty" yaml:"revisionHistoryLimit,omitempty"`
}

// DaemonSetUpdateStrategy represents the strategy a DaemonSet uses to
// replace existing pods (rolling update or on delete).
type DaemonSetUpdateStrategy struct {
	Type          string                  `json:"type,omitempty" yaml:"type,omitempty"`
	RollingUpdate *RollingUpdateDaemonSet `json:"rollingUpdate,omitempty" yaml:"rollingUpdate,omitempty"`
}

// RollingUpdateDaemonSet represents the parameters controlling the
// rolling update of pods in a DaemonSet.
type RollingUpdateDaemonSet struct {
	MaxUnavailable *IntOrString `json:"maxUnavailable,omitempty" yaml:"maxUnavailable,omitempty"`
	MaxSurge       *IntOrString `json:"maxSurge,omitempty" yaml:"maxSurge,omitempty"`
}

// DaemonSetStatus represents the observed state of a DaemonSet,
// reporting the number of pods scheduled, ready and available on nodes.
type DaemonSetStatus struct {
	CurrentNumberScheduled int32       `json:"currentNumberScheduled" yaml:"currentNumberScheduled"`
	NumberMisscheduled     int32       `json:"numberMisscheduled" yaml:"numberMisscheduled"`
	DesiredNumberScheduled int32       `json:"desiredNumberScheduled" yaml:"desiredNumberScheduled"`
	NumberReady            int32       `json:"numberReady" yaml:"numberReady"`
	ObservedGeneration     int64       `json:"observedGeneration,omitempty" yaml:"observedGeneration,omitempty"`
	UpdatedNumberScheduled int32       `json:"updatedNumberScheduled,omitempty" yaml:"updatedNumberScheduled,omitempty"`
	NumberAvailable        int32       `json:"numberAvailable,omitempty" yaml:"numberAvailable,omitempty"`
	NumberUnavailable      int32       `json:"numberUnavailable,omitempty" yaml:"numberUnavailable,omitempty"`
	CollisionCount         *int32      `json:"collisionCount,omitempty" yaml:"collisionCount,omitempty"`
	Conditions             []Condition `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	DaemonSetsAllowed      int32       `json:"daemonSetsAllowed,omitempty" yaml:"daemonSetsAllowed,omitempty"`
}

// Job represents a Kubernetes-style Job resource that runs a set of
// pods to completion as a one-off task.
type Job struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       JobSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     JobStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// JobList represents a collection of Job resources returned in a list
// response.
type JobList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []Job `json:"items" yaml:"items"`
}

// JobSpec represents the desired state of a Job: parallelism,
// completion count, backoff limit and pod template.
type JobSpec struct {
	Parallelism             *int32            `json:"parallelism,omitempty" yaml:"parallelism,omitempty"`
	Completions             *int32            `json:"completions,omitempty" yaml:"completions,omitempty"`
	ActiveDeadlineSeconds   *int64            `json:"activeDeadlineSeconds,omitempty" yaml:"activeDeadlineSeconds,omitempty"`
	PodFailurePolicy        *PodFailurePolicy `json:"podFailurePolicy,omitempty" yaml:"podFailurePolicy,omitempty"`
	BackoffLimit            *int32            `json:"backoffLimit,omitempty" yaml:"backoffLimit,omitempty"`
	Selector                *LabelSelector    `json:"selector,omitempty" yaml:"selector,omitempty"`
	ManualSelector          *bool             `json:"manualSelector,omitempty" yaml:"manualSelector,omitempty"`
	Template                PodTemplateSpec   `json:"template" yaml:"template"`
	TTLSecondsAfterFinished *int32            `json:"ttlSecondsAfterFinished,omitempty" yaml:"ttlSecondsAfterFinished,omitempty"`
	CompletionMode          *string           `json:"completionMode,omitempty" yaml:"completionMode,omitempty"`
	Suspend                 *bool             `json:"suspend,omitempty" yaml:"suspend,omitempty"`
}

// PodFailurePolicy represents the policy a Job applies to handle pod
// failures, expressed as a list of PodFailurePolicyRule entries.
type PodFailurePolicy struct {
	Rules []PodFailurePolicyRule `json:"rules" yaml:"rules"`
}

// PodFailurePolicyRule represents a single rule that matches pod
// failures and decides which counter they should be counted against.
type PodFailurePolicyRule struct {
	Action          string                                   `json:"action" yaml:"action"`
	OnExitCodes     *PodFailurePolicyOnExitCodesRequirement  `json:"onExitCodes,omitempty" yaml:"onExitCodes,omitempty"`
	OnPodConditions []PodFailurePolicyOnPodConditionsPattern `json:"onPodConditions,omitempty" yaml:"onPodConditions,omitempty"`
}

// PodFailurePolicyOnExitCodesRequirement represents the requirement on
// container exit codes that triggers a PodFailurePolicyRule.
type PodFailurePolicyOnExitCodesRequirement struct {
	ContainerName *string `json:"containerName,omitempty" yaml:"containerName,omitempty"`
	Operator      string  `json:"operator" yaml:"operator"`
	Values        []int32 `json:"values" yaml:"values"`
}

// PodFailurePolicyOnPodConditionsPattern represents a pod condition
// pattern that, when matched, triggers a PodFailurePolicyRule.
type PodFailurePolicyOnPodConditionsPattern struct {
	Type   string `json:"type" yaml:"type"`
	Status string `json:"status" yaml:"status"`
}

// JobStatus represents the observed state of a Job, tracking active,
// succeeded and failed pod counts and timing information.
type JobStatus struct {
	Conditions              []Condition              `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	StartTime               *time.Time               `json:"startTime,omitempty" yaml:"startTime,omitempty"`
	CompletionTime          *time.Time               `json:"completionTime,omitempty" yaml:"completionTime,omitempty"`
	Active                  int32                    `json:"active,omitempty" yaml:"active,omitempty"`
	Succeeded               int32                    `json:"succeeded,omitempty" yaml:"succeeded,omitempty"`
	Failed                  int32                    `json:"failed,omitempty" yaml:"failed,omitempty"`
	CompletedIndexes        string                   `json:"completedIndexes,omitempty" yaml:"completedIndexes,omitempty"`
	UncountedTerminatedPods *UncountedTerminatedPods `json:"uncountedTerminatedPods,omitempty" yaml:"uncountedTerminatedPods,omitempty"`
	Ready                   *int32                   `json:"ready,omitempty" yaml:"ready,omitempty"`
}

// UncountedTerminatedPods represents pods whose termination is known to
// the Job controller but not yet counted against the Job's metrics.
type UncountedTerminatedPods struct {
	Succeeded []string `json:"succeeded,omitempty" yaml:"succeeded,omitempty"`
	Failed    []string `json:"failed,omitempty" yaml:"failed,omitempty"`
}

// CronJob represents a Kubernetes-style CronJob resource that runs Jobs
// on a recurring schedule.
type CronJob struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       CronJobSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     CronJobStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// CronJobList represents a collection of CronJob resources returned in
// a list response.
type CronJobList struct {
	TypeMeta `json:",inline" yaml:",inline"`
	ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items    []CronJob `json:"items" yaml:"items"`
}

// CronJobSpec represents the desired schedule and template for a
// CronJob, including concurrency policy and history limits.
type CronJobSpec struct {
	Schedule                   string          `json:"schedule" yaml:"schedule"`
	TimeZone                   *string         `json:"timeZone,omitempty" yaml:"timeZone,omitempty"`
	StartingDeadlineSeconds    *int64          `json:"startingDeadlineSeconds,omitempty" yaml:"startingDeadlineSeconds,omitempty"`
	ConcurrencyPolicy          string          `json:"concurrencyPolicy,omitempty" yaml:"concurrencyPolicy,omitempty"`
	Suspend                    *bool           `json:"suspend,omitempty" yaml:"suspend,omitempty"`
	JobTemplate                JobTemplateSpec `json:"jobTemplate" yaml:"jobTemplate"`
	SuccessfulJobsHistoryLimit *int32          `json:"successfulJobsHistoryLimit,omitempty" yaml:"successfulJobsHistoryLimit,omitempty"`
	FailedJobsHistoryLimit     *int32          `json:"failedJobsHistoryLimit,omitempty" yaml:"failedJobsHistoryLimit,omitempty"`
}

// JobTemplateSpec represents the template used by a CronJob to create
// new Jobs on each scheduled run.
type JobTemplateSpec struct {
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       JobSpec `json:"spec,omitempty" yaml:"spec,omitempty"`
}

// CronJobStatus represents the observed state of a CronJob, including
// a list of active Jobs and the last successful schedule time.
type CronJobStatus struct {
	Active             []ObjectReference `json:"active,omitempty" yaml:"active,omitempty"`
	LastScheduleTime   *time.Time        `json:"lastScheduleTime,omitempty" yaml:"lastScheduleTime,omitempty"`
	LastSuccessfulTime *time.Time        `json:"lastSuccessfulTime,omitempty" yaml:"lastSuccessfulTime,omitempty"`
}
