package coordinator

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// StartOperatorServer starts a small local HTTP endpoint for broadcasting
// operator messages to active swarms.
func (c *Coordinator) StartOperatorServer(ctx context.Context, addr string) (string, error) {
	if strings.TrimSpace(addr) == "" {
		addr = "127.0.0.1:0"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/msg", c.handleOperatorMessage)
	mux.HandleFunc("/status", c.handleOperatorStatus)
	mux.HandleFunc("/notifications", c.handleOperatorNotifications)
	mux.HandleFunc("/trace", c.handleOperatorTrace)
	mux.HandleFunc("/artifacts", c.handleOperatorArtifacts)
	mux.HandleFunc("/ui", c.handleOperatorUI)
	mux.HandleFunc("/ui/", c.handleOperatorUI)
	mux.HandleFunc("/broadcast", c.handleOperatorBroadcast)
	mux.HandleFunc("/kill", c.handleOperatorKill)
	mux.HandleFunc("/bump", c.handleOperatorBump)
	mux.HandleFunc("/spawn", c.handleOperatorSpawn)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("listen operator server: %w", err)
	}

	srv := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	go func() {
		_ = srv.Serve(listener)
	}()

	return "http://" + listener.Addr().String(), nil
}

func (c *Coordinator) handleOperatorMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	message := strings.TrimSpace(body.Message)
	if message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	count := c.Broadcast(message)
	writeJSON(w, map[string]any{"ok": true, "broadcast_to": count})
}

func (c *Coordinator) handleOperatorStatus(w http.ResponseWriter, _ *http.Request) {
	c.mu.Lock()
	active := make(map[string]any, len(c.swarms))
	for name, sw := range c.swarms {
		status := sw.Status()
		if _, done := c.results[name]; done {
			status["active"] = false
		}
		active[name] = status
	}
	c.mu.Unlock()

	writeJSON(w, map[string]any{
		"summary":           c.Summary(),
		"active_challenges": active,
		"results":           c.Results(),
		"total_cost_usd":    c.TotalCost(),
		"usage":             c.CostSnapshot(),
	})
}

func (c *Coordinator) handleOperatorNotifications(w http.ResponseWriter, _ *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make(map[string][]string)
	for name, sw := range c.swarms {
		for _, item := range sw.Bus().All() {
			if item.Author == "coordinator_notification" {
				out[name] = append(out[name], item.Content)
			}
		}
	}
	writeJSON(w, map[string]any{"notifications": out})
}

func (c *Coordinator) handleOperatorTrace(w http.ResponseWriter, r *http.Request) {
	challenge := strings.TrimSpace(r.URL.Query().Get("challenge"))
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	lastN := 40
	if v := strings.TrimSpace(r.URL.Query().Get("last")); v != "" {
		fmt.Sscanf(v, "%d", &lastN)
	}
	lines, err := readRecentTrace("logs", challenge, model, lastN)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"lines": lines})
}

func (c *Coordinator) handleOperatorArtifacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	challenge := strings.TrimSpace(r.URL.Query().Get("challenge"))
	if challenge == "" {
		http.Error(w, "challenge is required", http.StatusBadRequest)
		return
	}
	workspace, err := safeWorkspacePath(c.challengesDir, challenge, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	if rel == "" {
		files, err := listArtifacts(workspace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{"files": files})
		return
	}

	path, err := safeWorkspacePath(c.challengesDir, challenge, rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if info.IsDir() {
		http.Error(w, "artifact is a directory", http.StatusBadRequest)
		return
	}
	const maxPreviewBytes = 256 * 1024
	if info.Size() > maxPreviewBytes {
		http.Error(w, "artifact is too large to preview", http.StatusRequestEntityTooLarge)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isProbablyBinary(data) {
		http.Error(w, "artifact appears to be binary", http.StatusUnsupportedMediaType)
		return
	}
	writeJSON(w, map[string]any{
		"path":    filepath.ToSlash(filepath.Clean(rel)),
		"size":    info.Size(),
		"content": string(data),
	})
}

func (c *Coordinator) handleOperatorBroadcast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Challenge string `json:"challenge"`
		Message   string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	message := strings.TrimSpace(body.Message)
	if message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}
	count := c.BroadcastTo(strings.TrimSpace(body.Challenge), message)
	writeJSON(w, map[string]any{"ok": true, "broadcast_to": count})
}

