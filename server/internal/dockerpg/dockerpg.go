// Package dockerpg 在 Go 服务启动时自动检测并确保 PostgreSQL Docker 容器处于运行状态。
// 适用于服务器重启后只启动了 Go 程序、而 PostgreSQL 容器尚未拉起的场景。
// 如果容器已在运行则不做任何操作，不影响正常启动性能。
package dockerpg

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"cool-dispatch/internal/logger"
)

// Config 定义 PostgreSQL 容器管理所需的配置参数。
type Config struct {
	// ContainerName 是 Docker 容器名称，与 docker-compose.postgres.yml 一致。
	ContainerName string
	// ComposeFile 是 docker-compose 文件的绝对路径。
	ComposeFile string
	// ServiceName 是 compose 文件中 PostgreSQL 服务的名称。
	ServiceName string
	// Port 是 PostgreSQL 对外暴露的宿主机端口，用于就绪检测。
	Port string
	// ReadyTimeout 是等待容器就绪的超时时间。
	ReadyTimeout time.Duration
}

// DefaultConfig 返回项目默认的 PostgreSQL 容器配置。
func DefaultConfig(composeFile string) Config {
	return Config{
		ContainerName: envOrDefault("POSTGRES_CONTAINER_NAME", "cool-dispatch-postgres"),
		ComposeFile:   composeFile,
		ServiceName:   "postgres",
		Port:          envOrDefault("POSTGRES_PORT", "9101"),
		ReadyTimeout:  30 * time.Second,
	}
}

// EnsureRunning 检查 PostgreSQL Docker 容器状态，未运行时自动拉起。
// 如果容器已在运行，函数立即返回 nil，不会有任何副作用。
// 整个过程使用 logger 输出日志，方便在服务端日志中追踪。
func EnsureRunning(cfg Config) error {
	// 第一步：检查 Docker 是否已安装
	if !isDockerInstalled() {
		logger.Warnf("[dockerpg] Docker 未安装，跳过 PostgreSQL 容器检测（请确保数据库已通过其他方式启动）")
		return nil
	}

	// 第二步：检查容器是否已在运行
	if isContainerRunning(cfg.ContainerName) {
		logger.Infof("[dockerpg] PostgreSQL 容器已在运行: %s", cfg.ContainerName)
		return nil
	}

	// 第三步：容器未运行，尝试拉起
	logger.Warnf("[dockerpg] PostgreSQL 容器未运行: %s，正在尝试启动...", cfg.ContainerName)

	// 优先尝试：容器存在但已停止，直接 start
	if isContainerExists(cfg.ContainerName) {
		logger.Infof("[dockerpg] 容器已存在但未运行，执行 docker compose start...")
		if err := dockerComposeStart(cfg); err != nil {
			logger.Errorf("[dockerpg] docker compose start 失败: %v，尝试 up -d...", err)
			// start 失败时回退到 up -d
			if err2 := dockerComposeUp(cfg); err2 != nil {
				return fmt.Errorf("无法启动 PostgreSQL 容器: start 失败=%v, up 失败=%v", err, err2)
			}
		}
	} else {
		// 容器不存在，使用 up -d 创建并启动
		logger.Infof("[dockerpg] 容器不存在，执行 docker compose up -d...")
		if err := dockerComposeUp(cfg); err != nil {
			return fmt.Errorf("无法创建并启动 PostgreSQL 容器: %v", err)
		}
	}

	// 第四步：等待容器就绪
	if err := waitForReady(cfg); err != nil {
		return err
	}

	logger.Infof("[dockerpg] PostgreSQL 容器已成功启动并就绪: %s", cfg.ContainerName)
	return nil
}

// isDockerInstalled 检查系统是否安装了 Docker。
func isDockerInstalled() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

// isContainerRunning 检查指定名称的容器是否正在运行。
func isContainerRunning(name string) bool {
	// docker ps --format '{{.Names}}' 只列出正在运行的容器名
	out, err := exec.Command("docker", "ps", "--format", "{{.Names}}").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

// isContainerExists 检查指定名称的容器是否存在（包括已停止的）。
func isContainerExists(name string) bool {
	// docker ps -a --format '{{.Names}}' 列出所有容器名
	out, err := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

// dockerComposeStart 对已存在但停止的容器执行 start。
func dockerComposeStart(cfg Config) error {
	cmd := exec.Command("docker", "compose", "-f", cfg.ComposeFile, "start", cfg.ServiceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// dockerComposeUp 使用 docker compose up -d 创建并启动容器。
func dockerComposeUp(cfg Config) error {
	cmd := exec.Command("docker", "compose", "-f", cfg.ComposeFile, "up", "-d", cfg.ServiceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// waitForReady 轮询等待 PostgreSQL 容器就绪。
// 通过 docker inspect 检查容器健康状态或运行状态来判定。
func waitForReady(cfg Config) error {
	deadline := time.Now().Add(cfg.ReadyTimeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	logger.Infof("[dockerpg] 等待 PostgreSQL 容器就绪（超时: %s）...", cfg.ReadyTimeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("PostgreSQL 容器未在 %s 内就绪，请手动检查: docker logs %s", cfg.ReadyTimeout, cfg.ContainerName)
		}

		// 检查容器健康/运行状态
		status := getContainerStatus(cfg.ContainerName)
		if status == "healthy" || status == "running" {
			logger.Infof("[dockerpg] PostgreSQL 容器状态: %s", status)
			return nil
		}

		<-ticker.C
	}
}

// getContainerStatus 获取容器的健康状态或运行状态。
func getContainerStatus(name string) string {
	// 优先使用 Health.Status，没有健康检查则回退到 State.Status
	out, err := exec.Command(
		"docker", "inspect",
		"--format", "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}",
		name,
	).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// envOrDefault 从环境变量读取值，不存在时返回默认值。
func envOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
