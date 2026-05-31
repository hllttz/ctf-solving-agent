package tools

import (
	"context"
	"fmt"

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
	content, err := t.sb.ReadFile(args.Path)
	if err != nil {
		return fmt.Sprintf("read_file failed for %q: %v\nTry list_files on the parent directory or use bash to inspect the path.", args.Path, err), nil
	}
	return content, nil
}

// WriteFileTool writes content to files in the sandbox workspace.
type WriteFileTool struct {
	sb sandbox.Sandbox
}

func NewWriteFileTool(sb sandbox.Sandbox) *WriteFileTool { return &WriteFileTool{sb: sb} }

func (t *WriteFileTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "write_file",
		Desc: "Write content to a file in the sandbox workspace.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path":    {Type: schema.String, Desc: "Absolute path to write", Required: true},
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
	if err := t.sb.WriteFile(args.Path, args.Content); err != nil {
		return "", err
	}
	return "File written successfully.", nil
}

// ListFilesTool lists files in sandbox directories.
type ListFilesTool struct {
	sb sandbox.Sandbox
}

func NewListFilesTool(sb sandbox.Sandbox) *ListFilesTool { return &ListFilesTool{sb: sb} }

func (t *ListFilesTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "list_files",
		Desc: "List files in a sandbox directory (runs ls -la).",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "Directory path (default: workspace)", Required: false},
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
		args.Path = t.sb.Workspace()
	}
	return t.sb.ListFiles(args.Path)
}
