package source

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestStructuredToolCallFingerprintIsDeterministic(t *testing.T) {
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
			first, err := StructuredToolCallFingerprint(tt.toolName, tt.equalTo[0])
			if err != nil {
				t.Fatalf("fingerprint: %v", err)
			}
			second, err := StructuredToolCallFingerprint(tt.toolName, tt.equalTo[1])
			if err != nil {
				t.Fatalf("fingerprint: %v", err)
			}
			if first != second {
				t.Fatalf("fingerprints differ: %q vs %q", first, second)
			}
			if !strings.HasPrefix(first, structuredToolCallFingerprintPrefix) {
				t.Fatalf("fingerprint = %q, want a versioned value", first)
			}
		})
	}
}

func TestStructuredToolCallFingerprintTrimsTheToolName(t *testing.T) {
	trimmed := mustStructuredFingerprint(t, "exec_command", `{"cmd":"ls"}`)
	if padded := mustStructuredFingerprint(t, "  exec_command\n", `{"cmd":"ls"}`); padded != trimmed {
		t.Fatal("surrounding whitespace in the tool name must not change the fingerprint")
	}
}

func TestStructuredToolCallFingerprintSeparatesDifferentCalls(t *testing.T) {
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
			left := mustStructuredFingerprint(t, tt.leftName, tt.leftArgs)
			right := mustStructuredFingerprint(t, tt.rightName, tt.rightArgs)
			if left == right {
				t.Fatalf("fingerprints must differ, both are %q", left)
			}
		})
	}
}

// A float conversion would round these two integers onto the same value, so a
// stable fingerprint has to keep them apart.
func TestStructuredToolCallFingerprintKeepsNumbersExact(t *testing.T) {
	exact := mustStructuredFingerprint(t, "exec_command", `{"timeout":9007199254740993}`)
	neighbour := mustStructuredFingerprint(t, "exec_command", `{"timeout":9007199254740992}`)
	if exact == neighbour {
		t.Fatal("integers that only differ in the last digit must differ")
	}
	if !strings.HasPrefix(exact, structuredToolCallFingerprintPrefix) {
		t.Fatalf("fingerprint = %q, want a versioned value", exact)
	}
}

func TestStructuredToolCallFingerprintRejectsUnusableArguments(t *testing.T) {
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
			if _, err := StructuredToolCallFingerprint(tt.toolName, tt.arguments); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestStructuredToolCallFingerprintErrorOmitsArguments(t *testing.T) {
	const secret = "SECRET_ARGUMENT_LITERAL_DO_NOT_RETAIN"
	_, err := StructuredToolCallFingerprint("exec_command", `{"cmd":"`+secret)
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error exposes the source arguments: %v", err)
	}
}

func TestStructuredToolCallFingerprintOrEmpty(t *testing.T) {
	direct := mustStructuredFingerprint(t, "exec_command", `{"cmd":"ls"}`)
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
			if got := StructuredToolCallFingerprintOrEmpty(tt.toolName, []byte(tt.arguments)); got != tt.want {
				t.Fatalf("fingerprint = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFreeformToolCallFingerprintIsDeterministic(t *testing.T) {
	const input = "const r = await tools.exec_command({\"cmd\":\"ls\"}); text(r.output);"
	first := mustFreeformFingerprint(t, "exec", input)
	second := mustFreeformFingerprint(t, "exec", input)
	if first != second {
		t.Fatalf("fingerprints differ: %q vs %q", first, second)
	}
	if !strings.HasPrefix(first, freeformToolCallFingerprintPrefix) {
		t.Fatalf("fingerprint = %q, want a versioned freeform value", first)
	}
}

func TestFreeformToolCallFingerprintSeparatesDifferentCalls(t *testing.T) {
	tests := []struct {
		name       string
		leftName   string
		leftInput  string
		rightName  string
		rightInput string
	}{
		{
			name:     "different tool name",
			leftName: "exec", leftInput: "echo hi",
			rightName: "apply_patch", rightInput: "echo hi",
		},
		{
			name:     "tool name case",
			leftName: "exec", leftInput: "echo hi",
			rightName: "EXEC", rightInput: "echo hi",
		},
		{
			name:     "different input",
			leftName: "exec", leftInput: "echo hi",
			rightName: "exec", rightInput: "echo bye",
		},
		{
			name:     "input prefix",
			leftName: "exec", leftInput: "echo hi",
			rightName: "exec", rightInput: "echo hi there",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := mustFreeformFingerprint(t, tt.leftName, tt.leftInput)
			right := mustFreeformFingerprint(t, tt.rightName, tt.rightInput)
			if left == right {
				t.Fatalf("fingerprints must differ, both are %q", left)
			}
		})
	}
}

// Freeform input has no declared structure, so V1 hashes it as written. Only
// trimming the tool name is allowed.
func TestFreeformToolCallFingerprintKeepsInputWhitespaceSignificant(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
	}{
		{name: "surrounding whitespace", left: "echo hi", right: " echo hi "},
		{name: "trailing newline", left: "echo hi", right: "echo hi\n"},
		{name: "inner whitespace", left: "a b", right: "a  b"},
		{name: "tab against space", left: "a\tb", right: "a b"},
		{name: "blank against empty line", left: "a\nb", right: "a\n\nb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := mustFreeformFingerprint(t, "exec", tt.left)
			right := mustFreeformFingerprint(t, "exec", tt.right)
			if left == right {
				t.Fatalf("inputs %q and %q must not share a fingerprint", tt.left, tt.right)
			}
		})
	}
}

