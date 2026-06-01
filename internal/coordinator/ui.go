package coordinator

import (
	_ "embed"
	"net/http"
)

//go:embed ui/index.html
var operatorUI []byte

func (c *Coordinator) handleOperatorUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(operatorUI)
}
