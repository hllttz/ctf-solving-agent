package prompt

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Meta holds challenge metadata parsed from metadata.yml.
type Meta struct {
	Name           string   `yaml:"name"`
	Category       string   `yaml:"category"`
	Description    string   `yaml:"description"`
	Points         int      `yaml:"points"`
	Value          int      `yaml:"value"`
	Tags           []string `yaml:"tags"`
	Arch           string   `yaml:"arch"`
	Host           string   `yaml:"host"`
	Port           int      `yaml:"port"`
	ServiceType    string   `yaml:"service_type"`
	ConnectionInfo string   `yaml:"connection_info"`
	Files          []string `yaml:"files"`
	Hints          []Hint   `yaml:"hints"`
}

type Hint struct {
	Text    string `yaml:"text"`
	Content string `yaml:"content"`
	Cost    int    `yaml:"cost"`
}

// LoadMeta reads and parses a challenge metadata.yml file.
func LoadMeta(path string) (*Meta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	var meta Meta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}
	return &meta, nil
}

// Build generates the system prompt for a challenge solver.
func Build(meta *Meta, distfilesPath, workspacePath string) string {
	var b strings.Builder

	// Header
	b.WriteString("You are an autonomous CTF (Capture The Flag) solving agent.\n")
	b.WriteString("Your goal is to find the FLAG for the challenge described below.\n\n")

	if hasConnection(meta) {
		conn := connectionCommand(meta)
		b.WriteString("> FIRST ACTION REQUIRED: Your very first tool call MUST connect to the service.\n")
		b.WriteString(fmt.Sprintf("> Run: `%s`\n", conn))
		b.WriteString("> Do NOT explore the sandbox filesystem first. The flag is on the service, not in the container.\n\n")
	}

	// Challenge info
	b.WriteString("## Challenge\n")
	b.WriteString(fmt.Sprintf("Name: %s\n", meta.Name))
	b.WriteString(fmt.Sprintf("Category: %s\n", meta.Category))
	if points := metaPoints(meta); points > 0 {
		b.WriteString(fmt.Sprintf("Points: %d\n", points))
	}
	if len(meta.Tags) > 0 {
		b.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(meta.Tags, ", ")))
	}
	if meta.Arch != "" {
		b.WriteString(fmt.Sprintf("Architecture: %s\n", meta.Arch))
	}
	b.WriteString("\n")

	// Description
	if meta.Description != "" {
		b.WriteString("## Description\n")
		b.WriteString(meta.Description + "\n\n")
	}

	// Connection info
	if hasConnection(meta) {
		b.WriteString("## Connection\n")
		if meta.Host != "" && meta.Port > 0 {
			b.WriteString(fmt.Sprintf("Host: %s\n", meta.Host))
			b.WriteString(fmt.Sprintf("Port: %d\n", meta.Port))
		}
		b.WriteString(fmt.Sprintf("Use from sandbox: %s\n", connectionCommand(meta)))
		// Rewrite localhost -> host.docker.internal for Docker
		if meta.Host == "localhost" || meta.Host == "127.0.0.1" {
			b.WriteString("(Note: use host.docker.internal instead of localhost in the sandbox)\n")
		}
		b.WriteString(connectionGuidance(meta))
		b.WriteString("\n")
	}

	// Distfiles
	if len(meta.Files) > 0 {
		b.WriteString("## Provided Files (in /challenge/distfiles/)\n")
		for _, f := range meta.Files {
			b.WriteString(fmt.Sprintf("- %s\n", f))
			if hint := fileHint(f); hint != "" {
				b.WriteString(fmt.Sprintf("  Hint: %s\n", hint))
			}
		}
		b.WriteString("\n")
	}

	// Hints
	if len(meta.Hints) > 0 {
		b.WriteString("## Hints\n")
		for _, h := range meta.Hints {
			text := h.Text
			if text == "" {
				text = h.Content
			}
			if text != "" {
				b.WriteString(fmt.Sprintf("- %s\n", text))
			}
		}
		b.WriteString("\n")
	}

	// Category-specific guidance
	b.WriteString(categoryGuidance(meta.Category))
	b.WriteString(generalAnalysisGuidance(meta))

	// Instructions
	b.WriteString("## Instructions\n")
	if hasConnection(meta) {
		b.WriteString("1. Connect to the service now.\n")
	} else {
		b.WriteString("1. Inspect distfiles now.\n")
	}
	b.WriteString("2. Use the bash tool to run commands in the sandbox.\n")
	b.WriteString("3. Use the available tools (bash, read_file, write_file, list_files, view_image, web_fetch, webhook_create, webhook_get_requests, report_flag, post_finding, notify_coordinator, check_findings).\n")
	b.WriteString("4. Be persistent and creative. Try multiple approaches.\n")
	b.WriteString("5. The sandbox has extensive CTF tools installed - check /tools.txt for reference.\n")
	b.WriteString("6. Ignore placeholder flags like CTF{flag} or CTF{placeholder}; report only the real challenge flag.\n")
	b.WriteString("7. When you find the real printable flag in prefix{...} form, call report_flag with the exact flag, brief method, confidence, and evidence/reproduction command. This records the result locally and does not submit anywhere. Do not report raw bytes, escaped byte strings, encoded blobs, placeholders, or decoys as flags; decode or derive the final flag first.\n")
	b.WriteString("8. Before each tool call, briefly state your investigation intent in your own words: what you are trying to learn or verify, and why this is the next useful step. Keep it concise and technical.\n")
	b.WriteString("9. After important results, briefly update what changed in your understanding before continuing.\n")
	b.WriteString("10. If report_flag is impossible, end with JSON on its own line: {\"type\":\"flag_found\",\"flag\":\"<flag>\",\"method\":\"<brief method>\"}\n")
	b.WriteString("11. If JSON is impossible, output `FLAG: <value>` on its own line.\n")

	return b.String()
}

