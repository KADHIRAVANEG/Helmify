// Package yaml parses raw Kubernetes manifest files (Deployment, Service,
// ConfigMap, Ingress) from a directory into the shared intermediate model.
package yaml

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/KADHIRAVANEG/helmify/internal/model"
	goyaml "gopkg.in/yaml.v3"
)

// rawDoc is a loose representation of "some k8s manifest" before we know
// its kind. We only decode the bits we care about for v0.1.
type rawDoc struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Metadata   rawMetadata       `yaml:"metadata"`
	Spec       map[string]any    `yaml:"spec"`
	Data       map[string]string `yaml:"data"`
}

type rawMetadata struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels"`
}

// Parse walks inputDir, reads every *.yaml/*.yml file (supporting
// multi-document files separated by "---"), and builds a Project.
func Parse(inputDir string, projectName string) (*model.Project, error) {
	proj := &model.Project{Name: projectName}

	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return nil, fmt.Errorf("reading input dir %q: %w", inputDir, err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		path := filepath.Join(inputDir, e.Name())
		if err := parseFile(path, proj); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
	}

	if len(proj.Workloads) == 0 {
		return nil, fmt.Errorf("no Deployment/StatefulSet resources found in %s", inputDir)
	}

	return proj, nil
}

func parseFile(path string, proj *model.Project) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	dec := goyaml.NewDecoder(strings.NewReader(string(raw)))
	for {
		var doc rawDoc
		if err := dec.Decode(&doc); err != nil {
			if err.Error() == "EOF" {
				break
			}
			// Skip documents that don't decode cleanly (e.g. empty "---" separators)
			break
		}
		if doc.Kind == "" {
			continue
		}
		applyDoc(doc, proj)
	}
	return nil
}

func applyDoc(doc rawDoc, proj *model.Project) {
	switch doc.Kind {
	case "Deployment", "StatefulSet":
		proj.Workloads = append(proj.Workloads, workloadFromRaw(doc))
	case "Service":
		proj.Services = append(proj.Services, serviceFromRaw(doc))
	case "ConfigMap":
		proj.ConfigMaps = append(proj.ConfigMaps, model.ConfigMap{
			Name: doc.Metadata.Name,
			Data: doc.Data,
		})
	case "Ingress":
		proj.Ingresses = append(proj.Ingresses, ingressFromRaw(doc))
	}
}

func workloadFromRaw(doc rawDoc) model.Workload {
	w := model.Workload{
		Kind:     doc.Kind,
		Name:     doc.Metadata.Name,
		Labels:   doc.Metadata.Labels,
		Replicas: 1,
	}

	if r, ok := doc.Spec["replicas"]; ok {
		if n, ok := toInt32(r); ok {
			w.Replicas = n
		}
	}

	tmplSpec, _ := doc.Spec["template"].(map[string]any)
	podSpec, _ := tmplSpec["spec"].(map[string]any)
	containers, _ := podSpec["containers"].([]any)

	if len(containers) > 0 {
		c, _ := containers[0].(map[string]any)
		w.Container = containerFromRaw(c)
	}

	return w
}

func containerFromRaw(c map[string]any) model.Container {
	cont := model.Container{}

	if name, ok := c["name"].(string); ok {
		cont.Name = name
	}
	if image, ok := c["image"].(string); ok {
		img, tag := splitImage(image)
		cont.Image = img
		cont.Tag = tag
	}

	if envList, ok := c["env"].([]any); ok {
		for _, e := range envList {
			em, ok := e.(map[string]any)
			if !ok {
				continue
			}
			name, _ := em["name"].(string)
			value, _ := em["value"].(string)
			cont.Env = append(cont.Env, model.EnvVar{Name: name, Value: value})
		}
	}

	if portList, ok := c["ports"].([]any); ok {
		for _, p := range portList {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			port := model.Port{Protocol: "TCP"}
			if n, ok := pm["name"].(string); ok {
				port.Name = n
			}
			if cp, ok := toInt32(pm["containerPort"]); ok {
				port.ContainerPort = cp
			}
			if proto, ok := pm["protocol"].(string); ok {
				port.Protocol = proto
			}
			cont.Ports = append(cont.Ports, port)
		}
	}

	if resMap, ok := c["resources"].(map[string]any); ok {
		if reqs, ok := resMap["requests"].(map[string]any); ok {
			if cpu, ok := reqs["cpu"].(string); ok {
				cont.Resources.RequestsCPU = cpu
			}
			if mem, ok := reqs["memory"].(string); ok {
				cont.Resources.RequestsMemory = mem
			}
		}
		if lims, ok := resMap["limits"].(map[string]any); ok {
			if cpu, ok := lims["cpu"].(string); ok {
				cont.Resources.LimitsCPU = cpu
			}
			if mem, ok := lims["memory"].(string); ok {
				cont.Resources.LimitsMemory = mem
			}
		}
	}

	return cont
}

func serviceFromRaw(doc rawDoc) model.Service {
	svc := model.Service{
		Name: doc.Metadata.Name,
		Type: "ClusterIP",
	}
	if t, ok := doc.Spec["type"].(string); ok {
		svc.Type = t
	}
	if portList, ok := doc.Spec["ports"].([]any); ok && len(portList) > 0 {
		pm, _ := portList[0].(map[string]any)
		if p, ok := toInt32(pm["port"]); ok {
			svc.Port = p
		}
		if tp, ok := toInt32(pm["targetPort"]); ok {
			svc.TargetPort = tp
		}
		if proto, ok := pm["protocol"].(string); ok {
			svc.Protocol = proto
		} else {
			svc.Protocol = "TCP"
		}
	}
	return svc
}

func ingressFromRaw(doc rawDoc) model.Ingress {
	ing := model.Ingress{Name: doc.Metadata.Name}
	if rules, ok := doc.Spec["rules"].([]any); ok && len(rules) > 0 {
		rule, _ := rules[0].(map[string]any)
		if host, ok := rule["host"].(string); ok {
			ing.Host = host
		}
		if httpMap, ok := rule["http"].(map[string]any); ok {
			if paths, ok := httpMap["paths"].([]any); ok && len(paths) > 0 {
				pathMap, _ := paths[0].(map[string]any)
				if p, ok := pathMap["path"].(string); ok {
					ing.Path = p
				}
			}
		}
	}
	if ing.Path == "" {
		ing.Path = "/"
	}
	return ing
}

// splitImage splits "repo/image:tag" into ("repo/image", "tag").
// Defaults tag to "latest" when none is present.
func splitImage(image string) (string, string) {
	idx := strings.LastIndex(image, ":")
	// Guard against a colon that's part of a registry port, e.g. localhost:5000/app
	slashIdx := strings.LastIndex(image, "/")
	if idx == -1 || idx < slashIdx {
		return image, "latest"
	}
	return image[:idx], image[idx+1:]
}

func toInt32(v any) (int32, bool) {
	switch n := v.(type) {
	case int:
		return int32(n), true
	case int64:
		return int32(n), true
	case float64:
		return int32(n), true
	default:
		return 0, false
	}
}
