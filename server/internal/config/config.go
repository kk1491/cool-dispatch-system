package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

// Config 统一承载服务启动所需的环境配置，避免各层直接读取环境变量。
type Config struct {
	// AppEnv 标识运行环境，用于切换 Gin 模式和静态资源行为。
	AppEnv string
	// Port 是 HTTP 服务监听端口。
	Port string
	// DatabaseURL 是 PostgreSQL 连接串。
	DatabaseURL string
	// FrontendOrigin 是允许跨域访问的前端来源地址。
	FrontendOrigin string
	// FrontendDist 是生产环境下前端构建产物目录。
	FrontendDist string
	// HTTPReadHeaderTimeoutSeconds 控制请求头读取超时，优先防止慢速头部攻击拖住连接。
	HTTPReadHeaderTimeoutSeconds int
	// HTTPReadTimeoutSeconds 控制整个请求读取超时，避免慢上传长期占用连接。
	HTTPReadTimeoutSeconds int
	// HTTPWriteTimeoutSeconds 控制响应写出超时，避免写阻塞无限挂起。
	HTTPWriteTimeoutSeconds int
	// HTTPIdleTimeoutSeconds 控制 keep-alive 空闲连接生命周期。
	HTTPIdleTimeoutSeconds int
	// HTTPMaxHeaderBytes 限制单请求头部总大小，降低超大 Header DoS 风险。
	HTTPMaxHeaderBytes int
	// MaxJSONBodyBytes 限制普通 JSON 写接口请求体大小，避免大包拖垮解析与内存。
	MaxJSONBodyBytes int64
	// MaxWebhookBodyBytes 限制公开 webhook 请求体大小，减少匿名大包攻击面。
	MaxWebhookBodyBytes int64
	// SeedAdminName 是演示数据初始化时默认管理员显示名称。
	SeedAdminName string
	// SeedAdminPhone 是演示数据初始化时默认管理员登录手机号。
	SeedAdminPhone string
	// SeedAdminPassword 只用于开发/演示数据初始化管理员账号密码，缺失时不会再回退到硬编码默认值。
	SeedAdminPassword string
	// SeedTechnicianPassword 只用于开发/演示数据初始化技师账号密码。
	SeedTechnicianPassword string
	// DevTechnicianName 是开发/调试默认师傅账号的显示名称，每次启动无条件同步。
	DevTechnicianName string
	// DevTechnicianPhone 是开发/调试默认师傅账号的登录手机号。
	DevTechnicianPhone string
	// DevTechnicianPassword 是开发/调试默认师傅账号的登录密码。
	DevTechnicianPassword string
	// LineChannelSecret 与 LINE Developers 后台 channel secret 对齐，供 webhook 签名校验使用。
	LineChannelSecret string
	// WebhookPublicBaseURL 用于生成管理员可见的 webhook 回调地址，优先使用显式公网域名配置。
	WebhookPublicBaseURL string
	// CookieSecure 控制认证 Cookie 是否只允许在 HTTPS 链路上传输。
	CookieSecure bool
	// CookieSameSite 控制认证 Cookie 的 SameSite 策略。
	CookieSameSite string
	// AutoMigrate 控制启动时是否自动执行 GORM 迁移。
	AutoMigrate bool
	// SeedDemoData 控制启动时是否初始化演示数据。
	SeedDemoData bool
	// EnableStatic 控制后端是否托管前端静态资源。
	EnableStatic bool
	// CloudflareAccountID 是 Cloudflare 账户 ID，用于 Images API 路径拼接。
	CloudflareAccountID string
	// CloudflareAPIToken 是 Cloudflare API Token，需具备 Images Write 权限，用于图床上传与删除。
	CloudflareAPIToken string
	// LogDir 是日志文件存放目录，相对于工作目录或绝对路径。
	LogDir string
}