func connectionCommand(meta *Meta) string {
	if strings.TrimSpace(meta.ConnectionInfo) != "" {
		return rewriteLocalhost(strings.TrimSpace(meta.ConnectionInfo))
	}
	host := meta.Host
	if host == "localhost" || host == "127.0.0.1" {
		host = "host.docker.internal"
	}
	if meta.ServiceType == "web" {
		return fmt.Sprintf("curl http://%s:%d/", host, meta.Port)
	}
	return fmt.Sprintf("nc %s %d", host, meta.Port)
}

func hasConnection(meta *Meta) bool {
	return strings.TrimSpace(meta.ConnectionInfo) != "" || (meta.Host != "" && meta.Port > 0)
}

func metaPoints(meta *Meta) int {
	if meta.Points > 0 {
		return meta.Points
	}
	return meta.Value
}

func rewriteLocalhost(s string) string {
	s = strings.ReplaceAll(s, "localhost", "host.docker.internal")
	return strings.ReplaceAll(s, "127.0.0.1", "host.docker.internal")
}

func fileHint(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".jpg") ||
		strings.HasSuffix(lower, ".jpeg") || strings.HasSuffix(lower, ".gif") ||
		strings.HasSuffix(lower, ".bmp"):
		return "Image file - use view_image to inspect"
	case strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".md"):
		return "Text file - use read_file to inspect"
	case strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".tar.gz") ||
		strings.HasSuffix(lower, ".tgz"):
		return "Archive - use bash 'unzip' or 'tar xzf' to extract"
	case strings.HasSuffix(lower, ".pcap") || strings.HasSuffix(lower, ".pcapng"):
		return "Packet capture - use tshark or wireshark (tshark) to analyze"
	case strings.HasSuffix(lower, ".py") || strings.HasSuffix(lower, ".pyc"):
		return "Python file - use read_file or decompyle3 to inspect"
	default:
		return ""
	}
}

func connectionGuidance(meta *Meta) string {
	conn := connectionCommand(meta)
	if isWebConnection(meta) {
		return `This is a web service. Start with:
- bash: curl -i ` + webURL(meta) + `
- web_fetch for simple GET/POST requests when no cookies/session are needed.
- Prefer bash+curl or Python requests for cookies, sessions, redirects, and repeated fuzzing.
`
	}
	return `This is a TCP service. Each bash call is a fresh process. For multi-line interaction, use a heredoc:
` + conn + ` <<'EOF'
command1
command2
EOF
For stateful interaction, write a Python socket or pwntools script in /workspace.
`
}

func connURL(meta *Meta) string {
	host := meta.Host
	if host == "localhost" || host == "127.0.0.1" {
		host = "host.docker.internal"
	}
	return fmt.Sprintf("http://%s:%d", host, meta.Port)
}

