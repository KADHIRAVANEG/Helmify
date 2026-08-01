// Package compose parses a docker-compose.yml file (version 2/3 style)
// into the shared intermediate model. Each compose "service" becomes one
// model.Workload (+ a matching model.Service when ports are published).
package compose

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/KADHIRAVANEG/helmify/internal/model"
	goyaml "gopkg.in/yaml.v3"
)

// rawCompose is a loose representation of a docker-compose file.
// We only decode the fields v0.2 needs.
type rawCompose struct {
	Services map[string]rawService `yaml:"services"`
}

type rawService struct {
	Image       string   `yaml:"image"`
	Ports       []any    `yaml:"ports"` // can be "8080:80" strings or maps
	Environment any      `yaml:"environment"` // can be a map OR a list of "KEY=VALUE" strings
	Command     any      `yaml:"command"`     // string or list
	Deploy      rawDeploy `yaml:"deploy"`
}

type rawDeploy struct {
	Replicas  int              `yaml:"replicas"`
	Resources rawDeployResources `yaml:"resources"`
}

type rawDeployResources struct {
	Limits   rawResourceSpec `yaml:"limits"`
	Reservations rawResourceSpec `yaml:"reservations"`
}

type rawResourceSpec struct {
	CPUs   string `yaml:"cpus"`
	Memory string `yaml:"memory"`
}

// Parse reads composePath and builds a Project. projectName defaults to
// the "name:" field if present, else the caller-supplied fallback.
func Parse(composePath string, projectName string) (*model.Project, error) {
	raw, err := os.ReadFile(composePath)
	if err != nil {
		return nil, fmt.Errorf("reading compose file %q: %w", composePath, err)
	}

	var rc rawCompose
	if err := goyaml.Unmarshal(raw, &rc); err != nil {
		return nil, fmt.Errorf("parsing compose YAML: %w", err)
	}

	if len(rc.Services) == 0 {
		return nil, fmt.Errorf("no services found in %s", composePath)
	}

	proj := &model.Project{Name: projectName}

	for name, svc := range rc.Services {
		w := model.Workload{
			Kind:     "Deployment",
			Name:     sanitizeName(name),
			Replicas: 1,
		}
		if svc.Deploy.Replicas > 0 {
			w.Replicas = int32(svc.Deploy.Replicas)
		}

		image, tag := splitImage(svc.Image)
		container := model.Container{
			Name:  sanitizeName(name),
			Image: image,
			Tag:   tag,
		}

		container.Env = parseEnvironment(svc.Environment)
		container.Command, container.Args = parseCommand(svc.Command)
		container.Resources = parseResources(svc.Deploy.Resources)

		var svcPorts []model.Port
		for _, p := range svc.Ports {
			containerPort, hostPort, ok := parsePort(p)
			if !ok {
				continue
			}
			container.Ports = append(container.Ports, model.Port{
				Name:          "http",
				ContainerPort: containerPort,
				Protocol:      "TCP",
			})
			svcPorts = append(svcPorts, model.Port{ContainerPort: hostPort})
		}

		w.Container = container
		proj.Workloads = append(proj.Workloads, w)

		// Publish a Service for the first exposed port, if any -
		// mirrors how compose's "ports:" implies reachability.
		if len(container.Ports) > 0 {
			proj.Services = append(proj.Services, model.Service{
				Name:       sanitizeName(name),
				Type:       "ClusterIP",
				Port:       container.Ports[0].ContainerPort,
				TargetPort: container.Ports[0].ContainerPort,
				Protocol:   "TCP",
			})
		}
	}

	return proj, nil
}

// parsePort handles both compose port shapes:
//   - "8080:80" / "80" (string, "host:container" or just "container")
//   - {target: 80, published: 8080} (map form)
// Returns (containerPort, hostPort, ok).
func parsePort(p any) (int32, int32, bool) {
	switch v := p.(type) {
	case string:
		parts := strings.Split(v, ":")
		if len(parts) == 2 {
			host, err1 := strconv.Atoi(parts[0])
			cont, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil {
				return int32(cont), int32(host), true
			}
		}
		if len(parts) == 1 {
			cont, err := strconv.Atoi(parts[0])
			if err == nil {
				return int32(cont), int32(cont), true
			}
		}
	case map[string]any:
		target, _ := toInt(v["target"])
		published, _ := toInt(v["published"])
		if published == 0 {
			published = target
		}
		if target > 0 {
			return int32(target), int32(published), true
		}
	}
	return 0, 0, false
}

// parseEnvironment handles both compose environment shapes:
//   - map: { KEY: value }
//   - list: ["KEY=value", "KEY2=value2"]
func parseEnvironment(env any) []model.EnvVar {
	var out []model.EnvVar
	switch v := env.(type) {
	case map[string]any:
		for k, val := range v {
			out = append(out, model.EnvVar{Name: k, Value: fmt.Sprintf("%v", val)})
		}
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			parts := strings.SplitN(s, "=", 2)
			if len(parts) == 2 {
				out = append(out, model.EnvVar{Name: parts[0], Value: parts[1]})
			}
		}
	}
	return out
}

// parseCommand handles both compose command shapes: a single string
// (shell form) or a list of strings (exec form).
func parseCommand(cmd any) (command []string, args []string) {
	switch v := cmd.(type) {
	case string:
		return []string{"/bin/sh", "-c"}, []string{v}
	case []any:
		var parts []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			return []string{parts[0]}, parts[1:]
		}
	}
	return nil, nil
}

func parseResources(r rawDeployResources) model.Resources {
	res := model.Resources{}
	if r.Reservations.CPUs != "" {
		res.RequestsCPU = cpuToK8s(r.Reservations.CPUs)
	}
	if r.Reservations.Memory != "" {
		res.RequestsMemory = r.Reservations.Memory
	}
	if r.Limits.CPUs != "" {
		res.LimitsCPU = cpuToK8s(r.Limits.CPUs)
	}
	if r.Limits.Memory != "" {
		res.LimitsMemory = r.Limits.Memory
	}
	return res
}

// cpuToK8s converts compose's fractional-core notation ("0.5") into
// Kubernetes millicpu notation ("500m").
func cpuToK8s(cpus string) string {
	f, err := strconv.ParseFloat(cpus, 64)
	if err != nil {
		return cpus
	}
	return fmt.Sprintf("%dm", int(f*1000))
}

func splitImage(image string) (string, string) {
	idx := strings.LastIndex(image, ":")
	slashIdx := strings.LastIndex(image, "/")
	if idx == -1 || idx < slashIdx {
		return image, "latest"
	}
	return image[:idx], image[idx+1:]
}

func sanitizeName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
