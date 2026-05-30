package tools

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/verialabs/ctf-agent/internal/sandbox"
)

// ViewImageTool reads an image file from the sandbox. Returns description for non-multimodal models.
type ViewImageTool struct {
	sb sandbox.Sandbox
}

func NewViewImageTool(sb sandbox.Sandbox) *ViewImageTool { return &ViewImageTool{sb: sb} }

func (t *ViewImageTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "view_image",
		Desc: "Read an image file from the sandbox. Works with PNG, JPG, GIF, BMP. " +
			"Use this to view screenshots, QR codes, stego images, etc.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "Path to the image file", Required: true},
		}),
	}, nil
}

func (t *ViewImageTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct{ Path string `json:"path"` }
	if err := unmarshalArgs(argsJSON, &args); err != nil {
		return "", fmt.Errorf("view_image: %w", err)
	}

	data, err := t.sb.ReadFileBytes(args.Path)
	if err != nil {
		return "", fmt.Errorf("view_image: %w", err)
	}

	if len(data) > 4*1024*1024 {
		return "", fmt.Errorf("view_image: file too large (%d bytes, max 4MB)", len(data))
	}

	mime := detectImageMime(data)
	if mime == "" {
		return "", fmt.Errorf("view_image: not a recognized image format")
	}

	return fmt.Sprintf("Image loaded: %s, %d bytes, type: %s", args.Path, len(data), mime), nil
}

func detectImageMime(data []byte) string {
	if len(data) < 4 {
		return ""
	}
	// PNG
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "image/png"
	}
	// JPEG
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}
	// GIF
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 {
		return "image/gif"
	}
	// BMP
	if data[0] == 0x42 && data[1] == 0x4D {
		return "image/bmp"
	}
	// WebP
	if len(data) >= 12 && data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		return "image/webp"
	}
	return ""
}
