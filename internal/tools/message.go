package tools

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/verialabs/ctf-agent/internal/bus"
)

// CheckFindingsTool reads unread findings from sibling solvers via the message bus.
type CheckFindingsTool struct {
	bus       *bus.MessageBus
	cursor    *int
	agentName string
}

func NewCheckFindingsTool(b *bus.MessageBus, cursor *int) *CheckFindingsTool {
	return NewCheckFindingsToolFor(b, cursor, "")
}

func NewCheckFindingsToolFor(b *bus.MessageBus, cursor *int, agentName string) *CheckFindingsTool {
	return &CheckFindingsTool{bus: b, cursor: cursor, agentName: agentName}
}

func (t *CheckFindingsTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "check_findings",
		Desc: "Check for findings shared by other solver agents working on the same challenge. " +
			"Use this when you're stuck or want to see what others have discovered.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{}),
	}, nil
}

func (t *CheckFindingsTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	items, nextCursor := t.bus.CheckFor(t.agentName, *t.cursor)
	*t.cursor = nextCursor

	if len(items) == 0 {
		return "No new findings from sibling solvers.", nil
	}

	result := ""
	for _, item := range items {
		result += fmt.Sprintf("[%s] %s\n", item.Author, item.Content)
	}
	return result, nil
}

// PostFindingTool lets a solver share a durable finding with sibling solvers.
type PostFindingTool struct {
	bus       *bus.MessageBus
	agentName string
}

func NewPostFindingTool(b *bus.MessageBus, agentName string) *PostFindingTool {
	return &PostFindingTool{bus: b, agentName: agentName}
}

func (t *PostFindingTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "post_finding",
		Desc: "Share a concise technical finding with sibling solver agents working on the same challenge.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"finding": {Type: schema.String, Desc: "A concise finding, lead, or failed approach worth sharing", Required: true},
		}),
	}, nil
}

func (t *PostFindingTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Finding string `json:"finding"`
	}
	if err := unmarshalArgs(argsJSON, &args); err != nil {
		return "", fmt.Errorf("post_finding: %w", err)
	}
	if args.Finding == "" {
		return "No finding posted: finding was empty.", nil
	}
	t.bus.Post(t.agentName, args.Finding)
	return "Finding shared with sibling solvers.", nil
}
