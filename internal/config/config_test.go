package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsDataConfigAndEnvironmentOverrides(t *testing.T) {
	dir := t.TempDir()
	content := `FMLYSYS_ADMIN_USERNAME=file-admin
FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD="pass # with spaces"
FMLYSYS_WECHAT_APP_ID=file-app
FMLYSYS_WECHAT_APP_SECRET=file-secret
FMLYSYS_DEV_AUTH_ENABLED=true
`
	if err := os.WriteFile(filepath.Join(dir, LocalConfigFilename), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	unsetForTest(t,
		"FMLYSYS_ADMIN_USERNAME",
		"FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD",
		"FMLYSYS_WECHAT_APP_ID",
		"FMLYSYS_WECHAT_APP_SECRET",
		"FMLYSYS_DEV_AUTH_ENABLED",
		"FMLYSYS_PORT",
		"FMLYSYS_BIND_HOST",
	)
	t.Setenv("FMLYSYS_DATA_DIR", dir)
	t.Setenv("FMLYSYS_ADMIN_USERNAME", "env-admin")
	t.Setenv("FMLYSYS_PORT", "18080")
	t.Setenv("FMLYSYS_BIND_HOST", "127.0.0.1")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:18080" || cfg.Port != 18080 || cfg.BindHost != "127.0.0.1" {
		t.Fatalf("unexpected listener config: %+v", cfg)
	}
	if cfg.AdminUsername != "env-admin" {
		t.Fatalf("expected environment to override file, got %q", cfg.AdminUsername)
	}
	if cfg.AdminBootstrapPassword != "pass # with spaces" {
		t.Fatalf("unexpected password %q", cfg.AdminBootstrapPassword)
	}
	if cfg.WeChatAppID != "file-app" || cfg.WeChatAppSecret != "file-secret" {
		t.Fatalf("unexpected WeChat config: %+v", cfg)
	}
	if !cfg.WeChatConfigured() {
		t.Fatal("WeChat should be configured with AppID + AppSecret only")
	}
	if !cfg.DevAuthEnabled {
		t.Fatal("expected boolean value from data/config.env")
	}
}

func TestLoadRequiresPortEnvironment(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, LocalConfigFilename), []byte("FMLYSYS_PORT=9999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unsetForTest(t, "FMLYSYS_PORT")
	t.Setenv("FMLYSYS_DATA_DIR", dir)
	if _, err := Load(); err == nil {
		t.Fatal("expected missing FMLYSYS_PORT environment variable to fail startup")
	}
}

func TestLoadRequiresBindHostEnvironment(t *testing.T) {
	dir := t.TempDir()
	unsetForTest(t, "FMLYSYS_BIND_HOST")
	t.Setenv("FMLYSYS_DATA_DIR", dir)
	t.Setenv("FMLYSYS_PORT", "18080")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing FMLYSYS_BIND_HOST environment variable to fail startup")
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FMLYSYS_DATA_DIR", dir)
	t.Setenv("FMLYSYS_PORT", "70000")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid FMLYSYS_PORT to fail startup")
	}
}

func TestLoadRejectsMalformedConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, LocalConfigFilename), []byte("BROKEN_LINE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FMLYSYS_DATA_DIR", dir)
	t.Setenv("FMLYSYS_PORT", "18080")
	if _, err := Load(); err == nil {
		t.Fatal("expected malformed local config to fail startup")
	}
}

func unsetForTest(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		old, had := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		key, old, had := key, old, had
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(key, old)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
}
