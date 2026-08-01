// Package generator scaffolds a full Helm chart (Chart.yaml, values.yaml,
// templates/*) on disk from a model.Project.
package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/KADHIRAVANEG/helmify/internal/model"
)

// Options controls how the chart is generated.
type Options struct {
	OutputDir string
	Secure    bool // when true, the hardening package is applied before writing
}

// Generate writes a complete Helm chart for proj into opts.OutputDir.
func Generate(proj *model.Project, opts Options) error {
	dirs := []string{
		opts.OutputDir,
		filepath.Join(opts.OutputDir, "templates"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", d, err)
		}
	}

	if err := writeFile(filepath.Join(opts.OutputDir, "Chart.yaml"), chartYAMLTemplate, proj); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(opts.OutputDir, "values.yaml"), valuesYAMLTemplate, proj); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(opts.OutputDir, ".helmignore"), helmignoreTemplate, proj); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(opts.OutputDir, "templates", "_helpers.tpl"), helpersTemplate, proj); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(opts.OutputDir, "templates", "NOTES.txt"), notesTemplate, proj); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(opts.OutputDir, "README.md"), chartReadmeTemplate, proj); err != nil {
		return err
	}

	for _, w := range proj.Workloads {
		fname := fmt.Sprintf("%s-%s.yaml", strings.ToLower(w.Kind), sanitize(w.Name))
		if err := writeFile(filepath.Join(opts.OutputDir, "templates", fname), workloadTemplate, workloadView{Project: proj, Workload: w, Secure: opts.Secure}); err != nil {
			return err
		}
		if opts.Secure {
			npFile := fmt.Sprintf("networkpolicy-%s.yaml", sanitize(w.Name))
			if err := writeFile(filepath.Join(opts.OutputDir, "templates", npFile), networkPolicyTemplate, workloadView{Project: proj, Workload: w, Secure: opts.Secure}); err != nil {
				return err
			}
		}
	}

	for _, s := range proj.Services {
		fname := fmt.Sprintf("service-%s.yaml", sanitize(s.Name))
		if err := writeFile(filepath.Join(opts.OutputDir, "templates", fname), serviceTemplate, serviceView{Project: proj, Service: s}); err != nil {
			return err
		}
	}

	for _, cm := range proj.ConfigMaps {
		fname := fmt.Sprintf("configmap-%s.yaml", sanitize(cm.Name))
		if err := writeFile(filepath.Join(opts.OutputDir, "templates", fname), configMapTemplate, configMapView{Project: proj, ConfigMap: cm}); err != nil {
			return err
		}
	}

	for _, ing := range proj.Ingresses {
		fname := fmt.Sprintf("ingress-%s.yaml", sanitize(ing.Name))
		if err := writeFile(filepath.Join(opts.OutputDir, "templates", fname), ingressTemplate, ingressView{Project: proj, Ingress: ing}); err != nil {
			return err
		}
	}

	return nil
}

type workloadView struct {
	Project  *model.Project
	Workload model.Workload
	Secure   bool
}

type serviceView struct {
	Project *model.Project
	Service model.Service
}

type configMapView struct {
	Project   *model.Project
	ConfigMap model.ConfigMap
}

type ingressView struct {
	Project *model.Project
	Ingress model.Ingress
}

func sanitize(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

func writeFile(path, tmplText string, data any) error {
	t, err := template.New(filepath.Base(path)).Funcs(template.FuncMap{
		"default": func(def, val string) string {
			if val == "" {
				return def
			}
			return val
		},
	}).Parse(tmplText)
	if err != nil {
		return fmt.Errorf("parsing template for %s: %w", path, err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()

	if err := t.Execute(f, data); err != nil {
		return fmt.Errorf("rendering %s: %w", path, err)
	}
	return nil
}
