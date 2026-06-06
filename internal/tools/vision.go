package tools

import (
	"context"
	"fmt"
	"strings"

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
		Desc: "Read an image file from the sandbox. Works with PNG, JPG, GIF, BMP, WebP, TIFF. " +
			"Use this to view screenshots, QR codes, stego images, etc.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Desc: "Path to the image file", Required: true},
		}),
	}, nil
}

func (t *ViewImageTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := unmarshalArgs(argsJSON, &args); err != nil {
		return "", fmt.Errorf("view_image: %w", err)
	}

	path, data, err := t.readImage(args.Path)
	if err != nil {
		return fmt.Sprintf("view_image failed for %q: %v\nTry list_files on the parent directory or use bash commands such as file, xxd, pngcheck, identify, binwalk, or foremost to inspect and repair the file.", args.Path, err), nil
	}

	if len(data) > 4*1024*1024 {
		return fmt.Sprintf("view_image skipped %q: file too large (%d bytes, max 4MB).\nUse bash tools such as file, exiftool, strings, binwalk, or Python/OpenCV to inspect it.", path, len(data)), nil
	}

	mime := detectImageMime(data)
	if mime == "" {
		return fmt.Sprintf("view_image could not recognize %q as PNG/JPEG/GIF/BMP/WebP/TIFF (%d bytes).\nUse bash tools such as file, xxd, pngcheck, identify, binwalk, foremost, or Python to inspect magic bytes and repair the image before retrying.", path, len(data)), nil
	}

	return fmt.Sprintf(`Image loaded: %s
Size: %d bytes
Type: %s

Suggested next commands:
- file %s
- exiftool %s
- strings %s | head -80
- binwalk %s
- zsteg %s
If this is a QR/screenshot/visual clue and the model cannot inspect pixels directly, use bash tools such as zbarimg, tesseract, or python/OpenCV in /workspace.`,
		path, len(data), mime, shellQuote(path), shellQuote(path), shellQuote(path), shellQuote(path), shellQuote(path)), nil
}

func (t *ViewImageTool) readImage(path string) (string, []byte, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil, fmt.Errorf("path is required")
	}
	candidates := candidateImagePaths(path)
	var lastErr error
	for _, candidate := range candidates {
		data, err := t.sb.ReadFileBytes(candidate)
		if err == nil {
			return candidate, data, nil
		}
		lastErr = err
	}
	return "", nil, lastErr
}

func candidateImagePaths(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	candidates := []string{path}
	if !strings.HasPrefix(path, "/") {
		candidates = append(candidates, challengeRelativeCandidates(path, "/challenge/distfiles", "/workspace", "/challenge/workspace")...)
	}
	return candidates
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
	// TIFF
	if data[0] == 0x49 && data[1] == 0x49 && data[2] == 0x2A && data[3] == 0x00 {
		return "image/tiff"
	}
	if data[0] == 0x4D && data[1] == 0x4D && data[2] == 0x00 && data[3] == 0x2A {
		return "image/tiff"
	}
	return ""
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
