package v1alpha1

type LLMEngineSpec struct {
	Model         ModelSpec         `json:"model"`
	Serving       ServingSpec       `json:"serving"`
	Autoscaling   AutoscalingSpec   `json:"autoscaling,omitempty"`
	Observability ObservabilitySpec `json:"observability,omitempty"`
	Routing       RoutingSpec       `json:"routing,omitempty"`
}

type ModelSpec struct {
	Name               string `json:"name"`
	Image              string `json:"image"`
	ModelURI           string `json:"modelURI"`
	TensorParallelSize int32  `json:"tensorParallelSize,omitempty"`
	MaxModelLen        int32  `json:"maxModelLen,omitempty"`
}

type ServingSpec struct {
	Replicas int32  `json:"replicas"`
	Port     int32  `json:"port"`
	GPU      int32  `json:"gpu"`
	CPU      string `json:"cpu,omitempty"`
	Memory   string `json:"memory,omitempty"`
}

type AutoscalingSpec struct {
	Enabled              bool  `json:"enabled"`
	MinReplicas          int32 `json:"minReplicas,omitempty"`
	MaxReplicas          int32 `json:"maxReplicas,omitempty"`
	TargetQueueDepth     int32 `json:"targetQueueDepth,omitempty"`
	TargetGPUUtilization int32 `json:"targetGPUUtilization,omitempty"`
}

type ObservabilitySpec struct {
	ServiceMonitor   bool `json:"serviceMonitor,omitempty"`
	GrafanaDashboard bool `json:"grafanaDashboard,omitempty"`
}

type RoutingSpec struct {
	Enabled          bool   `json:"enabled,omitempty"`
	Weight           int32  `json:"weight,omitempty"`
	FallbackProvider string `json:"fallbackProvider,omitempty"`
}

type LLMEngineStatus struct {
	Phase              string      `json:"phase,omitempty"`
	Endpoint           string      `json:"endpoint,omitempty"`
	AvailableReplicas  int32       `json:"availableReplicas,omitempty"`
	ObservedGeneration int64       `json:"observedGeneration,omitempty"`
	Conditions         []Condition `json:"conditions,omitempty"`
}

type Condition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}