// fileConfig 描述 config.yaml 中允许声明的配置项；使用指针区分“未配置”与零值。
type fileConfig struct {
	AppEnv                       *string `yaml:"app_env"`
	Port                         *string `yaml:"port"`
	DatabaseURL                  *string `yaml:"database_url"`
	FrontendOrigin               *string `yaml:"frontend_origin"`
	FrontendDist                 *string `yaml:"frontend_dist"`
	HTTPReadHeaderTimeoutSeconds *int    `yaml:"http_read_header_timeout_seconds"`
	HTTPReadTimeoutSeconds       *int    `yaml:"http_read_timeout_seconds"`
	HTTPWriteTimeoutSeconds      *int    `yaml:"http_write_timeout_seconds"`
	HTTPIdleTimeoutSeconds       *int    `yaml:"http_idle_timeout_seconds"`
	HTTPMaxHeaderBytes           *int    `yaml:"http_max_header_bytes"`
	MaxJSONBodyBytes             *int64  `yaml:"max_json_body_bytes"`
	MaxWebhookBodyBytes          *int64  `yaml:"max_webhook_body_bytes"`
	SeedAdminName                *string `yaml:"seed_admin_name"`
	SeedAdminPhone               *string `yaml:"seed_admin_phone"`
	SeedAdminPassword            *string `yaml:"seed_admin_password"`
	SeedTechnicianPassword       *string `yaml:"seed_technician_password"`
	DevTechnicianName            *string `yaml:"dev_technician_name"`
	DevTechnicianPhone           *string `yaml:"dev_technician_phone"`
	DevTechnicianPassword        *string `yaml:"dev_technician_password"`
	LineChannelSecret            *string `yaml:"line_channel_secret"`
	WebhookPublicBaseURL         *string `yaml:"webhook_public_base_url"`
	CookieSecure                 *bool   `yaml:"cookie_secure"`
	CookieSameSite               *string `yaml:"cookie_same_site"`
	AutoMigrate                  *bool   `yaml:"auto_migrate"`
	SeedDemoData                 *bool   `yaml:"seed_demo_data"`
	EnableStatic                 *bool   `yaml:"enable_static"`
	CloudflareAccountID          *string `yaml:"cloudflare_account_id"`
	CloudflareAPIToken           *string `yaml:"cloudflare_api_token"`
	LogDir                       *string `yaml:"log_dir"`
}

