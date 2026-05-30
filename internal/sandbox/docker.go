package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const execTimeout = 60 * time.Second

type DockerSandbox struct {
	containerID string
	containerName string
	workspace   string
	distfiles   string
	challengeDir string
	mu          sync.Mutex
}

func NewDocker(ctx context.Context, image, name, challengeDir string) (Sandbox, error) {
	workspace := "/workspace"
	distfiles := "/challenge/distfiles"

	workspaceHost := challengeDir + "/workspace"
	distfilesHost := challengeDir + "/distfiles"

	os.MkdirAll(workspaceHost, 0755)

	// Remove existing container with same name
	exec.CommandContext(ctx, "docker", "rm", "-f", name).Run()

	// Create and start container
	createCmd := exec.CommandContext(ctx, "docker", "run", "-d",
		"--name", name,
		"--network", "bridge",
		"--add-host", "host.docker.internal:host-gateway",
		"--cap-add", "SYS_ADMIN",
		"--cap-add", "SYS_PTRACE",
		"--cpus", "2",
		"--memory", "16g",
		"-v", distfilesHost+":"+distfiles+":ro",
		"-v", workspaceHost+":"+workspace,
		"-w", workspace,
		image,
		"sleep", "infinity",
	)

	output, err := createCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker run: %w: %s", err, string(output))
	}

	containerID := strings.TrimSpace(string(output))

	return &DockerSandbox{
		containerID:   containerID,
		containerName: name,
		workspace:     workspace,
		distfiles:     distfiles,
		challengeDir:  challengeDir,
	}, nil
}

func (s *DockerSandbox) Exec(ctx context.Context, command string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	timeoutCtx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "docker", "exec", "-i",
		s.containerName, "timeout", "60", "bash", "-c", command)

	output, err := cmd.CombinedOutput()
	outStr := string(output)

	const maxLen = 24000
	if len(outStr) > maxLen {
		outStr = outStr[:maxLen] + "\n... [output truncated]"
	}

	if err != nil {
		if outStr != "" {
			return outStr, nil
		}
		return "", fmt.Errorf("exec: %w", err)
	}

	return outStr, nil
}

func (s *DockerSandbox) ReadFile(path string) (string, error) {
	content, err := s.readFileViaDocker(path)
	if err != nil {
		return "", err
	}
	if isBinary(content) {
		return "[binary file - use view_image for images or exec for analysis]", nil
	}
	return string(content), nil
}

func (s *DockerSandbox) ReadFileBytes(path string) ([]byte, error) {
	return s.readFileViaDocker(path)
}

func (s *DockerSandbox) WriteFile(path, content string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "exec", "-i",
		s.containerName, "tee", path, ">", "/dev/null")
	cmd.Stdin = strings.NewReader(content)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("write_file: %w: %s", err, string(output))
	}
	return nil
}

func (s *DockerSandbox) ListFiles(dir string) (string, error) {
	return s.Exec(context.Background(), "ls -la "+shellEscape(dir))
}

func (s *DockerSandbox) readFileViaDocker(path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "exec", "-i",
		s.containerName, "cat", path)
	return cmd.Output()
}

func (s *DockerSandbox) Stop(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "stop", "-t", "5", s.containerName)
	return cmd.Run()
}

func (s *DockerSandbox) Remove(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", s.containerName)
	return cmd.Run()
}

func (s *DockerSandbox) Workspace() string   { return s.workspace }
func (s *DockerSandbox) Distfiles() string   { return s.distfiles }
func (s *DockerSandbox) ContainerID() string { return s.containerID }

func isBinary(data []byte) bool {
	for i := 0; i < len(data) && i < 512; i++ {
		if data[i] < 8 || (data[i] > 13 && data[i] < 32) {
			return true
		}
	}
	return false
}

func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