func isWebConnection(meta *Meta) bool {
	conn := strings.TrimSpace(meta.ConnectionInfo)
	return meta.ServiceType == "web" || strings.HasPrefix(conn, "http://") || strings.HasPrefix(conn, "https://")
}

func webURL(meta *Meta) string {
	if strings.TrimSpace(meta.ConnectionInfo) != "" {
		return connectionCommand(meta)
	}
	return connURL(meta) + "/"
}

func generalAnalysisGuidance(meta *Meta) string {
	var b strings.Builder
	if hasImage(meta.Files) {
		b.WriteString(`## Image Guidance
- Call view_image first for image files.
- If view_image reports corruption, inspect and repair magic bytes with xxd/file before retrying.
- Then use exiftool, strings, binwalk, zsteg, steghide/stegseek as appropriate.

`)
	}
	if shouldIncludeBinaryGuidance(meta) {
		b.WriteString(`## Binary Analysis Tools
- pyghidra is available for decompilation. Example:
  python3 - <<'PY'
  import pyghidra
  with pyghidra.open_program('/challenge/distfiles/binary') as flat_api:
      program = flat_api.currentProgram
      print(program.getListing())
  PY
- Also try file, checksec, strings, objdump, r2, gdb, angr, capstone.

`)
	}
	return b.String()
}

func hasImage(files []string) bool {
	for _, name := range files {
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".jpg") ||
			strings.HasSuffix(lower, ".jpeg") || strings.HasSuffix(lower, ".gif") ||
			strings.HasSuffix(lower, ".bmp") || strings.HasSuffix(lower, ".webp") ||
			strings.HasSuffix(lower, ".tiff") || strings.HasSuffix(lower, ".tif") {
			return true
		}
	}
	return false
}

func shouldIncludeBinaryGuidance(meta *Meta) bool {
	cat := strings.ToLower(meta.Category)
	if cat == "" || strings.Contains(cat, "pwn") || strings.Contains(cat, "rev") ||
		strings.Contains(cat, "reverse") || strings.Contains(cat, "binary") || strings.Contains(cat, "misc") {
		return true
	}
	for _, name := range meta.Files {
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".elf") || strings.HasSuffix(lower, ".so") ||
			strings.HasSuffix(lower, ".exe") || strings.HasSuffix(lower, ".bin") {
			return true
		}
	}
	return false
}

func categoryGuidance(category string) string {
	switch strings.ToLower(category) {
	case "pwn", "binary exploitation":
		return `## Binary Exploitation Guidance
- Use file, checksec to analyze the binary
- Use radare2 or gdb for reverse engineering
- Use pwntools for exploit development
- Common techniques: buffer overflow, ROP, format string, heap exploitation
- The binary may have mitigations (NX, ASLR, PIE, Canary, RELRO)

`
	case "rev", "reverse engineering":
		return `## Reverse Engineering Guidance
- Use file, strings, objdump for initial analysis
- Use radare2, ghidra (pyghidra), or gdb for deep analysis
- Use angr for symbolic execution
- Look for encoded/encrypted strings and constants
- Python .pyc files: use decompyle3 or pycdc

`
	case "crypto", "cryptography":
		return `## Cryptography Guidance
- Use RsaCtfTool for RSA challenges
- Use SageMath for mathematical operations
- Use CADO-NFS for factorization
- Use flatter for LLL lattice reduction
- Look for common attacks: padding oracle, weak primes, etc.

`
	case "forensics", "forensic":
		return `## Forensics Guidance
- Use binwalk, foremost for file carving
- Use steghide, stegseek, zsteg for steganography
- Use exiftool for metadata analysis
- Use volatility3 for memory forensics
- Use tesseract for OCR

`
	case "web", "web exploitation":
		return `## Web Exploitation Guidance
- Use curl for HTTP requests
- Look for XSS, SQLi, SSRF, path traversal
- Use webhook_create + webhook_get_requests for callbacks
- Check source code, cookies, headers, and robots.txt
- Test for command injection and file inclusion

`
	case "misc", "miscellaneous":
		return `## Miscellaneous Guidance
- Think creatively - misc challenges often combine multiple techniques
- Inspect all files thoroughly
- Look for hidden data in files, encodings, and protocols

`
	default:
		return ""
	}
}
