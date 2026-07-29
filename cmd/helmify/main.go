// Command helmify generates a Helm chart from an existing project's
// Kubernetes manifests (v0.1: YAML input only).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/KADHIRAVANEG/helmify/internal/generator"
	"github.com/KADHIRAVANEG/helmify/internal/model"
	composeparser "github.com/KADHIRAVANEG/helmify/internal/parser/compose"
	yamlparser "github.com/KADHIRAVANEG/helmify/internal/parser/yaml"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "generate":
		if err := runGenerate(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "helmify: error:", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "helmify: unknown command %q\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func runGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	input := fs.String("input", "", "path to input source (directory of YAML manifests)")
	output := fs.String("output", "./chart", "output directory for the generated Helm chart")
	name := fs.String("name", "", "chart/project name (defaults to the input directory name)")
	secure := fs.Bool("secure", false, "apply CIS-aligned security hardening (securityContext, NetworkPolicy, etc.)")
	fs.Parse(args)

	if *input == "" {
		return fmt.Errorf("--input is required")
	}

	info, err := os.Stat(*input)
	if err != nil {
		return fmt.Errorf("reading --input: %w", err)
	}

	chartName := *name
	if chartName == "" {
		chartName = filepath.Base(strings.TrimRight(*input, "/"))
	}

	var proj *model.Project

	switch {
	case info.IsDir():
		proj, err = yamlparser.Parse(*input, chartName)
		if err != nil {
			return fmt.Errorf("parsing YAML input: %w", err)
		}
	case isComposeFile(*input):
		if *name == "" {
			// compose files are usually named docker-compose.yml, so the
			// filename itself isn't a good default chart name - fall back
			// to the parent directory name instead.
			abs, _ := filepath.Abs(*input)
			chartName = filepath.Base(filepath.Dir(abs))
		}
		proj, err = composeparser.Parse(*input, chartName)
		if err != nil {
			return fmt.Errorf("parsing docker-compose input: %w", err)
		}
	default:
		return fmt.Errorf("--input must be a directory of YAML manifests or a docker-compose.yml file (Dockerfile-only support coming in v0.3)")
	}

	if err := generator.Generate(proj, generator.Options{
		OutputDir: *output,
		Secure:    *secure,
	}); err != nil {
		return fmt.Errorf("generating chart: %w", err)
	}

	mode := "standard"
	if *secure {
		mode = "secure (CIS-hardened)"
	}
	fmt.Printf("✓ Generated %s Helm chart %q at %s\n", mode, chartName, *output)
	fmt.Printf("  workloads: %d  services: %d  configmaps: %d  ingresses: %d\n",
		len(proj.Workloads), len(proj.Services), len(proj.ConfigMaps), len(proj.Ingresses))
	fmt.Printf("  next: helm lint %s\n", *output)
	return nil
}

// isComposeFile does a lightweight filename check rather than trying to
// sniff content - compose files are conventionally named one of these.
func isComposeFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml":
		return true
	}
	return false
}

func printUsage() {
	fmt.Println(`helmify - generate Helm charts from existing project artifacts

Usage:
  helmify generate --input <dir> --output <dir> [--name <name>] [--secure]

Flags:
  --input    path to a directory of Kubernetes YAML manifests
  --output   output directory for the generated chart (default "./chart")
  --name     chart name (defaults to the input directory's base name)
  --secure   apply CIS-aligned hardening (securityContext, NetworkPolicy, etc.)

Examples:
  helmify generate --input ./manifests --output ./chart
  helmify generate --input ./manifests --output ./chart --secure`)
}