// Load 负责按“默认值 -> config.yaml -> 环境变量”的优先级加载服务配置。
func Load() Config {
	cfg := Config{
		AppEnv:                       "development",
		Port:                         "9102",
		DatabaseURL:                  "postgres://postgres:postgres@localhost:9101/cool_dispatch?sslmode=disable",
		FrontendOrigin:               "http://localhost:5173",
		FrontendDist:                 "../dist/client",
		HTTPReadHeaderTimeoutSeconds: 5,
		HTTPReadTimeoutSeconds:       15,
		HTTPWriteTimeoutSeconds:      30,
		HTTPIdleTimeoutSeconds:       60,
		HTTPMaxHeaderBytes:           1 << 20,
		MaxJSONBodyBytes:             1 << 20,
		MaxWebhookBodyBytes:          256 << 10,
		CookieSameSite:               "lax",
		LogDir:                       "logs",
	}

	if raw := strings.TrimSpace(os.Getenv("GIN_MODE")); raw != "" {
		cfg.AppEnv = raw
	}

	if fileCfg, ok := loadFileConfig(); ok {
		applyFileConfig(&cfg, fileCfg)
	}

	appEnv := getEnv("APP_ENV", cfg.AppEnv)
	cfg.AppEnv = getEnv("GIN_MODE", appEnv)
	cfg.Port = getEnv("PORT", cfg.Port)
	cfg.DatabaseURL = getEnv("DATABASE_URL", cfg.DatabaseURL)
	cfg.FrontendOrigin = getEnv("FRONTEND_ORIGIN", cfg.FrontendOrigin)
	cfg.FrontendDist = getEnv("FRONTEND_DIST", cfg.FrontendDist)
	cfg.HTTPReadHeaderTimeoutSeconds = getIntEnv("HTTP_READ_HEADER_TIMEOUT_SECONDS", cfg.HTTPReadHeaderTimeoutSeconds)
	cfg.HTTPReadTimeoutSeconds = getIntEnv("HTTP_READ_TIMEOUT_SECONDS", cfg.HTTPReadTimeoutSeconds)
	cfg.HTTPWriteTimeoutSeconds = getIntEnv("HTTP_WRITE_TIMEOUT_SECONDS", cfg.HTTPWriteTimeoutSeconds)
	cfg.HTTPIdleTimeoutSeconds = getIntEnv("HTTP_IDLE_TIMEOUT_SECONDS", cfg.HTTPIdleTimeoutSeconds)
	cfg.HTTPMaxHeaderBytes = getIntEnv("HTTP_MAX_HEADER_BYTES", cfg.HTTPMaxHeaderBytes)
	cfg.MaxJSONBodyBytes = getInt64Env("MAX_JSON_BODY_BYTES", cfg.MaxJSONBodyBytes)
	cfg.MaxWebhookBodyBytes = getInt64Env("MAX_WEBHOOK_BODY_BYTES", cfg.MaxWebhookBodyBytes)
	cfg.SeedAdminName = strings.TrimSpace(getEnv("SEED_ADMIN_NAME", cfg.SeedAdminName))
	cfg.SeedAdminPhone = strings.TrimSpace(getEnv("SEED_ADMIN_PHONE", cfg.SeedAdminPhone))
	cfg.SeedAdminPassword = strings.TrimSpace(getEnv("SEED_ADMIN_PASSWORD", cfg.SeedAdminPassword))
	cfg.SeedTechnicianPassword = strings.TrimSpace(getEnv("SEED_TECHNICIAN_PASSWORD", cfg.SeedTechnicianPassword))
	cfg.DevTechnicianName = strings.TrimSpace(getEnv("DEV_TECHNICIAN_NAME", cfg.DevTechnicianName))
	cfg.DevTechnicianPhone = strings.TrimSpace(getEnv("DEV_TECHNICIAN_PHONE", cfg.DevTechnicianPhone))
	cfg.DevTechnicianPassword = strings.TrimSpace(getEnv("DEV_TECHNICIAN_PASSWORD", cfg.DevTechnicianPassword))
	cfg.LineChannelSecret = getEnv("LINE_CHANNEL_SECRET", cfg.LineChannelSecret)
	cfg.WebhookPublicBaseURL = strings.TrimSpace(getEnv("WEBHOOK_PUBLIC_BASE_URL", getEnv("PUBLIC_BASE_URL", cfg.WebhookPublicBaseURL)))
	cfg.CookieSecure = getBoolEnv("COOKIE_SECURE", cfg.AppEnv == "production" || cfg.CookieSecure)
	cfg.CookieSameSite = getCookieSameSiteEnv("COOKIE_SAME_SITE", cfg.CookieSameSite)
	cfg.AutoMigrate = getBoolEnv("AUTO_MIGRATE", cfg.AutoMigrate)
	cfg.SeedDemoData = getBoolEnv("SEED_DEMO_DATA", cfg.SeedDemoData)
	cfg.EnableStatic = cfg.AppEnv == "production" || getBoolEnv("SERVE_STATIC", cfg.EnableStatic)
	cfg.CloudflareAccountID = strings.TrimSpace(getEnv("CLOUDFLARE_ACCOUNT_ID", cfg.CloudflareAccountID))
	cfg.CloudflareAPIToken = strings.TrimSpace(getEnv("CLOUDFLARE_API_TOKEN", cfg.CloudflareAPIToken))
	cfg.LogDir = strings.TrimSpace(getEnv("LOG_DIR", cfg.LogDir))

	return cfg
}

// loadFileConfig 按约定位置读取 config.yaml/config.yml；文件不存在时静默跳过。
func loadFileConfig() (fileConfig, bool) {
	for _, path := range configFileCandidates() {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg fileConfig
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			continue
		}
		return cfg, true
	}
	return fileConfig{}, false
}

// configFileCandidates 返回允许自动探测的配置文件路径，兼容从仓库根目录或 server/ 目录启动。
func configFileCandidates() []string {
	if explicit := strings.TrimSpace(os.Getenv("CONFIG_FILE")); explicit != "" {
		return []string{explicit}
	}
	return []string{
		"config.yaml",
		"config.yml",
		"../config.yaml",
		"../config.yml",
	}
}