func (c *Coordinator) handleOperatorKill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Challenge string `json:"challenge"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if !c.Kill(strings.TrimSpace(body.Challenge)) {
		http.Error(w, "challenge not found", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (c *Coordinator) handleOperatorBump(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Challenge string `json:"challenge"`
		Model     string `json:"model"`
		Insights  string `json:"insights"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if !c.Bump(strings.TrimSpace(body.Challenge), strings.TrimSpace(body.Model), strings.TrimSpace(body.Insights)) {
		http.Error(w, "challenge/model not found", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (c *Coordinator) handleOperatorSpawn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Challenge string `json:"challenge"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	challenge := strings.TrimSpace(body.Challenge)
	if challenge == "" {
		http.Error(w, "challenge is required", http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(filepath.Join(c.challengesDir, challenge, "metadata.yml")); err != nil {
		http.Error(w, "local challenge metadata not found", http.StatusNotFound)
		return
	}
	if !c.Spawn(nil, challenge) {
		http.Error(w, "challenge already running or invalid", http.StatusConflict)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "challenge": challenge})
}

// Broadcast sends a coordinator message to all currently known swarms.
func (c *Coordinator) Broadcast(message string) int {
	return c.BroadcastTo("", message)
}

func (c *Coordinator) BroadcastTo(challenge, message string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for name, sw := range c.swarms {
		if challenge != "" && name != challenge {
			continue
		}
		sw.Bus().Broadcast(message)
		count++
	}
	return count
}

func (c *Coordinator) Kill(challenge string) bool {
	c.mu.Lock()
	sw := c.swarms[challenge]
	c.mu.Unlock()
	if sw == nil {
		return false
	}
	sw.Kill()
	return true
}

func (c *Coordinator) Bump(challenge, model, insights string) bool {
	c.mu.Lock()
	sw := c.swarms[challenge]
	c.mu.Unlock()
	if sw == nil {
		return false
	}
	return sw.Bump(model, insights)
}

func readRecentTrace(logDir, challenge, model string, lastN int) ([]string, error) {
	if lastN <= 0 {
		lastN = 40
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil, err
	}
	challenge = tracePart(challenge)
	model = tracePart(model)
	var newest string
	var newestInfo os.FileInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		name := entry.Name()
		if challenge != "" && !strings.Contains(name, challenge) {
			continue
		}
		if model != "" && !strings.Contains(name, model) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if newest == "" || info.ModTime().After(newestInfo.ModTime()) {
			newest = filepath.Join(logDir, name)
			newestInfo = info
		}
	}
	if newest == "" {
		return nil, fmt.Errorf("trace not found")
	}

	f, err := os.Open(newest)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > lastN {
			lines = lines[1:]
		}
	}
	return lines, scanner.Err()
}

func ReadRecentTrace(logDir, challenge, model string, lastN int) ([]string, error) {
	return readRecentTrace(logDir, challenge, model, lastN)
}

type artifactInfo struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}

func listArtifacts(workspace string) ([]artifactInfo, error) {
	if _, err := os.Stat(workspace); err != nil {
		return nil, err
	}
	var files []artifactInfo
	err := filepath.WalkDir(workspace, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == workspace {
			return nil
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files = append(files, artifactInfo{
			Path:  filepath.ToSlash(rel),
			Name:  entry.Name(),
			Size:  info.Size(),
			IsDir: entry.IsDir(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func safeWorkspacePath(challengesDir, challenge, rel string) (string, error) {
	challenge = filepath.Clean(strings.TrimSpace(challenge))
	if challenge == "." || challenge == "" || strings.Contains(challenge, string(filepath.Separator)) {
		return "", fmt.Errorf("invalid challenge")
	}
	base, err := filepath.Abs(filepath.Join(challengesDir, challenge, "workspace"))
	if err != nil {
		return "", err
	}
	if rel == "" {
		return base, nil
	}
	cleanRel := filepath.Clean(strings.TrimPrefix(rel, "/"))
	if cleanRel == "." {
		return base, nil
	}
	path, err := filepath.Abs(filepath.Join(base, cleanRel))
	if err != nil {
		return "", err
	}
	if path != base && !strings.HasPrefix(path, base+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid artifact path")
	}
	return path, nil
}

func isProbablyBinary(data []byte) bool {
	for i := 0; i < len(data) && i < 512; i++ {
		if data[i] == 0 || data[i] < 8 || (data[i] > 13 && data[i] < 32) {
			return true
		}
	}
	return false
}

func tracePart(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, ":", "_")
	return s
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
