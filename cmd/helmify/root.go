package main

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "helmify",
	Short: "helmify generates Helm charts from existing project artifacts",
	Long: `helmify is a CLI tool that generates production-ready Helm charts from existing project artifacts
— raw Kubernetes YAML manifests, docker-compose.yml files, or even a bare Dockerfile.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
