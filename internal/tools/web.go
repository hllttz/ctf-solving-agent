package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

var httpClient = &http.Client{
	Timeout:       15 * time.Second,
	CheckRedirect: checkWebFetchRedirect,
}

var blockedCIDRs = []string{
	"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
	"169.254.0.0/16", "::1/128", "fc00::/7", "fe80::/10",
}

func isBlocked(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		addrs, err := net.LookupIP(host)
		if err != nil || len(addrs) == 0 {
			return false
		}
		return hasBlockedIP(addrs)
	}
	return isBlockedIP(ip)
}

func hasBlockedIP(ips []net.IP) bool {
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return true
		}
	}
	return false
}

func isBlockedIP(ip net.IP) bool {
	for _, cidr := range blockedCIDRs {
		_, block, _ := net.ParseCIDR(cidr)
		if block != nil && block.Contains(ip) {
			return true
		}
	}
	return false
}

func checkWebFetchRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return fmt.Errorf("too many redirects")
	}
	if req != nil && req.URL != nil {
		host := req.URL.Hostname()
		if isBlocked(host) {
			return fmt.Errorf("blocked redirect: %s is an internal/private address", host)
		}
	}
	return nil
}

// WebFetchTool performs HTTP GET requests.
type WebFetchTool struct{}

func NewWebFetchTool() *WebFetchTool { return &WebFetchTool{} }

func (t *WebFetchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "web_fetch",
		Desc: "Fetch a URL. Supports method/body. Internal/private IPs are blocked for security.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"url":    {Type: schema.String, Desc: "URL to fetch", Required: true},
			"method": {Type: schema.String, Desc: "HTTP method (default GET)", Required: false},
			"body":   {Type: schema.String, Desc: "Optional request body", Required: false},
		}),
	}, nil
}

func (t *WebFetchTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		URL    string `json:"url"`
		Method string `json:"method"`
		Body   string `json:"body"`
	}
	if err := unmarshalArgs(argsJSON, &args); err != nil {
		return "", fmt.Errorf("web_fetch: %w", err)
	}

	u := args.URL
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = "http://" + u
	}

	parsed, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	host := parsed.Hostname()

	if isBlocked(host) {
		return "", fmt.Errorf("blocked: %s is an internal/private address", host)
	}

	method := strings.ToUpper(strings.TrimSpace(args.Method))
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequest(method, u, strings.NewReader(args.Body))
	if err != nil {
		return "", fmt.Errorf("request error: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	if args.Body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch error: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 500*1024))
	const maxLen = 24000
	if len(body) > maxLen {
		body = body[:maxLen]
	}
	return fmt.Sprintf("HTTP %d %s\n\n%s", resp.StatusCode, resp.Status, string(body)), nil
}

// WebhookCreateTool creates a webhook.site token for XSS/SSRF callbacks.
type WebhookCreateTool struct{}

func NewWebhookCreateTool() *WebhookCreateTool { return &WebhookCreateTool{} }

func (t *WebhookCreateTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name:        "webhook_create",
		Desc:        "Create a webhook.site token for capturing HTTP callbacks (XSS, SSRF).",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{}),
	}, nil
}

func (t *WebhookCreateTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	resp, err := httpClient.Post("https://webhook.site/token", "application/json", nil)
	if err != nil {
		return "", fmt.Errorf("webhook_create: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		UUID string `json:"uuid"`
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err := unmarshalArgs(string(body), &result); err != nil {
		return "", fmt.Errorf("webhook_create parse: %w (body: %s)", err, string(body))
	}

	return fmt.Sprintf("Webhook created: https://webhook.site/%s\nUse webhook_get_requests with token=%s to retrieve captured requests.",
		result.UUID, result.UUID), nil
}

// WebhookGetRequestsTool retrieves captured webhook requests.
type WebhookGetRequestsTool struct{}

func NewWebhookGetRequestsTool() *WebhookGetRequestsTool { return &WebhookGetRequestsTool{} }

func (t *WebhookGetRequestsTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "webhook_get_requests",
		Desc: "Retrieve captured requests for a webhook.site token.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"token": {Type: schema.String, Desc: "The webhook.site token/UUID", Required: true},
		}),
	}, nil
}

func (t *WebhookGetRequestsTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Token string `json:"token"`
	}
	if err := unmarshalArgs(argsJSON, &args); err != nil {
		return "", fmt.Errorf("webhook_get_requests: %w", err)
	}

	url := "https://webhook.site/token/" + args.Token + "/requests?sorting=newest"
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("webhook_get_requests: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 100*1024))
	const maxLen = 24000
	if len(body) > maxLen {
		body = body[:maxLen]
	}
	return string(body), nil
}
