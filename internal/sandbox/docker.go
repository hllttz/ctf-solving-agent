package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const execTimeout = 60 * time.Second
const ContainerLabel = "ctf-agent"

type DockerOptions struct {
	Image        string
	Name         string
	ChallengeDir string
	MemoryLimit  string
}

type DockerSandbox struct {
	containerID   string
	containerName string
	workspace     string
	distfiles     string
	challengeDir  string
	mu            sync.Mutex
}

func NewDocker(ctx context.Context, image, name, challengeDir string) (Sandbox, error) {
	return NewDockerWithOptions(ctx, DockerOptions{
		Image:        image,
		Name:         name,
		ChallengeDir: challengeDir,
		MemoryLimit:  "16g",
	})
}

func NewDockerWithOptions(ctx context.Context, opts DockerOptions) (Sandbox, error) {
	workspace := "/workspace"
	distfiles := "/challenge/distfiles"

	memoryLimit := strings.TrimSpace(opts.MemoryLimit)
	if memoryLimit == "" {
		memoryLimit = "16g"
	}

	challengeDir, err := filepath.Abs(opts.ChallengeDir)
	if err != nil {
		return nil, fmt.Errorf("resolve challenge dir: %w", err)
	}
	workspaceHost := filepath.Join(challengeDir, "workspace")
	distfilesHost := filepath.Join(challengeDir, "distfiles")
	metadataHost := filepath.Join(challengeDir, "metadata.yml")

	os.MkdirAll(workspaceHost, 0755)

	// Remove existing container with same name
	exec.CommandContext(ctx, "docker", "rm", "-f", opts.Name).Run()

	// Create and start container
	mountMetadata := false
	if _, err := os.Stat(metadataHost); err == nil {
		mountMetadata = true
	}
	args := dockerRunArgs(opts, workspaceHost, distfilesHost, metadataHost, mountMetadata)
	createCmd := exec.CommandContext(ctx, "docker", args...)

	output, err := createCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker run: %w: %s", err, string(output))
	}

	containerID := strings.TrimSpace(string(output))

	return &DockerSandbox{
		containerID:   containerID,
		containerName: opts.Name,
		workspace:     workspace,
		distfiles:     distfiles,
		challengeDir:  challengeDir,
	}, nil
}

func CleanupOrphans(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "ps", "-aq", "--filter", "label="+ContainerLabel+"=true")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("list orphan containers: %w", err)
	}
	ids := strings.Fields(string(output))
	if len(ids) == 0 {
		return nil
	}
	args := append([]string{"rm", "-f"}, ids...)
	if output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("remove orphan containers: %w: %s", err, string(output))
	}
	return nil
}

func dockerRunArgs(opts DockerOptions, workspaceHost, distfilesHost, metadataHost string, mountMetadata bool) []string {
	workspace := "/workspace"
	distfiles := "/challenge/distfiles"
	metadata := "/challenge/metadata.yml"

	memoryLimit := strings.TrimSpace(opts.MemoryLimit)
	if memoryLimit == "" {
		memoryLimit = "16g"
	}

	args := []string{
		"run", "-d",
		"--name", opts.Name,
		"--label", ContainerLabel + "=true",
		"--network", "bridge",
		"--add-host", "host.docker.internal:host-gateway",
		"--cap-add", "SYS_ADMIN",
		"--cap-add", "SYS_PTRACE",
		"--cpus", "2",
		"--memory", memoryLimit,
		"-v", distfilesHost + ":" + distfiles + ":ro",
		"-v", workspaceHost + ":" + workspace,
	}
	if mountMetadata {
		args = append(args, "-v", metadataHost+":"+metadata+":ro")
	}
	return append(args,
		"-w", workspace,
		opts.Image,
		"sleep", "infinity",
	)
}

func (s *DockerSandbox) Exec(ctx context.Context, command string) (string, error) {
	result, err := s.ExecWithTimeout(ctx, command, int(execTimeout.Seconds()))
	if err != nil {
		return "", err
	}
	return FormatExecResult(result), nil
}

func (s *DockerSandbox) ExecWithTimeout(ctx context.Context, command string, timeoutSeconds int) (*ExecResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if timeoutSeconds <= 0 {
		timeoutSeconds = int(execTimeout.Seconds())
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds+5)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "docker", "exec", "-i",
		s.containerName, "timeout", "--signal=KILL", "--kill-after=5", strconv.Itoa(timeoutSeconds), "bash", "-c", command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := &ExecResult{
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}

	if err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			result.ExitCode = -1
			result.Stderr = appendLine(result.Stderr, "Command timed out")
			return result, nil
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return nil, fmt.Errorf("exec: %w", err)
	}

	return result, nil
}

func (s *DockerSandbox) ReadFile(path string) (string, error) {
	content, err := s.readFileViaDocker(path)
	if err != nil {
		return "", err
	}
	if isBinary(content) {
		return BinaryFileHint(path, len(content)), nil
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
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("read_file %q: %w: %s", path, err, strings.TrimSpace(string(output)))
	}
	return output, nil
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

func BinaryFileHint(path string, size int) string {
	return fmt.Sprintf("Binary file (%d bytes). Use bash to inspect it:\n  file %s\n  xxd %s | head -40\n  strings %s | head -80\n  exiftool %s\n  binwalk %s",
		size, shellEscape(path), shellEscape(path), shellEscape(path), shellEscape(path), shellEscape(path))
}

func appendLine(s, line string) string {
	if strings.TrimSpace(s) == "" {
		return line
	}
	return strings.TrimRight(s, "\n") + "\n" + line
}
