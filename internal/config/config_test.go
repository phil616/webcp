package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeyInjectionAndValidation(t *testing.T) {
	t.Setenv("WEBCP_ACCESS_KEY_FILE", "")
	t.Setenv("WEBCP_ACCESS_KEY", "")
	if _, err := Load(""); err == nil {
		t.Fatal("missing key accepted")
	}
	t.Setenv("WEBCP_ACCESS_KEY", "webcp-environment-access-key")
	if value, err := Load(""); err != nil || value.AccessKey != "webcp-environment-access-key" {
		t.Fatal("environment injection failed", err)
	}
	keyFile := filepath.Join(t.TempDir(), "access-key")
	if err := os.WriteFile(keyFile, []byte("webcp-file-access-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBCP_ACCESS_KEY_FILE", keyFile)
	if _, err := Load(""); err == nil {
		t.Fatal("ambiguous secret injection accepted")
	}
	os.Unsetenv("WEBCP_ACCESS_KEY")
	if value, err := Load(""); err != nil || value.AccessKey != "webcp-file-access-key" {
		t.Fatal("secret file injection failed", err)
	}
	t.Setenv("WEBCP_ACCESS_KEY_FILE", "")
	settings := filepath.Join(t.TempDir(), "webcp.json")
	os.WriteFile(settings, []byte(`{"accessKey":"webcp-config-access-key","container":"custom-openresty"}`), 0600)
	if value, err := Load(settings); err != nil || value.Container != "custom-openresty" {
		t.Fatal("configuration failed", err)
	}
	os.WriteFile(settings, []byte(`{"accessKey":"webcp-config-access-key","unknown":true}`), 0600)
	if _, err := Load(settings); err == nil || strings.Contains(err.Error(), "webcp-config-access-key") {
		t.Fatal("unsafe configuration error")
	}
}

func TestConfigRejectsOversizeAndNonObjectInput(t *testing.T) {
	t.Setenv("WEBCP_ACCESS_KEY", "webcp-release-config-test-key")
	t.Setenv("WEBCP_ACCESS_KEY_FILE", "")
	file := filepath.Join(t.TempDir(), "webcp.json")
	for _, input := range []string{"null", "[]", "{}" + strings.Repeat(" ", maxConfigSize) + "true"} {
		if err := os.WriteFile(file, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(file); err == nil {
			t.Fatal("invalid or truncated configuration accepted")
		}
	}
}
