package main

import (
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
	"github.com/spf13/cobra"
)

var (
	input  string
	output string
	name   string
	secure bool
	lint   bool
	argocd bool
	flux   bool
	rbac   bool
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a Helm chart",
	Long:  `Generate a Helm chart from a directory of Kubernetes YAML manifests, a docker-compose.yml file, or a Dockerfile.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if input == "" {
			return fmt.Errorf("--input is required")
		}

		info, err := os.Stat(input)
		if err != nil {
			return fmt.Errorf("reading --input: %w", err)
		}

		chartName := name
		if chartName == "" {
			chartName = filepath.Base(strings.TrimRight(input, "/"))
		}

		var proj *model.Project

		switch {
		case info.IsDir():
			proj, err = yamlparser.Parse(input, chartName)
			if err != nil {
				return fmt.Errorf("parsing YAML input: %w", err)
			}
		case isComposeFile(input):
			if name == "" {
				abs, _ := filepath.Abs(input)
				chartName = filepath.Base(filepath.Dir(abs))
			}
			proj, err = composeparser.Parse(input, chartName)
			if err != nil {
				return fmt.Errorf("parsing docker-compose input: %w", err)
			}
		case isDockerfile(input):
			if name == "" {
				abs, _ := filepath.Abs(input)
				chartName = filepath.Base(filepath.Dir(abs))
			}
			proj, err = dockerfileparser.Parse(input, chartName)
			if err != nil {
				return fmt.Errorf("parsing Dockerfile input: %w", err)
			}
		default:
			return fmt.Errorf("--input must be a directory of YAML manifests, a docker-compose.yml file, or a Dockerfile")
		}

		if err := generator.Generate(proj, generator.Options{
			OutputDir: output,
			Secure:    secure,
			ArgoCD:    argocd,
			Flux:      flux,
			RBAC:      rbac,
		}); err != nil {
			return fmt.Errorf("generating chart: %w", err)
		}

		mode := "standard"
		if secure {
			mode = "secure (CIS-hardened)"
		}
		fmt.Printf("✓ Generated %s Helm chart %q at %s\n", mode, chartName, output)
		fmt.Printf("  workloads: %d  cronjobs: %d  services: %d  configmaps: %d  ingresses: %d\n",
			len(proj.Workloads), len(proj.CronJobs), len(proj.Services), len(proj.ConfigMaps), len(proj.Ingresses))
		fmt.Printf("  next: helm lint %s\n", output)

		if lint {
			if err := runHelmLint(output); err != nil {
				return err
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)

	generateCmd.Flags().StringVar(&input, "input", "", "path to input source (directory of YAML manifests)")
	generateCmd.Flags().StringVar(&output, "output", "./chart", "output directory for the generated Helm chart")
	generateCmd.Flags().StringVar(&name, "name", "", "chart/project name (defaults to the input directory name)")
	generateCmd.Flags().BoolVar(&secure, "secure", false, "apply CIS-aligned security hardening (securityContext, NetworkPolicy, etc.)")
	generateCmd.Flags().BoolVar(&lint, "lint", false, "run 'helm lint' on the generated chart automatically (requires helm on PATH)")
	generateCmd.Flags().BoolVar(&argocd, "argocd", false, "generate an ArgoCD Application manifest alongside the chart")
	generateCmd.Flags().BoolVar(&flux, "flux", false, "generate a Flux HelmRelease manifest alongside the chart")
	generateCmd.Flags().BoolVar(&rbac, "rbac", false, "generate ServiceAccount, Role, and RoleBinding manifests")
}

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
