// Package model defines the intermediate representation that every parser
// (yaml, compose, dockerfile) produces and that the generator consumes.
// Keeping one shared shape means the generator doesn't need to know or care
// which input source it came from.
package model

// EnvVar is a single environment variable.
type EnvVar struct {
	Name  string
	Value string
}

// Port is a single container port.
type Port struct {
	Name          string
	ContainerPort int32
	Protocol      string // "TCP" or "UDP", defaults to "TCP"
}

// Resources mirrors a k8s ResourceRequirements, kept as plain strings
// (e.g. "250m", "256Mi") so we can pass them straight through to templates.
type Resources struct {
	RequestsCPU    string
	RequestsMemory string
	LimitsCPU      string
	LimitsMemory   string
}

// Container represents a single container within a workload.
type Container struct {
	Name      string
	Image     string
	Tag       string
	Env       []EnvVar
	Ports     []Port
	Resources Resources
	Command   []string
	Args      []string
}

// Workload represents a Deployment/StatefulSet-like resource.
type Workload struct {
	Kind      string // "Deployment", "StatefulSet", etc. Defaults to "Deployment".
	Name      string
	Replicas  int32
	Labels    map[string]string
	Container Container
}

// Service represents a k8s Service to expose a Workload.
type Service struct {
	Name       string
	Type       string // ClusterIP, NodePort, LoadBalancer
	Port       int32
	TargetPort int32
	Protocol   string
}

// ConfigMap represents non-secret configuration data.
type ConfigMap struct {
	Name string
	Data map[string]string
}

// Ingress represents a minimal Ingress definition.
type Ingress struct {
	Name  string
	Host  string
	Path  string
	Port  int32
	Class string
}

// Project is the full intermediate model for one input source.
// Every parser (yaml/compose/dockerfile) builds one of these; the
// generator only ever reads from a Project.
type Project struct {
	Name       string
	Workloads  []Workload
	Services   []Service
	ConfigMaps []ConfigMap
	Ingresses  []Ingress
}
