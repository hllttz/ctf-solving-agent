package tools

import (
	"context"
	"strings"
	"testing"
)

func TestReportFlagToolStoresLatestReport(t *testing.T) {
	reporter := NewFlagReporter()
	tool := NewReportFlagTool(reporter)

	out, err := tool.InvokableRun(context.Background(), `{"flag":" CTF{real} ","method":"xor decode","confidence":"high","evidence":"python solve.py prints it"}`)
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}
	if !strings.Contains(out, `"flag":"CTF{real}"`) {
		t.Fatalf("output = %q", out)
	}

	report, ok := reporter.Latest()
	if !ok {
		t.Fatalf("missing latest report")
	}
	if report.Flag != "CTF{real}" {
		t.Fatalf("flag = %q", report.Flag)
	}
	if report.Method != "xor decode" {
		t.Fatalf("method = %q", report.Method)
	}
	if report.Confidence != "high" {
		t.Fatalf("confidence = %q", report.Confidence)
	}
	if report.Evidence != "python solve.py prints it" {
		t.Fatalf("evidence = %q", report.Evidence)
	}
}

func TestReportFlagToolIgnoresEmptyFlag(t *testing.T) {
	reporter := NewFlagReporter()
	tool := NewReportFlagTool(reporter)

	out, err := tool.InvokableRun(context.Background(), `{"flag":"   "}`)
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}
	if !strings.Contains(out, "flag was empty") {
		t.Fatalf("output = %q", out)
	}
	if _, ok := reporter.Latest(); ok {
		t.Fatalf("unexpected report")
	}
}

func TestReportFlagToolRejectsEscapedBytes(t *testing.T) {
	reporter := NewFlagReporter()
	tool := NewReportFlagTool(reporter)

	out, err := tool.InvokableRun(context.Background(), `{"flag":"#)\\x1e$8\\x0e\\x15 7\\x0e\\x05 \\x00\\x0e7\\x12\\x1d\\x0f$\\x01\\x019","method":"read bundled blob"}`)
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}
	if !strings.Contains(out, "rejected") {
		t.Fatalf("output = %q", out)
	}
	if _, ok := reporter.Latest(); ok {
		t.Fatalf("unexpected report")
	}
}

func TestReportFlagToolRejectsControlBytes(t *testing.T) {
	reporter := NewFlagReporter()
	tool := NewReportFlagTool(reporter)

	out, err := tool.InvokableRun(context.Background(), `{"flag":"CTF{bad\u0000flag}"}`)
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}
	if !strings.Contains(out, "rejected") {
		t.Fatalf("output = %q", out)
	}
	if _, ok := reporter.Latest(); ok {
		t.Fatalf("unexpected report")
	}
}

func TestReportFlagToolExplainsWhitespaceInsideBraces(t *testing.T) {
	reporter := NewFlagReporter()
	tool := NewReportFlagTool(reporter)

	out, err := tool.InvokableRun(context.Background(), `{"flag":"DASCTF{good_yOu_get_the _ffffflag!}","method":"ocr"}`)
	if err != nil {
		t.Fatalf("InvokableRun error: %v", err)
	}
	for _, want := range []string{"rejected", "whitespace inside braces", "OCR", "underscores"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
	if _, ok := reporter.Latest(); ok {
		t.Fatalf("unexpected report")
	}
}
