package solver

import "testing"

func TestExtractFlagResultJSON(t *testing.T) {
	flag, method := extractFlagResult(`{"type":"flag_found","flag":"CTF{real_flag}","method":"decoded xor"}`)
	if flag != "CTF{real_flag}" {
		t.Fatalf("flag = %q", flag)
	}
	if method != "decoded xor" {
		t.Fatalf("method = %q", method)
	}
}

func TestExtractFlagResultFencedJSON(t *testing.T) {
	flag, method := extractFlagResult("done\n```json\n{\"type\":\"flag_found\",\"flag\":\"flag{inside}\",\"method\":\"web leak\"}\n```")
	if flag != "flag{inside}" {
		t.Fatalf("flag = %q", flag)
	}
	if method != "web leak" {
		t.Fatalf("method = %q", method)
	}
}

func TestExtractFlagResultFlagLine(t *testing.T) {
	flag, method := extractFlagResult("analysis mentions CTF{example}\nFLAG: CTF{actual}")
	if flag != "CTF{actual}" {
		t.Fatalf("flag = %q", flag)
	}
	if method != "FLAG line" {
		t.Fatalf("method = %q", method)
	}
}

func TestExtractFlagResultPatternFallback(t *testing.T) {
	flag, method := extractFlagResult("final answer is hsctf{legacy}")
	if flag != "hsctf{legacy}" {
		t.Fatalf("flag = %q", flag)
	}
	if method != "flag pattern fallback" {
		t.Fatalf("method = %q", method)
	}
}

func TestExtractFlagResultIgnoresNonFlagJSONType(t *testing.T) {
	flag, method := extractFlagResult(`{"type":"candidate","flag":"CTF{maybe}","method":"guess"}`)
	if flag != "" || method != "" {
		t.Fatalf("flag=%q method=%q", flag, method)
	}
}

func TestExtractFlagResultRejectsEscapedBytesFlagLine(t *testing.T) {
	flag, method := extractFlagResult(`FLAG: #)\x1e$8\x0e\x15 7\x0e\x05 \x00\x0e7\x12\x1d\x0f$\x01\x019`)
	if flag != "" || method != "" {
		t.Fatalf("flag=%q method=%q", flag, method)
	}
}

func TestExtractFlagResultRejectsEscapedBytesJSON(t *testing.T) {
	flag, method := extractFlagResult(`{"type":"flag_found","flag":"#)\\x1e$8\\x0e\\x15 7\\x0e\\x05 \\x00\\x0e7\\x12\\x1d\\x0f$\\x01\\x019","method":"read blob"}`)
	if flag != "" || method != "" {
		t.Fatalf("flag=%q method=%q", flag, method)
	}
}
