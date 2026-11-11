package service

import (
	"strings"
	"testing"
)

func TestNewEmployeeCode(t *testing.T) {
	const (
		prefix     = "EMP-"
		HexDigits  = "0123456789ABCDEF"
		hexSuffixL = 10
	)

	t.Run("format", func(t *testing.T) {
		code := newEmployeeCode()
		if !strings.HasPrefix(code, prefix) {
			t.Fatalf("code %q does not strat with %q", code, prefix)
		}
		suffix := code[len(prefix):]
		if len(suffix) != hexSuffixL {
			t.Fatalf("code %q: suffix length = %d, want %d", code, len(suffix), hexSuffixL)
		}
		for _, r := range suffix {
			if !strings.ContainsRune(HexDigits, r) {
				t.Fatalf("code %q: %q is not an uppercase hex digit", code, r)
			}
		}
	})

	t.Run("unique", func(t *testing.T) {
		//define a set here
		seen := make(map[string]struct{})
		for range 100 {
			c := newEmployeeCode()
			if _, dup := seen[c]; dup {
				t.Fatalf("dupliacate code genearted %v", dup)
			}
			seen[c] = struct{}{}
		}
	})
}

func TestDefaultEmplymentType(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty defaults", in: "", want: "full_time"},
		{name: "whitespace defaults", in: "   ", want: "full_time"},
		{name: "explicit value kept", in: "part_time", want: "part_time"},
		{name: "another value kept", in: "contract", want: "contract"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultEmploymentType(tt.in); got != tt.want {
				t.Fatalf("defaultEmploymentType(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
