// Command helmify generates a Helm chart from an existing project's
// Kubernetes manifests (v0.1: YAML input only).
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/KADHIRAVANEG/helmify/internal/generator"
	"github.com/KADHIRAVANEG/helmify/internal/model"
	composeparser "github.com/KADHIRAVANEG/helmify/internal/parser/compose"
	dockerfileparser "github.com/KADHIRAVANEG/helmify/internal/parser/dockerfile"
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
	lint := fs.Bool("lint", false, "run 'helm lint' on the generated chart automatically (requires helm on PATH)")
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
	case isDockerfile(*input):
		if *name == "" {
			abs, _ := filepath.Abs(*input)
			chartName = filepath.Base(filepath.Dir(abs))
		}
		proj, err = dockerfileparser.Parse(*input, chartName)
		if err != nil {
			return fmt.Errorf("parsing Dockerfile input: %w", err)
		}
	default:
		return fmt.Errorf("--input must be a directory of YAML manifests, a docker-compose.yml file, or a Dockerfile")
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

	if *lint {
		if err := runHelmLint(*output); err != nil {
			return err
		}
	}
	return nil
}

// runHelmLint shells out to "helm lint" on the generated chart. It requires
// helm to be installed and on PATH - if it isn't, we report that clearly
// rather than failing generation itself (the chart was still generated fine).
func runHelmLint(chartDir string) error {
	if _, err := exec.LookPath("helm"); err != nil {
		fmt.Println("  ⚠ --lint requested but 'helm' was not found on PATH; skipping lint (install helm to enable this check)")
		return nil
	}

	fmt.Println("  running helm lint...")
	cmd := exec.Command("helm", "lint", chartDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm lint failed: %w", err)
	}
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

func isDockerfile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return base == "dockerfile" || strings.HasPrefix(base, "dockerfile.")
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
  helmify generate --input ./manifests --output ./chart --secure
  helmify generate --input docker-compose.yml --output ./chart
  helmify generate --input Dockerfile --name my-app --output ./chart`)
}