// applyFileConfig 将 config.yaml 中声明的值覆盖到运行配置上。
func applyFileConfig(cfg *Config, fileCfg fileConfig) {
	if cfg == nil {
		return
	}
	if fileCfg.AppEnv != nil {
		cfg.AppEnv = strings.TrimSpace(*fileCfg.AppEnv)
		cfg.CookieSecure = cfg.AppEnv == "production"
		cfg.EnableStatic = cfg.AppEnv == "production"
	}

	applyString := func(target *string, value *string) {
		if value != nil {
			*target = strings.TrimSpace(*value)
		}
	}
	applyInt := func(target *int, value *int) {
		if value != nil {
			*target = *value
		}
	}
	applyInt64 := func(target *int64, value *int64) {
		if value != nil {
			*target = *value
		}
	}
	applyBool := func(target *bool, value *bool) {
		if value != nil {
			*target = *value
		}
	}

	applyString(&cfg.Port, fileCfg.Port)
	applyString(&cfg.DatabaseURL, fileCfg.DatabaseURL)
	applyString(&cfg.FrontendOrigin, fileCfg.FrontendOrigin)
	applyString(&cfg.FrontendDist, fileCfg.FrontendDist)
	applyInt(&cfg.HTTPReadHeaderTimeoutSeconds, fileCfg.HTTPReadHeaderTimeoutSeconds)
	applyInt(&cfg.HTTPReadTimeoutSeconds, fileCfg.HTTPReadTimeoutSeconds)
	applyInt(&cfg.HTTPWriteTimeoutSeconds, fileCfg.HTTPWriteTimeoutSeconds)
	applyInt(&cfg.HTTPIdleTimeoutSeconds, fileCfg.HTTPIdleTimeoutSeconds)
	applyInt(&cfg.HTTPMaxHeaderBytes, fileCfg.HTTPMaxHeaderBytes)
	applyInt64(&cfg.MaxJSONBodyBytes, fileCfg.MaxJSONBodyBytes)
	applyInt64(&cfg.MaxWebhookBodyBytes, fileCfg.MaxWebhookBodyBytes)
	applyString(&cfg.SeedAdminName, fileCfg.SeedAdminName)
	applyString(&cfg.SeedAdminPhone, fileCfg.SeedAdminPhone)
	applyString(&cfg.SeedAdminPassword, fileCfg.SeedAdminPassword)
	applyString(&cfg.SeedTechnicianPassword, fileCfg.SeedTechnicianPassword)
	applyString(&cfg.DevTechnicianName, fileCfg.DevTechnicianName)
	applyString(&cfg.DevTechnicianPhone, fileCfg.DevTechnicianPhone)
	applyString(&cfg.DevTechnicianPassword, fileCfg.DevTechnicianPassword)
	applyString(&cfg.LineChannelSecret, fileCfg.LineChannelSecret)
	applyString(&cfg.WebhookPublicBaseURL, fileCfg.WebhookPublicBaseURL)
	if fileCfg.CookieSameSite != nil {
		cfg.CookieSameSite = normalizeCookieSameSite(*fileCfg.CookieSameSite, cfg.CookieSameSite)
	}
	applyBool(&cfg.CookieSecure, fileCfg.CookieSecure)
	applyBool(&cfg.AutoMigrate, fileCfg.AutoMigrate)
	applyBool(&cfg.SeedDemoData, fileCfg.SeedDemoData)
	applyBool(&cfg.EnableStatic, fileCfg.EnableStatic)
	applyString(&cfg.CloudflareAccountID, fileCfg.CloudflareAccountID)
	applyString(&cfg.CloudflareAPIToken, fileCfg.CloudflareAPIToken)
	applyString(&cfg.LogDir, fileCfg.LogDir)
}

// getEnv 读取字符串环境变量，不存在时返回默认值。
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// getBoolEnv 读取布尔环境变量，解析失败时回退到默认值，避免启动阶段直接报错。
func getBoolEnv(key string, fallback bool) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

// getIntEnv 读取整型环境变量，解析失败时回退默认值，避免因配置错误直接导致服务无法启动。
func getIntEnv(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

// getInt64Env 读取 int64 环境变量，主要用于请求体上限等字节级配置。
func getInt64Env(key string, fallback int64) int64 {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

// getCookieSameSiteEnv 读取 Cookie SameSite 配置，非法值时回退安全默认值。
func getCookieSameSiteEnv(key string, fallback string) string {
	return normalizeCookieSameSite(os.Getenv(key), fallback)
}

// normalizeCookieSameSite 统一校验 Cookie SameSite 文本，非法值时回退到安全默认值。
func normalizeCookieSameSite(raw string, fallback string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return fallback
	}
	switch normalized {
	case "lax", "strict", "none":
		return normalized
	default:
		return fallback
	}
}
