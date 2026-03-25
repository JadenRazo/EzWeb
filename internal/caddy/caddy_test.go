package caddy

import (
	"testing"
)

func TestSanitizeDomain_StripsInjectionChars(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"exam\nple.com", "example.com"},
		{"exam\rple.com", "example.com"},
		{"exam`ple.com", "example.com"},
		{"exam{ple}.com", "example.com"},
		{"  example.com  ", "example.com"},
	}
	for _, tc := range cases {
		got := sanitizeDomain(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeDomain(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSanitizeDomain_NormalPassthrough(t *testing.T) {
	cases := []string{
		"example.com",
		"sub.example.com",
		"my-site.example.co.uk",
	}
	for _, c := range cases {
		got := sanitizeDomain(c)
		if got != c {
			t.Errorf("sanitizeDomain(%q) = %q, want unchanged", c, got)
		}
	}
}

func TestValidateExtraDirective_AcceptsClean(t *testing.T) {
	cases := []string{
		"header X-Frame-Options DENY",
		"gzip",
		"encode zstd gzip",
	}
	for _, d := range cases {
		if err := validateExtraDirective(d); err != nil {
			t.Errorf("validateExtraDirective(%q) returned unexpected error: %v", d, err)
		}
	}
}

func TestValidateExtraDirective_RejectsNewlines(t *testing.T) {
	cases := []string{
		"header X-Foo Bar\nmalicious block {",
		"gzip\r\nmalicious",
	}
	for _, d := range cases {
		if err := validateExtraDirective(d); err == nil {
			t.Errorf("validateExtraDirective(%q) should have returned an error", d)
		}
	}
}

func TestValidateExtraDirective_RejectsBraces(t *testing.T) {
	cases := []string{
		"header X-Foo { injected }",
		"respond { status 200 }",
	}
	for _, d := range cases {
		if err := validateExtraDirective(d); err == nil {
			t.Errorf("validateExtraDirective(%q) should have returned an error", d)
		}
	}
}
