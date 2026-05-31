package cost

import "testing"

func TestDeepSeekPricing(t *testing.T) {
	usage := Usage{
		Model:        "deepseek-v4-flash",
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}
	if got := usage.Cost(); got != 1.37 {
		t.Fatalf("cost = %.2f", got)
	}
}
