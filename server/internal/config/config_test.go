package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadUsesDefaultsWhenEnvMissing 验证缺省环境变量时会回退到内置安全默认值。
func TestLoadUsesDefaultsWhenEnvMissing(t *testing.T) {
	keys := []string{
		"APP_ENV",
		"GIN_MODE",
		"PORT",
		"DATABASE_URL",
		"FRONTEND_ORIGIN",
		"FRONTEND_DIST",
		"HTTP_READ_HEADER_TIMEOUT_SECONDS",
		"HTTP_READ_TIMEOUT_SECONDS",
		"HTTP_WRITE_TIMEOUT_SECONDS",
		"HTTP_IDLE_TIMEOUT_SECONDS",
		"HTTP_MAX_HEADER_BYTES",
		"MAX_JSON_BODY_BYTES",
		"MAX_WEBHOOK_BODY_BYTES",
		"SEED_ADMIN_NAME",
		"SEED_ADMIN_PHONE",
		"SEED_ADMIN_PASSWORD",
		"SEED_TECHNICIAN_PASSWORD",
		"LINE_CHANNEL_SECRET",
		"WEBHOOK_PUBLIC_BASE_URL",
		"PUBLIC_BASE_URL",
		"COOKIE_SECURE",
		"COOKIE_SAME_SITE",
		"AUTO_MIGRATE",
		"SEED_DEMO_DATA",
		"SERVE_STATIC",
		"CONFIG_FILE",
	}
	for _, key := range keys {
		t.Setenv(key, "")
	}

	cfg := Load()
	if cfg.AppEnv != "development" {
		t.Fatalf("expected default app env, got %s", cfg.AppEnv)
	}
	if cfg.Port != "9102" {
		t.Fatalf("expected default port, got %s", cfg.Port)
	}
	if cfg.DatabaseURL == "" || cfg.FrontendOrigin == "" || cfg.FrontendDist == "" {
		t.Fatalf("expected defaults populated, got %+v", cfg)
	}
	if cfg.WebhookPublicBaseURL != "" {
		t.Fatalf("expected empty default webhook public base url, got %+v", cfg)
	}
	if cfg.CookieSecure {
		t.Fatalf("expected default cookie secure disabled in development, got %+v", cfg)
	}
	if cfg.CookieSameSite != "lax" {
		t.Fatalf("expected default cookie same-site lax, got %+v", cfg)
	}
	if cfg.HTTPReadHeaderTimeoutSeconds != 5 || cfg.HTTPReadTimeoutSeconds != 15 || cfg.HTTPWriteTimeoutSeconds != 30 || cfg.HTTPIdleTimeoutSeconds != 60 {
		t.Fatalf("expected default timeout values, got %+v", cfg)
	}
	if cfg.HTTPMaxHeaderBytes != 1<<20 || cfg.MaxJSONBodyBytes != 1<<20 || cfg.MaxWebhookBodyBytes != 256<<10 {
		t.Fatalf("expected default body/header limits, got %+v", cfg)
	}
	if cfg.AutoMigrate || cfg.SeedDemoData {
		t.Fatalf("expected default bools false, got %+v", cfg)
	}
	if cfg.EnableStatic {
		t.Fatalf("expected static serving disabled in default development mode")
	}
	if cfg.SeedAdminName != "" || cfg.SeedAdminPhone != "" || cfg.SeedAdminPassword != "" || cfg.SeedTechnicianPassword != "" {
		t.Fatalf("expected seed passwords empty by default, got %+v", cfg)
	}
}

