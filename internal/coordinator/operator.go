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
	mux.HandleFunc("/broadcast", c.handleOperatorBroadcast)
	mux.HandleFunc("/kill", c.handleOperatorKill)
	mux.HandleFunc("/bump", c.handleOperatorBump)

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
		active[name] = sw.Status()
	}
	c.mu.Unlock()

	writeJSON(w, map[string]any{
		"summary":           c.Summary(),
		"active_challenges": active,
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
