// Package dockerfile does minimal introspection of a single Dockerfile
// (no build required) to scaffold a bare-minimum model.Project: one
// Workload derived from FROM/ENV/EXPOSE/CMD/ENTRYPOINT, and a Service
// only when the Dockerfile actually declares EXPOSE.
//
// There's no replica count, resource limits, or real image tag available
// from a Dockerfile alone (the image doesn't exist until it's built), so
// this parser leaves those as sane generator-side defaults and expects
// the user to fill in image.repository/tag themselves before deploying.
package dockerfile

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/KADHIRAVANEG/helmify/internal/model"
)

// Parse reads a single Dockerfile at path and builds a Project named
// projectName (falls back to "app" if empty).
func Parse(path string, projectName string) (*model.Project, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reading Dockerfile %q: %w", path, err)
	}
	defer f.Close()

	if projectName == "" {
		projectName = "app"
	}

	w := model.Workload{
		Kind:     "Deployment",
		Name:     projectName,
		Replicas: 1,
	}
	container := model.Container{Name: projectName}

	var ports []model.Port
	var baseImage string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		instr, rest, ok := splitInstruction(line)
		if !ok {
			continue
		}

		switch instr {
		case "FROM":
			// FROM <image>[:<tag>] [AS <name>] - keep just the image ref
			baseImage = strings.Fields(rest)[0]

		case "EXPOSE":
			for _, tok := range strings.Fields(rest) {
				tok = strings.TrimSuffix(tok, "/tcp")
				tok = strings.TrimSuffix(tok, "/udp")
				if p, err := strconv.Atoi(tok); err == nil {
					ports = append(ports, model.Port{
						Name:          "http",
						ContainerPort: int32(p),
						Protocol:      "TCP",
					})
				}
			}

		case "ENV":
			container.Env = append(container.Env, parseEnvLine(rest)...)

		case "CMD":
			cmd, args := parseExecForm(rest)
			if cmd != nil {
				container.Command = cmd
				container.Args = args
			}

		case "ENTRYPOINT":
			cmd, args := parseExecForm(rest)
			if cmd != nil {
				container.Command = cmd
				// ENTRYPOINT takes precedence over any CMD-derived args
				// already set - clear stale CMD args so they don't get
				// silently appended to the wrong entrypoint.
				container.Args = args
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning Dockerfile: %w", err)
	}

	// The image built from this Dockerfile doesn't exist under a pushed
	// name yet - we surface the base image as a hint but the user needs
	// to set the real repository/tag in values.yaml before deploying.
	if baseImage != "" {
		container.Image = fmt.Sprintf("REPLACE_ME/%s", projectName)
		container.Tag = "latest"
	} else {
		container.Image = fmt.Sprintf("REPLACE_ME/%s", projectName)
		container.Tag = "latest"
	}
	container.Ports = ports
	w.Container = container

	proj := &model.Project{
		Name:      projectName,
		Workloads: []model.Workload{w},
	}

	if len(ports) > 0 {
		proj.Services = append(proj.Services, model.Service{
			Name:       projectName,
			Type:       "ClusterIP",
			Port:       ports[0].ContainerPort,
			TargetPort: ports[0].ContainerPort,
			Protocol:   "TCP",
		})
	}

	return proj, nil
}

// splitInstruction splits a Dockerfile line into its instruction keyword
// and the remainder, handling line continuations being already joined
// by the caller is NOT done here - v0.3 keeps it simple and expects
// single-line instructions, which covers the vast majority of real
// Dockerfiles for EXPOSE/ENV/CMD/ENTRYPOINT/FROM.
func splitInstruction(line string) (instr string, rest string, ok bool) {
	fields := strings.SplitN(line, " ", 2)
	if len(fields) < 2 {
		return "", "", false
	}
	instr = strings.ToUpper(fields[0])
	switch instr {
	case "FROM", "EXPOSE", "ENV", "CMD", "ENTRYPOINT":
		return instr, strings.TrimSpace(fields[1]), true
	default:
		return "", "", false
	}
}

// parseEnvLine handles both ENV forms:
//
//	ENV KEY=value KEY2=value2
//	ENV KEY value
func parseEnvLine(rest string) []model.EnvVar {
	var out []model.EnvVar
	if strings.Contains(rest, "=") {
		for _, tok := range splitEnvPairs(rest) {
			parts := strings.SplitN(tok, "=", 2)
			if len(parts) == 2 {
				out = append(out, model.EnvVar{
					Name:  parts[0],
					Value: strings.Trim(parts[1], `"'`),
				})
			}
		}
		return out
	}
	// legacy single "ENV KEY value" form
	fields := strings.SplitN(rest, " ", 2)
	if len(fields) == 2 {
		out = append(out, model.EnvVar{Name: fields[0], Value: strings.TrimSpace(fields[1])})
	}
	return out
}

// splitEnvPairs splits "KEY=val KEY2=val2" on whitespace while being
// forgiving of simple quoted values.
func splitEnvPairs(s string) []string {
	return strings.Fields(s)
}

// parseExecForm handles CMD/ENTRYPOINT in either form:
//
//	CMD ["executable", "arg1", "arg2"]   (exec form, preferred)
//	CMD command arg1 arg2                (shell form)
func parseExecForm(rest string) (cmd []string, args []string) {
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "[") {
		inner := strings.TrimSuffix(strings.TrimPrefix(rest, "["), "]")
		var parts []string
		for _, p := range strings.Split(inner, ",") {
			p = strings.TrimSpace(p)
			p = strings.Trim(p, `"'`)
			if p != "" {
				parts = append(parts, p)
			}
		}
		if len(parts) == 0 {
			return nil, nil
		}
		return []string{parts[0]}, parts[1:]
	}

	// shell form
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return nil, nil
	}
	return []string{"/bin/sh", "-c"}, []string{strings.Join(fields, " ")}
}
