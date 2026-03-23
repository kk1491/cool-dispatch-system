package logger

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GinMiddleware 返回 Gin 请求日志中间件，替代 gin.Logger()。
// 每个请求结束后输出一行结构化日志，包含方法、路径、状态码、耗时、客户端 IP。
// - 5xx → Error 级别（写入 error.log）
// - 4xx → Warn 级别
// - 其余 → Info 级别
func GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// 执行后续处理链
		c.Next()

		// 计算请求耗时
		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		// 拼接完整路径（含 query string）
		if query != "" {
			path = path + "?" + query
		}

		// 构造通用日志字段
		fields := []zap.Field{
			zap.Int("status", status),
			zap.String("method", method),
			zap.String("path", path),
			zap.String("ip", clientIP),
			zap.Duration("latency", latency),
			zap.Int("size", c.Writer.Size()),
		}

		// Gin 中间件产生的错误也记录到日志
		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.String()))
		}

		// 按状态码分级输出
		switch {
		case status >= 500:
			Error("server error", fields...)
		case status >= 400:
			Warn("client error", fields...)
		default:
			Info("request", fields...)
		}
	}
}

// GinRecoveryMiddleware 返回 Gin panic 恢复中间件，替代 gin.Recovery()。
// 捕获 panic 后输出 Error 级别日志（含堆栈），返回 500 并继续服务后续请求。
func GinRecoveryMiddleware() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		Error("panic recovered",
			zap.Any("error", recovered),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("ip", c.ClientIP()),
			zap.Stack("stacktrace"),
		)
		c.AbortWithStatus(500)
	})
}
