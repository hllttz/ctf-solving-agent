package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

var (
	flagShapePattern  = regexp.MustCompile(`(?i)^[a-z0-9_.-]{2,32}\{[!-~]{1,256}\}$`)
	byteEscapePattern = regexp.MustCompile(`(?i)\\(?:x[0-9a-f]{2}|u00[0-9a-f]{2})`)
)

// ReportedFlag is a solver's structured local flag report.
type ReportedFlag struct {
	Flag       string `json:"flag"`
	Method     string `json:"method,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	Evidence   string `json:"evidence,omitempty"`
}

// FlagReporter stores the latest structured flag report from a solver.
type FlagReporter struct {
	mu     sync.Mutex
	report *ReportedFlag
}

func NewFlagReporter() *FlagReporter {
	return &FlagReporter{}
}

func IsPlausibleFlag(flag string) bool {
	return flagRejectionReason(flag) == ""
}

func flagRejectionReason(flag string) string {
	flag = strings.TrimSpace(flag)
	if flag == "" {
		return "flag was empty"
	}
	if byteEscapePattern.MatchString(flag) {
		return "value contains escaped byte sequences. Do not report raw bytes or encoded blobs; decode or derive the final prefix{...} flag first."
	}
	for _, r := range flag {
		if r < 0x20 || r == 0x7f {
			return "value contains control characters. Do not report raw bytes; decode or derive the final printable prefix{...} flag first."
		}
	}
	if hasWhitespaceInsideBraces(flag) {
		return "value contains whitespace inside braces. OCR often inserts spaces or confuses spaces with underscores; visually verify the text and try confirmed variants such as replacing the whitespace with '_' before reporting."
	}
	if !flagShapePattern.MatchString(flag) {
		return "value does not look like a complete printable CTF flag. Do not report encoded blobs, placeholders, or decoys; decode or derive the final prefix{...} flag first."
	}
	return ""
}

func hasWhitespaceInsideBraces(flag string) bool {
	start := strings.IndexByte(flag, '{')
	end := strings.LastIndexByte(flag, '}')
	if start == -1 || end <= start {
		return false
	}
	for _, r := range flag[start+1 : end] {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return true
		}
	}
	return false
}

func (r *FlagReporter) Report(flag, method, confidence, evidence string) ReportedFlag {
	r.mu.Lock()
	defer r.mu.Unlock()

	report := ReportedFlag{
		Flag:       strings.TrimSpace(flag),
		Method:     strings.TrimSpace(method),
		Confidence: strings.TrimSpace(confidence),
		Evidence:   strings.TrimSpace(evidence),
	}
	r.report = &report
	return report
}

func (r *FlagReporter) Latest() (ReportedFlag, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.report == nil {
		return ReportedFlag{}, false
	}
	return *r.report, true
}

// ReportFlagTool lets a solver finish with a structured local flag report.
type ReportFlagTool struct {
	reporter *FlagReporter
}

func NewReportFlagTool(reporter *FlagReporter) *ReportFlagTool {
	return &ReportFlagTool{reporter: reporter}
}

func (t *ReportFlagTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "report_flag",
		Desc: "Report the final local flag once you have high confidence. This does not submit anywhere.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"flag":       {Type: schema.String, Desc: "The exact flag value", Required: true},
			"method":     {Type: schema.String, Desc: "Brief reproducible method used to find the flag", Required: false},
			"confidence": {Type: schema.String, Desc: "Confidence level such as high, medium, or low", Required: false},
			"evidence":   {Type: schema.String, Desc: "Short evidence or reproduction command showing why this is the real flag", Required: false},
		}),
	}, nil
}

func (t *ReportFlagTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Flag       string `json:"flag"`
		Method     string `json:"method"`
		Confidence string `json:"confidence"`
		Evidence   string `json:"evidence"`
	}
	if err := unmarshalArgs(argsJSON, &args); err != nil {
		return "", fmt.Errorf("report_flag: %w", err)
	}
	if strings.TrimSpace(args.Flag) == "" {
		return "No flag reported: flag was empty.", nil
	}
	if reason := flagRejectionReason(args.Flag); reason != "" {
		return "Flag report rejected: " + reason, nil
	}
	report := t.reporter.Report(args.Flag, args.Method, args.Confidence, args.Evidence)
	b, err := json.Marshal(report)
	if err != nil {
		return "", fmt.Errorf("report_flag: marshal: %w", err)
	}
	return "Flag report recorded locally: " + string(b), nil
}
