package handlers

import (
	"strings"
	"testing"
)

func TestValidateDomain_Valid(t *testing.T) {
	cases := []string{
		"example.com",
		"sub.example.com",
		"my-site.example.co.uk",
	}
	for _, c := range cases {
		if !validateDomain(c) {
			t.Errorf("expected %q to be valid", c)
		}
	}
}

func TestValidateDomain_Invalid(t *testing.T) {
	cases := []string{
		"",
		"-example.com",
		strings.Repeat("a", 254),
	}
	for _, c := range cases {
		if validateDomain(c) {
			t.Errorf("expected %q to be invalid", c)
		}
	}
}

func TestValidatePort_Valid(t *testing.T) {
	cases := []int{1024, 8080, 65535}
	for _, p := range cases {
		if !validatePort(p) {
			t.Errorf("expected port %d to be valid", p)
		}
	}
}

func TestValidatePort_Invalid(t *testing.T) {
	cases := []int{0, 80, 1023, 65536}
	for _, p := range cases {
		if validatePort(p) {
			t.Errorf("expected port %d to be invalid", p)
		}
	}
}

func TestValidateEmail_Valid(t *testing.T) {
	cases := []string{"user@example.com", "test.user+tag@sub.domain.com", ""}
	for _, c := range cases {
		if !validateEmail(c) {
			t.Errorf("expected %q to be valid", c)
		}
	}
}

func TestValidateEmail_Invalid(t *testing.T) {
	cases := []string{"notanemail", "@example.com", "user@"}
	for _, c := range cases {
		if validateEmail(c) {
			t.Errorf("expected %q to be invalid", c)
		}
	}
}

func TestValidatePhone_Valid(t *testing.T) {
	cases := []string{"+1 (555) 123-4567", "555-1234", ""}
	for _, c := range cases {
		if !validatePhone(c) {
			t.Errorf("expected %q to be valid", c)
		}
	}
}

func TestValidatePhone_Invalid(t *testing.T) {
	cases := []string{"abc", "12345"}
	for _, c := range cases {
		if validatePhone(c) {
			t.Errorf("expected %q to be invalid", c)
		}
	}
}

func TestValidateNotes(t *testing.T) {
	if !validateNotes("short note") {
		t.Error("short notes should be valid")
	}
	if !validateNotes("") {
		t.Error("empty notes should be valid")
	}
	if validateNotes(strings.Repeat("x", 1001)) {
		t.Error("notes over 1000 chars should be invalid")
	}
}

func TestSanitizeLogInput(t *testing.T) {
	if got := sanitizeLogInput("hello\nworld\r"); got != "helloworld" {
		t.Errorf("expected %q, got %q", "helloworld", got)
	}
}
