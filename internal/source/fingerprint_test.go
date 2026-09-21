package source

import (
	"strings"
	"testing"
)

func TestToolCallFingerprintIsDeterministic(t *testing.T) {
	tests := []struct {
		name      string
		toolName  string
		arguments string
		equalTo   [2]string
	}{
		{
			name:      "identical inputs",
			toolName:  "exec_command",
			arguments: `{"cmd":"ls -la","timeout":5}`,
			equalTo:   [2]string{`{"cmd":"ls -la","timeout":5}`, `{"cmd":"ls -la","timeout":5}`},
		},
		{
			name:      "object key order",
			toolName:  "exec_command",
			arguments: `{"cmd":"ls -la","timeout":5}`,
			equalTo:   [2]string{`{"cmd":"ls -la","timeout":5}`, `{"timeout":5,"cmd":"ls -la"}`},
		},
		{
			name:      "nested key order",
			toolName:  "exec_command",
			arguments: `{"a":{"x":1,"y":2},"b":[1,2]}`,
			equalTo:   [2]string{`{"a":{"x":1,"y":2},"b":[1,2]}`, `{"b":[1,2],"a":{"y":2,"x":1}}`},
		},
		{
			name:      "surrounding whitespace",
			toolName:  "exec_command",
			arguments: `{"cmd":"ls"}`,
			equalTo:   [2]string{"\n\t {\"cmd\":\"ls\"} \n", `{"cmd":"ls"}`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first, err := ToolCallFingerprint(tt.toolName, tt.equalTo[0])
			if err != nil {
				t.Fatalf("fingerprint: %v", err)
			}
			second, err := ToolCallFingerprint(tt.toolName, tt.equalTo[1])
			if err != nil {
				t.Fatalf("fingerprint: %v", err)
			}
			if first != second {
				t.Fatalf("fingerprints differ: %q vs %q", first, second)
			}
			if !strings.HasPrefix(first, "v1:") {
				t.Fatalf("fingerprint = %q, want a versioned value", first)
			}
		})
	}
}

func TestToolCallFingerprintTrimsTheToolName(t *testing.T) {
	trimmed := mustFingerprint(t, "exec_command", `{"cmd":"ls"}`)
	if padded := mustFingerprint(t, "  exec_command\n", `{"cmd":"ls"}`); padded != trimmed {
		t.Fatal("surrounding whitespace in the tool name must not change the fingerprint")
	}
}

func TestToolCallFingerprintSeparatesDifferentCalls(t *testing.T) {
	tests := []struct {
		name      string
		leftName  string
		leftArgs  string
		rightName string
		rightArgs string
	}{
		{
			name:     "different tool name",
			leftName: "exec_command", leftArgs: `{"cmd":"ls -la"}`,
			rightName: "read_file", rightArgs: `{"cmd":"ls -la"}`,
		},
		{
			name:     "tool name case",
			leftName: "exec_command", leftArgs: `{"cmd":"ls -la"}`,
			rightName: "EXEC_COMMAND", rightArgs: `{"cmd":"ls -la"}`,
		},
		{
			name:     "different argument value",
			leftName: "exec_command", leftArgs: `{"cmd":"ls -la"}`,
			rightName: "exec_command", rightArgs: `{"cmd":"ls"}`,
		},
		{
			name:     "argument added",
			leftName: "exec_command", leftArgs: `{"cmd":"ls -la"}`,
			rightName: "exec_command", rightArgs: `{"cmd":"ls -la","timeout":5}`,
		},
		{
			name:     "array order",
			leftName: "exec_command", leftArgs: `["a","b"]`,
			rightName: "exec_command", rightArgs: `["b","a"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := mustFingerprint(t, tt.leftName, tt.leftArgs)
			right := mustFingerprint(t, tt.rightName, tt.rightArgs)
			if left == right {
				t.Fatalf("fingerprints must differ, both are %q", left)
			}
		})
	}
}

// A float conversion would round these two integers onto the same value, so a
// stable fingerprint has to keep them apart.
func TestToolCallFingerprintKeepsNumbersExact(t *testing.T) {
	exact := mustFingerprint(t, "exec_command", `{"timeout":9007199254740993}`)
	neighbour := mustFingerprint(t, "exec_command", `{"timeout":9007199254740992}`)
	if exact == neighbour {
		t.Fatal("integers that only differ in the last digit must differ")
	}
	if !strings.HasPrefix(exact, "v1:") {
		t.Fatalf("fingerprint = %q, want a versioned value", exact)
	}
}

func TestToolCallFingerprintRejectsUnusableArguments(t *testing.T) {
	tests := []struct {
		name      string
		toolName  string
		arguments string
	}{
		{name: "empty tool name", toolName: "", arguments: `{"cmd":"ls"}`},
		{name: "blank tool name", toolName: "   ", arguments: `{"cmd":"ls"}`},
		{name: "malformed JSON", toolName: "exec_command", arguments: `{"cmd":`},
		{name: "empty arguments", toolName: "exec_command", arguments: ""},
		{name: "trailing JSON value", toolName: "exec_command", arguments: `{"cmd":"ls"} {"cmd":"pwd"}`},
		{name: "trailing garbage", toolName: "exec_command", arguments: `{"cmd":"ls"} x`},
		{name: "null arguments", toolName: "exec_command", arguments: `null`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ToolCallFingerprint(tt.toolName, tt.arguments); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestToolCallFingerprintErrorOmitsArguments(t *testing.T) {
	const secret = "SECRET_ARGUMENT_LITERAL_DO_NOT_RETAIN"
	_, err := ToolCallFingerprint("exec_command", `{"cmd":"`+secret)
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error exposes the source arguments: %v", err)
	}
}

func TestToolCallFingerprintOrEmpty(t *testing.T) {
	direct := mustFingerprint(t, "exec_command", `{"cmd":"ls"}`)
	tests := []struct {
		name      string
		toolName  string
		arguments string
		want      string
	}{
		{name: "inline JSON", toolName: "exec_command", arguments: `{"cmd":"ls"}`, want: direct},
		{name: "nested JSON string", toolName: "exec_command", arguments: `"{\"cmd\":\"ls\"}"`, want: direct},
		{name: "absent arguments", toolName: "exec_command", arguments: "", want: ""},
		{name: "null arguments", toolName: "exec_command", arguments: "null", want: ""},
		{name: "freeform arguments", toolName: "apply_patch", arguments: `*** Begin Patch`, want: ""},
		{name: "absent tool name", toolName: "", arguments: `{"cmd":"ls"}`, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToolCallFingerprintOrEmpty(tt.toolName, []byte(tt.arguments)); got != tt.want {
				t.Fatalf("fingerprint = %q, want %q", got, tt.want)
			}
		})
	}
}

func mustFingerprint(t *testing.T, toolName, arguments string) string {
	t.Helper()
	fingerprint, err := ToolCallFingerprint(toolName, arguments)
	if err != nil {
		t.Fatalf("fingerprint %q: %v", toolName, err)
	}
	return fingerprint
}
