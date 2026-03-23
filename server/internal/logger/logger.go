// Package logger 封装应用日志系统，基于 zap + lumberjack 实现：
// - 普通日志（DEBUG/INFO/WARN）写入 app.log
// - 错误日志（ERROR/FATAL）写入 error.log
// - 每类文件最多保留 MaxBackups 个备份，单文件最大 MaxSize MB
// - 同时输出到 stdout/stderr，方便开发调试
// - 全局单例 + 便捷函数，业务代码零侵入调用
package logger

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// 全局日志实例，Init 后可用。
var (
	// L 是结构化日志实例，适合用 zap.String / zap.Int 等 Field。
	L *zap.Logger
	// S 是 SugaredLogger，支持 printf 风格格式化，更灵活但略慢。
	S *zap.SugaredLogger
)

// Config 日志系统配置。
type Config struct {
	// LogDir 是日志文件所在目录，相对于工作目录或绝对路径。
	LogDir string
	// MaxSizeMB 是单个日志文件的最大大小（MB），超过后自动轮转。
	MaxSizeMB int
	// MaxBackups 是每种日志文件保留的最大备份数量。
	MaxBackups int
}

// Init 根据配置初始化全局日志实例。
// 调用后即可通过 logger.Info / logger.Error 等便捷函数写日志。
// 应在程序启动时最早调用，确保后续所有模块都能使用日志。
func Init(cfg Config) {
	// 应用默认值
	if cfg.LogDir == "" {
		cfg.LogDir = "logs"
	}
	if cfg.MaxSizeMB <= 0 {
		cfg.MaxSizeMB = 5
	}
	if cfg.MaxBackups <= 0 {
		cfg.MaxBackups = 2
	}

	// 确保日志目录存在
	_ = os.MkdirAll(cfg.LogDir, 0755)

	// ---------- 日志文件轮转配置 ----------

	// 普通日志文件（app.log）：接收 DEBUG/INFO/WARN 级别
	infoRotator := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.LogDir, "app.log"),
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		LocalTime:  true, // 备份文件名使用本地时间
	}

	// 错误日志文件（error.log）：接收 ERROR/DPANIC/PANIC/FATAL 级别
	errorRotator := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.LogDir, "error.log"),
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		LocalTime:  true,
	}

	// ---------- 编码器配置 ----------
	// 使用控制台文本格式，每行一条日志，方便人工浏览。
	// 格式示例：2026-03-23T13:22:29.123+0800	INFO	httpapi/router.go:55	request completed	{"method": "GET", "path": "/api/appointments", "status": 200, "latency": "12ms"}
	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,                 // INFO / ERROR 大写
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000"), // 易读时间格式
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder, // 短路径 package/file.go:line
	}

	encoder := zapcore.NewConsoleEncoder(encoderCfg)

	// ---------- 按级别分流到不同文件 ----------

	// 普通级别过滤器：DEBUG <= level < ERROR
	infoLevel := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.DebugLevel && lvl < zapcore.ErrorLevel
	})

	// 错误级别过滤器：level >= ERROR
	errorLevel := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.ErrorLevel
	})

	// 构建多核心日志管道：
	// - 普通日志 → app.log + stdout
	// - 错误日志 → error.log + stderr（同时也写入 app.log 保证完整性）
	core := zapcore.NewTee(
		// 普通日志同时写文件和标准输出
		zapcore.NewCore(encoder, zapcore.NewMultiWriteSyncer(
			zapcore.AddSync(infoRotator),
			zapcore.AddSync(os.Stdout),
		), infoLevel),
		// 错误日志同时写文件和标准错误
		zapcore.NewCore(encoder, zapcore.NewMultiWriteSyncer(
			zapcore.AddSync(errorRotator),
			zapcore.AddSync(os.Stderr),
		), errorLevel),
	)

	L = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	S = L.Sugar()
}

// Sync 刷新缓冲区，确保所有日志写入磁盘。
// 应在程序退出前调用（通常放在 main 的 defer 中）。
func Sync() {
	if L != nil {
		_ = L.Sync()
	}
}

// ---------- 便捷全局函数 ----------
// 业务代码直接调用 logger.Info(...) 即可，无需持有 Logger 实例。

// Info 记录普通信息日志。
func Info(msg string, fields ...zap.Field) { L.Info(msg, fields...) }

// Warn 记录警告日志。
func Warn(msg string, fields ...zap.Field) { L.Warn(msg, fields...) }

// Error 记录错误日志（写入 error.log）。
func Error(msg string, fields ...zap.Field) { L.Error(msg, fields...) }

// Debug 记录调试日志。
func Debug(msg string, fields ...zap.Field) { L.Debug(msg, fields...) }

// Fatal 记录致命错误日志并终止程序。
func Fatal(msg string, fields ...zap.Field) { L.Fatal(msg, fields...) }

// Infof 使用 printf 风格记录普通信息日志。
func Infof(template string, args ...interface{}) { S.Infof(template, args...) }

// Warnf 使用 printf 风格记录警告日志。
func Warnf(template string, args ...interface{}) { S.Warnf(template, args...) }

// Errorf 使用 printf 风格记录错误日志。
func Errorf(template string, args ...interface{}) { S.Errorf(template, args...) }

// Debugf 使用 printf 风格记录调试日志。
func Debugf(template string, args ...interface{}) { S.Debugf(template, args...) }

// Fatalf 使用 printf 风格记录致命错误日志并终止程序。
func Fatalf(template string, args ...interface{}) { S.Fatalf(template, args...) }
