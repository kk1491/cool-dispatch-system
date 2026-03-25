// Package backup 实现 PostgreSQL 数据库定时备份功能。
//
// 核心特性：
//   - 在独立协程中运行，不影响主服务正常处理请求
//   - 每天自动执行一次 pg_dump 全量备份
//   - 备份文件保存为 .sql.gz（gzip 压缩），大幅节省磁盘空间
//   - 最多保留近 7 天的备份，超出的自动清理
//   - 支持通过 context 优雅关闭
//   - 启动时立即执行一次备份，后续按 24 小时间隔定时触发
package backup

import (
	"compress/gzip"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"cool-dispatch/internal/logger"
)

// Config 数据库备份配置。
type Config struct {
	// DatabaseURL 是 PostgreSQL 连接字符串，用于提取 pg_dump 所需的连接参数。
	// 格式：postgres://user:pass@host:port/dbname?sslmode=disable
	DatabaseURL string

	// BackupDir 是备份文件存放目录，可以是相对路径或绝对路径。
	// 默认值为 "backups"，会在项目根目录下创建。
	BackupDir string

	// MaxBackups 是最多保留的备份文件数量。
	// 超出后自动删除最旧的备份。默认值为 7（保留近 7 天）。
	MaxBackups int

	// Interval 是两次备份之间的间隔时间。
	// 默认值为 24 小时（每天一次）。
	Interval time.Duration
}

// pgConnInfo 从 DatabaseURL 解析出的 PostgreSQL 连接参数，
// 用于构造 pg_dump 命令行或 PGPASSWORD 环境变量。
type pgConnInfo struct {
	host     string // 数据库主机地址
	port     string // 数据库端口
	user     string // 数据库用户名
	password string // 数据库密码
	dbName   string // 数据库名称
}

// parseDatabaseURL 从 PostgreSQL 连接字符串中提取各项连接参数。
// 支持标准 postgres:// URI 格式。
func parseDatabaseURL(dbURL string) (*pgConnInfo, error) {
	u, err := url.Parse(dbURL)
	if err != nil {
		return nil, fmt.Errorf("解析数据库 URL 失败: %w", err)
	}

	info := &pgConnInfo{
		host:   u.Hostname(),
		port:   u.Port(),
		dbName: strings.TrimPrefix(u.Path, "/"),
	}

	// 提取用户名和密码
	if u.User != nil {
		info.user = u.User.Username()
		info.password, _ = u.User.Password()
	}

	// 应用默认值
	if info.host == "" {
		info.host = "localhost"
	}
	if info.port == "" {
		info.port = "5432"
	}
	if info.user == "" {
		info.user = "postgres"
	}

	return info, nil
}

