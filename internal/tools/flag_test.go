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