func TestFreeformToolCallFingerprintTrimsTheToolName(t *testing.T) {
	trimmed := mustFreeformFingerprint(t, "exec", "echo hi")
	if padded := mustFreeformFingerprint(t, "  exec\n", "echo hi"); padded != trimmed {
		t.Fatal("surrounding whitespace in the tool name must not change the fingerprint")
	}
}

func TestFreeformToolCallFingerprintRejectsEmptyInput(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		input    string
	}{
		{name: "empty input", toolName: "exec", input: ""},
		{name: "empty tool name", toolName: "", input: "echo hi"},
		{name: "blank tool name", toolName: "   ", input: "echo hi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fingerprint, err := FreeformToolCallFingerprint(tt.toolName, tt.input)
			if err == nil {
				t.Fatal("want an error")
			}
			if fingerprint != "" {
				t.Fatalf("fingerprint = %q, want no usable value", fingerprint)
			}
		})
	}
}

func TestFreeformToolCallFingerprintOrEmpty(t *testing.T) {
	direct := mustFreeformFingerprint(t, "exec", "echo hi")
	tests := []struct {
		name     string
		toolName string
		input    string
		want     string
	}{
		{name: "freeform input", toolName: "exec", input: "echo hi", want: direct},
		{name: "empty input", toolName: "exec", input: "", want: ""},
		{name: "absent tool name", toolName: "", input: "echo hi", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FreeformToolCallFingerprintOrEmpty(tt.toolName, tt.input); got != tt.want {
				t.Fatalf("fingerprint = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFreeformToolCallFingerprintOmitsInput(t *testing.T) {
	const secret = "SECRET_FREEFORM_LITERAL_DO_NOT_RETAIN"
	fingerprint := mustFreeformFingerprint(t, "exec", "echo "+secret)
	if strings.Contains(fingerprint, secret) || strings.Contains(fingerprint, "echo") {
		t.Fatalf("fingerprint = %q, want an opaque digest", fingerprint)
	}
	if !isFingerprintDigest(fingerprint, freeformToolCallFingerprintPrefix) {
		t.Fatalf("fingerprint = %q, want a hex digest", fingerprint)
	}

	_, err := FreeformToolCallFingerprint("", "echo "+secret)
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error exposes the freeform input: %v", err)
	}
}

// The two domains describe different kinds of argument. Text that looks like
// JSON is still freeform text when the source declared it as freeform, so the
// same visible characters must never fingerprint equally across domains.
func TestStructuredAndFreeformFingerprintsDoNotCollide(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "object", text: `{"cmd":"ls"}`},
		{name: "object key order", text: `{"timeout":5,"cmd":"ls"}`},
		{name: "array", text: `["a","b"]`},
		{name: "number", text: `5`},
		{name: "string literal", text: `"ls"`},
		{name: "boolean literal", text: `true`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			structured := mustStructuredFingerprint(t, "exec", tt.text)
			freeform := mustFreeformFingerprint(t, "exec", tt.text)
			if structured == freeform {
				t.Fatalf("domains collided on %q: %q", tt.text, structured)
			}
			if !isFingerprintDigest(freeform, freeformToolCallFingerprintPrefix) {
				t.Fatalf("freeform fingerprint = %q, want the freeform domain", freeform)
			}
			if strings.HasPrefix(freeform, structuredToolCallFingerprintPrefix) {
				t.Fatalf("freeform fingerprint = %q, want a domain-qualified value", freeform)
			}
		})
	}
}

func mustStructuredFingerprint(t *testing.T, toolName, arguments string) string {
	t.Helper()
	fingerprint, err := StructuredToolCallFingerprint(toolName, arguments)
	if err != nil {
		t.Fatalf("fingerprint %q: %v", toolName, err)
	}
	return fingerprint
}

func mustFreeformFingerprint(t *testing.T, toolName, input string) string {
	t.Helper()
	fingerprint, err := FreeformToolCallFingerprint(toolName, input)
	if err != nil {
		t.Fatalf("fingerprint %q: %v", toolName, err)
	}
	return fingerprint
}

func isFingerprintDigest(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	digest := value[len(prefix):]
	if len(digest) != 64 {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}
