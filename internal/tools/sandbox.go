package tools

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/verialabs/ctf-agent/internal/sandbox"
)

// BashTool executes commands inside the sandbox container.
type BashTool struct {
	sb sandbox.Sandbox
}

func NewBashTool(sb sandbox.Sandbox) *BashTool { return &BashTool{sb: sb} }

func (t *BashTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "bash",
		Desc: "Execute a bash command in the sandbox. Use for running CTF tools, " +
			"analyzing files, debugging binaries. Output truncated at ~24K chars.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"command":         {Type: schema.String, Desc: "Bash command to execute", Required: true},
			"timeout_seconds": {Type: schema.Integer, Desc: "Optional command timeout in seconds (default 60)", Required: false},
		}),
	}, nil
}

func (t *BashTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := unmarshalArgs(argsJSON, &args); err != nil {
		return "", fmt.Errorf("bash: %w", err)
	}
	result, err := t.sb.ExecWithTimeout(ctx, args.Command, args.TimeoutSeconds)
	if err != nil {
		return "", err
	}
	return sandbox.FormatExecResult(result), nil
}

// ReadFileTool reads files from the sandbox.
type ReadFileTool struct {
	sb sandbox.Sandbox
}

func NewReadFileTool(sb sandbox.Sandbox) *ReadFileTool { return &ReadFileTool{sb: sb} }

func (t *ReadFileTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "read_file",
		Desc: "Read a file from the sandbox. Binary files are detected automatically.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "Absolute path to the file", Required: true},
		}),
	}, nil
}

func (t *ReadFileTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := unmarshalArgs(argsJSON, &args); err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	requested := strings.TrimSpace(args.Path)
	var tried []string
	var lastErr error
	for _, candidate := range readFileCandidates(t.sb, requested) {
		tried = append(tried, candidate)
		content, err := t.sb.ReadFile(candidate)
		if err == nil {
			return content, nil
		}
		lastErr = err
	}
	return fmt.Sprintf("read_file failed for %q: %v\nTried: %s\nTry list_files on the parent directory or use bash to inspect the path.", requested, lastErr, strings.Join(tried, ", ")), nil
}

func readFileCandidates(sb sandbox.Sandbox, requested string) []string {
	if requested == "" || path.IsAbs(requested) {
		return []string{requested}
	}
	candidates := challengeRelativeCandidates(requested, sb.Distfiles(), sb.Workspace())
	if len(candidates) == 0 {
		return []string{requested}
	}
	return candidates
}

func challengeRelativeCandidates(requested string, bases ...string) []string {
	rel := cleanRelativePath(requested)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return nil
	}
	candidates := make([]string, 0, len(bases))
	for _, base := range bases {
		if candidate, ok := safeJoin(base, rel); ok {
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func safeJoin(base, rel string) (string, bool) {
	base = path.Clean(base)
	joined := path.Join(base, rel)
	return joined, joined == base || strings.HasPrefix(joined, base+"/")
}

func cleanRelativePath(p string) string {
	return path.Clean(strings.ReplaceAll(p, "\\", "/"))
}

// WriteFileTool writes content to files in the sandbox workspace.
type WriteFileTool struct {
	sb sandbox.Sandbox
}

func NewWriteFileTool(sb sandbox.Sandbox) *WriteFileTool { return &WriteFileTool{sb: sb} }

func (t *WriteFileTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "write_file",
		Desc: "Write content to a file in the sandbox workspace. Relative paths are written under /workspace.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path":    {Type: schema.String, Desc: "Path to write. Relative paths resolve under /workspace.", Required: true},
			"content": {Type: schema.String, Desc: "Content to write", Required: true},
		}),
	}, nil
}

func (t *WriteFileTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := unmarshalArgs(argsJSON, &args); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}
	target := writeFilePath(t.sb, strings.TrimSpace(args.Path))
	if err := t.sb.WriteFile(target, args.Content); err != nil {
		return "", err
	}
	return fmt.Sprintf("File written successfully: %s", target), nil
}

func writeFilePath(sb sandbox.Sandbox, requested string) string {
	if requested == "" || path.IsAbs(requested) {
		return requested
	}
	candidates := challengeRelativeCandidates(requested, sb.Workspace())
	if len(candidates) == 0 {
		return requested
	}
	return candidates[0]
}

// ListFilesTool lists files in sandbox directories.
type ListFilesTool struct {
	sb sandbox.Sandbox
}

func NewListFilesTool(sb sandbox.Sandbox) *ListFilesTool { return &ListFilesTool{sb: sb} }

func (t *ListFilesTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "list_files",
		Desc: "List files in a sandbox directory (runs ls -la). Defaults to /challenge/distfiles.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "Directory path (default: /challenge/distfiles)", Required: false},
		}),
	}, nil
}

func (t *ListFilesTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := unmarshalArgs(argsJSON, &args); err != nil {
		return "", fmt.Errorf("list_files: %w", err)
	}
	if args.Path == "" {
		args.Path = t.sb.Distfiles()
	}
	return t.sb.ListFiles(args.Path)
}
