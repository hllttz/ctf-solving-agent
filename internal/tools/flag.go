package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
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
	report := t.reporter.Report(args.Flag, args.Method, args.Confidence, args.Evidence)
	b, err := json.Marshal(report)
	if err != nil {
		return "", fmt.Errorf("report_flag: marshal: %w", err)
	}
	return "Flag report recorded locally: " + string(b), nil
}
