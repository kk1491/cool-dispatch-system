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
	"bytes"
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

const defaultPostgresContainerName = "cool-dispatch-postgres"
const defaultPostgresContainerPort = "5432"

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
// 启动后会先检查今天是否已经生成过备份；若已备份则直接等待下次到期时间，
// 若今天尚未备份则立即补做一次，确保“每天最多一次”在重启后依然成立。
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

		for {
			// 每轮都重新计算下次备份到期时间，确保服务重启后不会因为固定 ticker 导致重复备份。
			waitDuration, scheduleLog := nextBackupDelay(cfg, time.Now())
			if scheduleLog != "" {
				logger.Infof("[backup] %s", scheduleLog)
			}

			timer := time.NewTimer(waitDuration)
			select {
			case <-ctx.Done():
				// 收到取消信号后，先回收 timer，避免 goroutine 或 channel 残留。
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				logger.Infof("[backup] 定时备份调度器已停止")
				return
			case <-timer.C:
				// 到期后执行一次备份；下轮循环会再次判断是否需要跳过。
				runBackup(cfg)
			}
		}
	}()
}

// nextBackupDelay 计算距离“下次允许备份”还需要等待多久。
// 规则如下：
// 1. 若当前数据库今天还没有备份记录，则立即执行，返回 0。
// 2. 若今天已经备份过，则等待“最近一次备份时间 + interval”后再执行。
// 这样可保证同一天多次重启时不会再次触发备份，同时仍保持每天一次的节奏。
func nextBackupDelay(cfg Config, now time.Time) (time.Duration, string) {
	connInfo, err := parseDatabaseURL(cfg.DatabaseURL)
	if err != nil {
		logger.Errorf("[backup] 解析数据库连接失败，改为立即尝试备份: %v", err)
		return 0, ""
	}

	latestBackupTime, found := findLatestBackupTime(cfg.BackupDir, connInfo.dbName)
	if !found {
		return 0, "尚未发现历史备份，将立即执行首次备份"
	}

	if sameDay(latestBackupTime, now) {
		nextTime := latestBackupTime.Add(cfg.Interval)
		waitDuration := time.Until(nextTime)
		if waitDuration < 0 {
			waitDuration = 0
		}
		return waitDuration, fmt.Sprintf(
			"今天已备份过，最近一次备份时间: %s；下次最早备份时间: %s",
			latestBackupTime.Format("2006-01-02 15:04:05"),
			nextTime.Format("2006-01-02 15:04:05"),
		)
	}

	return 0, "今天尚未备份，将立即执行补偿备份"
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
	// 优先使用宿主机 pg_dump；若宿主机未安装，则自动回退到 PostgreSQL 容器内执行。
	cmd, runnerDesc, err := buildDumpCommand(info)
	if err != nil {
		return err
	}
	logger.Infof("[backup] 本次备份使用导出方式: %s", runnerDesc)

	// 获取 pg_dump 的 stdout 管道
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("创建 pg_dump stdout 管道失败: %w", err)
	}

	// 收集 stderr，便于在备份失败时快速看到 pg_dump 的原始报错。
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

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
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("pg_dump 执行失败: %w: %s", err, errMsg)
		}
		return fmt.Errorf("pg_dump 执行失败: %w", err)
	}

	// 检查数据读取是否出错
	if readErr != nil {
		return readErr
	}

	return nil
}

