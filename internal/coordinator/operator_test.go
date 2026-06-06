package coordinator

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleOperatorUploadCreatesManualChallenge(t *testing.T) {
	root := t.TempDir()
	coord := New(root, []string{"anthropic/test"}, nil, "ctf-sandbox", 1)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	mustWriteField(t, writer, "name", "Baby Upload")
	mustWriteField(t, writer, "category", "crypto")
	mustWriteField(t, writer, "description", "uploaded from operator UI")
	part, err := writer.CreateFormFile("files", "given.txt")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write([]byte("hello ctf\n")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	coord.handleOperatorUpload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	challengeDir := filepath.Join(root, "baby-upload")
	if _, err := os.Stat(filepath.Join(challengeDir, "metadata.yml")); err != nil {
		t.Fatalf("metadata.yml not created: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(challengeDir, "distfiles", "given.txt"))
	if err != nil {
		t.Fatalf("uploaded file not copied: %v", err)
	}
	if string(got) != "hello ctf\n" {
		t.Fatalf("uploaded file content = %q", got)
	}
	meta, err := os.ReadFile(filepath.Join(challengeDir, "metadata.yml"))
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if !strings.Contains(string(meta), "category: crypto") {
		t.Fatalf("metadata did not preserve category:\n%s", string(meta))
	}
}

func TestHandleOperatorStatusIncludesLocalChallenges(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "baby-web"), 0755); err != nil {
		t.Fatalf("create challenge dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "baby-web", "metadata.yml"), []byte("name: baby-web\ncategory: web\n"), 0644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	coord := New(root, []string{"anthropic/test"}, nil, "ctf-sandbox", 1)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	coord.handleOperatorStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"local_challenges":["baby-web"]`) {
		t.Fatalf("status did not expose local challenge list: %s", rec.Body.String())
	}
}

func TestOperatorUIContainsOperatorControls(t *testing.T) {
	html := string(operatorUI)
	for _, want := range []string{
		`id="uploadForm"`,
		`fetch("/upload"`,
		`id="sendGuidance"`,
		`fetch(path`,
		`id="modelSummaries"`,
		`function currentSummaries`,
		`id="loadTrace"`,
		"fetch(`/trace?${params.toString()}",
		`function parseTraceLines`,
		`tool_result`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("operator UI missing %q", want)
		}
	}
}

func mustWriteField(t *testing.T, writer *multipart.Writer, key, value string) {
	t.Helper()
	if err := writer.WriteField(key, value); err != nil {
		t.Fatalf("WriteField(%s): %v", key, err)
	}
}