// TestLoadRespectsStaticFlagsAndBoolFallback 验证显式环境变量覆盖和非法值回退逻辑。
func TestLoadRespectsStaticFlagsAndBoolFallback(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("AUTO_MIGRATE", "not-a-bool")
	t.Setenv("SEED_DEMO_DATA", "false")
	t.Setenv("SERVE_STATIC", "false")
	t.Setenv("PUBLIC_BASE_URL", "https://dispatch.example.com")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("COOKIE_SAME_SITE", "strict")
	t.Setenv("HTTP_READ_TIMEOUT_SECONDS", "not-a-number")
	t.Setenv("MAX_JSON_BODY_BYTES", "2048")

	cfg := Load()
	if cfg.AppEnv != "production" {
		t.Fatalf("expected production app env, got %s", cfg.AppEnv)
	}
	if cfg.AutoMigrate {
		t.Fatalf("expected invalid AUTO_MIGRATE to fall back to false")
	}
	if cfg.SeedDemoData {
		t.Fatalf("expected explicit false seed flag")
	}
	if cfg.HTTPReadTimeoutSeconds != 15 {
		t.Fatalf("expected invalid HTTP_READ_TIMEOUT_SECONDS to fall back to 15, got %+v", cfg)
	}
	if cfg.MaxJSONBodyBytes != 2048 {
		t.Fatalf("expected explicit MAX_JSON_BODY_BYTES override, got %+v", cfg)
	}
	if !cfg.EnableStatic {
		t.Fatalf("expected production to force static serving")
	}
	if cfg.WebhookPublicBaseURL != "https://dispatch.example.com" {
		t.Fatalf("expected PUBLIC_BASE_URL fallback, got %+v", cfg)
	}
	if !cfg.CookieSecure || cfg.CookieSameSite != "strict" {
		t.Fatalf("expected cookie settings loaded from env, got %+v", cfg)
	}

	t.Setenv("APP_ENV", "development")
	t.Setenv("SERVE_STATIC", "true")
	t.Setenv("WEBHOOK_PUBLIC_BASE_URL", "https://hooks.example.com")
	t.Setenv("SEED_ADMIN_NAME", "系統管理員")
	t.Setenv("SEED_ADMIN_PHONE", "0900111222")
	t.Setenv("SEED_ADMIN_PASSWORD", "admin-password-123")
	t.Setenv("SEED_TECHNICIAN_PASSWORD", "tech-password-123")
	t.Setenv("HTTP_MAX_HEADER_BYTES", "8192")
	t.Setenv("MAX_WEBHOOK_BODY_BYTES", "4096")
	t.Setenv("COOKIE_SAME_SITE", "not-valid")
	cfg = Load()
	if !cfg.EnableStatic {
		t.Fatalf("expected SERVE_STATIC=true to enable static serving in development")
	}
	if cfg.WebhookPublicBaseURL != "https://hooks.example.com" {
		t.Fatalf("expected explicit WEBHOOK_PUBLIC_BASE_URL override, got %+v", cfg)
	}
	if cfg.SeedAdminName != "系統管理員" || cfg.SeedAdminPhone != "0900111222" || cfg.SeedAdminPassword != "admin-password-123" || cfg.SeedTechnicianPassword != "tech-password-123" {
		t.Fatalf("expected seed passwords loaded from env, got %+v", cfg)
	}
	if cfg.HTTPMaxHeaderBytes != 8192 || cfg.MaxWebhookBodyBytes != 4096 {
		t.Fatalf("expected explicit header/body limits loaded, got %+v", cfg)
	}
	if cfg.CookieSameSite != "lax" {
		t.Fatalf("expected invalid COOKIE_SAME_SITE to fall back to lax, got %+v", cfg)
	}
}

// TestLoadReadsConfigYAML 验证 config.yaml 可作为默认配置来源，并允许环境变量继续覆盖。
func TestLoadReadsConfigYAML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	content := []byte(`
app_env: production
port: "9200"
database_url: postgres://demo:demo@localhost:5432/demo?sslmode=disable
frontend_origin: https://dispatch.example.com
seed_admin_name: 營運管理員
seed_admin_phone: "0999000111"
seed_admin_password: yaml-admin-password
seed_technician_password: yaml-tech-password
seed_demo_data: true
cookie_same_site: strict
`)
	if err := os.WriteFile(configPath, content, 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	t.Setenv("CONFIG_FILE", configPath)
	t.Setenv("PORT", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("GIN_MODE", "")
	t.Setenv("SEED_ADMIN_NAME", "")
	t.Setenv("SEED_ADMIN_PHONE", "")
	t.Setenv("SEED_ADMIN_PASSWORD", "")
	t.Setenv("SEED_TECHNICIAN_PASSWORD", "")

	cfg := Load()
	if cfg.AppEnv != "production" || cfg.Port != "9200" {
		t.Fatalf("expected app env and port from yaml, got %+v", cfg)
	}
	if cfg.SeedAdminName != "營運管理員" || cfg.SeedAdminPhone != "0999000111" {
		t.Fatalf("expected admin identity from yaml, got %+v", cfg)
	}
	if cfg.SeedAdminPassword != "yaml-admin-password" || cfg.SeedTechnicianPassword != "yaml-tech-password" {
		t.Fatalf("expected seed passwords from yaml, got %+v", cfg)
	}
	if !cfg.SeedDemoData || !cfg.CookieSecure || cfg.CookieSameSite != "strict" {
		t.Fatalf("expected yaml booleans and cookie settings applied, got %+v", cfg)
	}

	t.Setenv("SEED_ADMIN_PHONE", "0911222333")
	cfg = Load()
	if cfg.SeedAdminPhone != "0911222333" {
		t.Fatalf("expected env var to override yaml, got %+v", cfg)
	}
}
