package docker

import (
	"strings"
	"testing"
)

// --- ValidateContainerName ---

func TestValidateContainerName_ValidNames(t *testing.T) {
	cases := []string{
		"mysite",
		"my-site",
		"my_site",
		"my.site",
		"site123",
		"a",
		"WordPress-01",
		"node.app_v2",
		"abc-def.ghi_123",
	}
	for _, name := range cases {
		if err := ValidateContainerName(name); err != nil {
			t.Errorf("ValidateContainerName(%q) returned unexpected error: %v", name, err)
		}
	}
}

func TestValidateContainerName_EmptyName(t *testing.T) {
	if err := ValidateContainerName(""); err == nil {
		t.Error("ValidateContainerName(\"\") should return an error for an empty name")
	}
}

func TestValidateContainerName_InvalidCharacters(t *testing.T) {
	cases := []string{
		"my site",    // space
		"my;site",   // semicolon
		"my`site",   // backtick
		"my$site",   // dollar sign
		"my&site",   // ampersand
		"my|site",   // pipe
		"my>site",   // redirect
		"my<site",   // redirect
		"my!site",   // exclamation
		"my(site)",  // parentheses
		"my*site",   // glob
		"my\"site",  // double quote
		"my'site",   // single quote
		"my\\site",  // backslash
		"my/site",   // forward slash
		"my@site",   // at sign
		"my#site",   // hash
		"my%site",   // percent
		"my^site",   // caret
		"my=site",   // equals
		"my+site",   // plus
		"my[site]",  // brackets
		"my{site}",  // braces
		"my~site",   // tilde
	}
	for _, name := range cases {
		if err := ValidateContainerName(name); err == nil {
			t.Errorf("ValidateContainerName(%q) should have returned an error", name)
		}
	}
}

func TestValidateContainerName_StartsWithNonAlphanumeric(t *testing.T) {
	cases := []string{
		"-mysite",
		"_mysite",
		".mysite",
	}
	for _, name := range cases {
		if err := ValidateContainerName(name); err == nil {
			t.Errorf("ValidateContainerName(%q) should have returned an error for non-alphanumeric start", name)
		}
	}
}

// --- RenderCompose ---

func TestRenderCompose_WordpressTemplate(t *testing.T) {
	vars := ComposeVars{
		ContainerName:  "testsite",
		Port:           8080,
		Domain:         "testsite.example.com",
		DBPassword:     "dbpass123",
		DBRootPassword: "rootpass456",
	}

	out, err := RenderCompose("wordpress", vars)
	if err != nil {
		t.Fatalf("RenderCompose(\"wordpress\") returned unexpected error: %v", err)
	}

	checks := []string{
		"testsite",
		"8080",
		"dbpass123",
		"rootpass456",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("RenderCompose output missing expected value %q", want)
		}
	}
}

func TestRenderCompose_StaticTemplate(t *testing.T) {
	vars := ComposeVars{
		ContainerName: "staticapp",
		Port:          9000,
		Domain:        "staticapp.example.com",
	}

	out, err := RenderCompose("static", vars)
	if err != nil {
		t.Fatalf("RenderCompose(\"static\") returned unexpected error: %v", err)
	}

	if !strings.Contains(out, "staticapp") {
		t.Error("RenderCompose output missing container name")
	}
	if !strings.Contains(out, "9000") {
		t.Error("RenderCompose output missing port")
	}
}

func TestRenderCompose_UnknownSlugReturnsError(t *testing.T) {
	vars := ComposeVars{
		ContainerName: "test",
		Port:          8080,
	}

	_, err := RenderCompose("nonexistent-template", vars)
	if err == nil {
		t.Error("RenderCompose with unknown slug should return an error")
	}
}