// buildDumpCommand 构造 pg_dump 导出命令。
// 优先级如下：
// 1. 若宿主机已安装 pg_dump，则直接在宿主机执行。
// 2. 若宿主机缺少 pg_dump，但 Docker 与 PostgreSQL 容器可用，则回退到容器内执行。
// 3. 若两者都不可用，则返回明确错误，提醒补齐运行环境。
func buildDumpCommand(info *pgConnInfo) (*exec.Cmd, string, error) {
	dumpArgs := []string{
		"-h", info.host,
		"-p", info.port,
		"-U", info.user,
		"-d", info.dbName,
		"--no-owner",
		"--no-privileges",
		"--clean",
		"--if-exists",
	}

	if pgDumpPath, err := exec.LookPath("pg_dump"); err == nil {
		cmd := exec.Command(pgDumpPath, dumpArgs...)
		cmd.Env = append(os.Environ(), "PGPASSWORD="+info.password)
		return cmd, "宿主機 pg_dump", nil
	}

	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return nil, "", fmt.Errorf("找不到 pg_dump，且系統也無法使用 Docker，無法完成資料庫備份")
	}

	containerName := strings.TrimSpace(os.Getenv("POSTGRES_CONTAINER_NAME"))
	if containerName == "" {
		containerName = defaultPostgresContainerName
	}

	if !isDockerContainerRunning(containerName) {
		return nil, "", fmt.Errorf("找不到 pg_dump，且 PostgreSQL 容器未在運行: %s", containerName)
	}

	// 容器內執行 pg_dump 時，若原始連線配置指向宿主機 localhost:9101，
	// 需要切換為容器內 PostgreSQL 的監聽位址與容器端口，否則會在容器內錯誤地回連宿主機端口。
	dockerConnInfo := normalizeConnInfoForDockerExec(info)
	dockerDumpArgs := []string{
		"-h", dockerConnInfo.host,
		"-p", dockerConnInfo.port,
		"-U", dockerConnInfo.user,
		"-d", dockerConnInfo.dbName,
		"--no-owner",
		"--no-privileges",
		"--clean",
		"--if-exists",
	}

	dockerArgs := []string{
		"exec",
		"-e", "PGPASSWORD=" + dockerConnInfo.password,
		containerName,
		"pg_dump",
	}
	dockerArgs = append(dockerArgs, dockerDumpArgs...)

	return exec.Command(dockerPath, dockerArgs...), fmt.Sprintf("Docker 容器 %s 內的 pg_dump", containerName), nil
}

// normalizeConnInfoForDockerExec 将宿主机视角的数据库地址改写为容器内可访问的地址。
// 当前项目 PostgreSQL 默认运行在同一个容器内，因此 localhost / 127.0.0.1 / ::1
// 都应改为容器内环回地址，并使用容器端口 5432（或 POSTGRES_CONTAINER_PORT 指定值）。
func normalizeConnInfoForDockerExec(info *pgConnInfo) *pgConnInfo {
	normalized := *info

	switch strings.TrimSpace(strings.ToLower(normalized.host)) {
	case "", "localhost", "127.0.0.1", "::1":
		normalized.host = "127.0.0.1"
		containerPort := strings.TrimSpace(os.Getenv("POSTGRES_CONTAINER_PORT"))
		if containerPort == "" {
			containerPort = defaultPostgresContainerPort
		}
		normalized.port = containerPort
	}

	if normalized.port == "" {
		normalized.port = defaultPostgresContainerPort
	}

	return &normalized
}

// isDockerContainerRunning 检查目标 Docker 容器是否正在运行。
// 这里只作为备份的兜底判断，出现命令失败时直接按“不可用”处理即可。
func isDockerContainerRunning(containerName string) bool {
	output, err := exec.Command("docker", "ps", "--format", "{{.Names}}").Output()
	if err != nil {
		return false
	}

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) == containerName {
			return true
		}
	}

	return false
}

// findLatestBackupTime 扫描指定数据库的备份文件，并返回最新一份备份的修改时间。
// 若没有找到任何备份文件，则返回 false，供调度器判断是否需要首次备份。
func findLatestBackupTime(backupDir string, dbName string) (time.Time, bool) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return time.Time{}, false
	}

	prefix := dbName + "_"
	suffix := ".sql.gz"
	var latest time.Time
	found := false

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// 失败中断时可能残留 0-byte 空文件，这类文件不能视为有效备份，
		// 否则重启后会误判“今天已完成备份”，导致当天不再重试。
		if info.Size() <= 0 {
			continue
		}

		if !found || info.ModTime().After(latest) {
			latest = info.ModTime()
			found = true
		}
	}

	return latest, found
}

// sameDay 判断两个时间是否处于同一个本地自然日。
// 备份重复判定采用自然日维度，更符合“每天只备份一次”的业务语义。
func sameDay(left time.Time, right time.Time) bool {
	leftYear, leftMonth, leftDay := left.Date()
	rightYear, rightMonth, rightDay := right.Date()
	return leftYear == rightYear && leftMonth == rightMonth && leftDay == rightDay
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