// StartScheduler 在独立协程中启动定时备份调度器。
// 启动后立即执行一次备份，后续按 cfg.Interval 间隔定时触发。
// 通过 ctx 取消可优雅停止调度器。
//
// 使用示例：
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	backup.StartScheduler(ctx, backup.Config{
//	    DatabaseURL: cfg.DatabaseURL,
//	    BackupDir:   "backups",
//	    MaxBackups:  7,
//	})
func StartScheduler(ctx context.Context, cfg Config) {
	// 应用默认值
	if cfg.BackupDir == "" {
		cfg.BackupDir = "backups"
	}
	if cfg.MaxBackups <= 0 {
		cfg.MaxBackups = 7
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 24 * time.Hour
	}

	// 确保备份目录存在
	if err := os.MkdirAll(cfg.BackupDir, 0755); err != nil {
		logger.Errorf("[backup] 创建备份目录失败: %v", err)
		return
	}

	// 在独立协程中运行，不阻塞主流程
	go func() {
		logger.Infof("[backup] 定时备份调度器已启动，备份间隔: %v，最多保留: %d 份，目录: %s",
			cfg.Interval, cfg.MaxBackups, cfg.BackupDir)

		// 启动后立即执行一次备份
		runBackup(cfg)

		// 使用 ticker 定时触发后续备份
		ticker := time.NewTicker(cfg.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// 收到取消信号，优雅退出
				logger.Infof("[backup] 定时备份调度器已停止")
				return
			case <-ticker.C:
				// 定时触发备份
				runBackup(cfg)
			}
		}
	}()
}

// runBackup 执行一次完整的备份流程：
// 1. 解析数据库连接参数
// 2. 执行 pg_dump 导出
// 3. 压缩备份文件
// 4. 清理过期备份
func runBackup(cfg Config) {
	startTime := time.Now()
	logger.Infof("[backup] 开始执行数据库备份...")

	// 解析数据库连接信息
	connInfo, err := parseDatabaseURL(cfg.DatabaseURL)
	if err != nil {
		logger.Errorf("[backup] 解析数据库连接失败: %v", err)
		return
	}

	// 生成备份文件名，格式：cool_dispatch_20260325_150714.sql.gz
	timestamp := startTime.Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.sql.gz", connInfo.dbName, timestamp)
	backupPath := filepath.Join(cfg.BackupDir, filename)

	// 执行 pg_dump 并压缩写入文件
	if err := dumpAndCompress(connInfo, backupPath); err != nil {
		logger.Errorf("[backup] 数据库备份失败: %v", err)
		// 清理可能残留的不完整文件
		_ = os.Remove(backupPath)
		return
	}

	// 获取备份文件大小用于日志展示
	fileInfo, _ := os.Stat(backupPath)
	sizeMB := float64(0)
	if fileInfo != nil {
		sizeMB = float64(fileInfo.Size()) / 1024 / 1024
	}

	elapsed := time.Since(startTime)
	logger.Infof("[backup] 数据库备份完成: %s (%.2f MB, 耗时 %v)", filename, sizeMB, elapsed)

	// 清理超出保留数量的旧备份
	cleanOldBackups(cfg.BackupDir, connInfo.dbName, cfg.MaxBackups)
}

// dumpAndCompress 使用 pg_dump 导出数据库并通过 gzip 压缩写入目标文件。
// pg_dump 的 stdout 直接通过管道连接到 gzip 压缩器，避免中间文件占用磁盘。
func dumpAndCompress(info *pgConnInfo, outputPath string) error {
	// 检查 pg_dump 是否可用
	pgDumpPath, err := exec.LookPath("pg_dump")
	if err != nil {
		return fmt.Errorf("找不到 pg_dump 命令，请确认已安装 PostgreSQL 客户端工具: %w", err)
	}

	// 构造 pg_dump 命令
	// --no-owner: 不输出 ALTER OWNER 语句，方便在不同环境恢复
	// --no-privileges: 不输出 GRANT/REVOKE 语句
	// --clean: 输出 DROP 语句，恢复时先清理旧对象
	// --if-exists: DROP 语句加 IF EXISTS，避免首次恢复报错
	cmd := exec.Command(pgDumpPath,
		"-h", info.host,
		"-p", info.port,
		"-U", info.user,
		"-d", info.dbName,
		"--no-owner",
		"--no-privileges",
		"--clean",
		"--if-exists",
	)

	// 通过 PGPASSWORD 环境变量传递密码，避免交互式输入
	cmd.Env = append(os.Environ(), "PGPASSWORD="+info.password)

	// 获取 pg_dump 的 stdout 管道
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("创建 pg_dump stdout 管道失败: %w", err)
	}

	// 创建目标压缩文件
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("创建备份文件失败: %w", err)
	}
	defer outFile.Close()

	// 创建 gzip 压缩写入器
	gzWriter, err := gzip.NewWriterLevel(outFile, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("创建 gzip 压缩器失败: %w", err)
	}

	// 启动 pg_dump 进程
	if err := cmd.Start(); err != nil {
		gzWriter.Close()
		return fmt.Errorf("启动 pg_dump 失败: %w", err)
	}

	// 使用 WaitGroup 确保数据读取完成后再关闭
	var wg sync.WaitGroup
	var readErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		// 从 pg_dump stdout 读取数据并写入 gzip 压缩器
		buf := make([]byte, 32*1024) // 32KB 缓冲区
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				if _, writeErr := gzWriter.Write(buf[:n]); writeErr != nil {
					readErr = fmt.Errorf("写入压缩数据失败: %w", writeErr)
					return
				}
			}
			if err != nil {
				break // EOF 或其他读取错误
			}
		}
	}()

	// 等待数据读取协程完成
	wg.Wait()

	// 关闭 gzip 写入器（刷新压缩缓冲区）
	if err := gzWriter.Close(); err != nil {
		return fmt.Errorf("关闭 gzip 压缩器失败: %w", err)
	}

	// 等待 pg_dump 进程退出
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("pg_dump 执行失败: %w", err)
	}

	// 检查数据读取是否出错
	if readErr != nil {
		return readErr
	}

	return nil
}

// cleanOldBackups 清理超出保留数量的旧备份文件。
// 按文件修改时间排序，保留最新的 maxBackups 个，删除其余。
func cleanOldBackups(backupDir string, dbName string, maxBackups int) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		logger.Errorf("[backup] 读取备份目录失败: %v", err)
		return
	}

	// 备份文件信息
	type fileEntry struct {
		path    string    // 完整路径
		modTime time.Time // 修改时间
	}

	var backups []fileEntry
	prefix := dbName + "_" // 匹配当前数据库的备份文件
	suffix := ".sql.gz"

	for _, entry := range entries {
		name := entry.Name()
		// 只匹配当前数据库的备份文件
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) && !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			backups = append(backups, fileEntry{
				path:    filepath.Join(backupDir, name),
				modTime: info.ModTime(),
			})
		}
	}

	// 如果备份数量未超限，无需清理
	if len(backups) <= maxBackups {
		return
	}

	// 按修改时间升序排列（最旧的在前面）
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].modTime.Before(backups[j].modTime)
	})

	// 删除最旧的备份，只保留最新的 maxBackups 个
	removeCount := len(backups) - maxBackups
	for i := 0; i < removeCount; i++ {
		if err := os.Remove(backups[i].path); err != nil {
			logger.Errorf("[backup] 删除旧备份失败: %s, 错误: %v", backups[i].path, err)
		} else {
			logger.Infof("[backup] 已清理过期备份: %s", filepath.Base(backups[i].path))
		}
	}
}
