// Package logger 中的 prependWriter 实现"新日志写在文件顶部"的 WriteSyncer。
//
// 核心思路：
//   - 每次 Write 时，先读取原文件内容，把新数据拼在最前面再整体写回。
//   - 当文件大小超过 MaxSize（MB）时，自动轮转：
//     当前文件重命名为 backup（带时间戳），然后开启新文件继续写入。
//   - 最多保留 MaxBackups 个备份文件，超出的按时间顺序删除最旧的。
//   - 使用互斥锁保证并发安全。
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// prependWriter 实现 zapcore.WriteSyncer 接口，
// 每次写入的新日志会出现在文件最上方，方便打开文件后直接看到最新内容。
type prependWriter struct {
	mu         sync.Mutex // 并发写入保护
	filename   string     // 日志文件完整路径（如 logs/app.log）
	maxSize    int64      // 单个文件最大字节数（超过后轮转）
	maxBackups int        // 最多保留的备份文件数量
}

// newPrependWriter 创建一个新的 prependWriter 实例。
// 参数：
//   - filename: 日志文件路径
//   - maxSizeMB: 单文件最大大小（MB）
//   - maxBackups: 最多保留备份数
func newPrependWriter(filename string, maxSizeMB int, maxBackups int) *prependWriter {
	return &prependWriter{
		filename:   filename,
		maxSize:    int64(maxSizeMB) * 1024 * 1024, // MB 转换为字节
		maxBackups: maxBackups,
	}
}

// Write 实现 io.Writer 接口。
// 将新数据写入文件开头，保持"最新日志在最上面"的效果。
// 如果写入后文件大小超限，自动触发轮转。
func (w *prependWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// 读取现有文件内容（文件不存在则为空）
	existing, _ := os.ReadFile(w.filename)

	// 确保日志目录存在
	dir := filepath.Dir(w.filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 拼接新内容：新日志在前 + 旧内容在后
	newContent := append([]byte{}, p...)
	newContent = append(newContent, existing...)

	// 写入文件（覆盖模式，权限 0644）
	if err := os.WriteFile(w.filename, newContent, 0644); err != nil {
		return 0, fmt.Errorf("写入日志文件失败: %w", err)
	}

	// 检查是否需要轮转（文件大小超过上限）
	if int64(len(newContent)) >= w.maxSize {
		w.rotate()
	}

	return len(p), nil
}

// Sync 实现 zapcore.WriteSyncer 接口，刷新文件缓冲区。
// 由于 Write 中使用 os.WriteFile 直接写入，这里无需额外操作。
func (w *prependWriter) Sync() error {
	return nil
}

// rotate 执行日志轮转：
// 1. 将当前文件重命名为带时间戳的备份文件
// 2. 清理超出 maxBackups 限制的旧备份
func (w *prependWriter) rotate() {
	// 生成备份文件名，格式：app-20260325-150714.log
	ext := filepath.Ext(w.filename)                         // .log
	base := strings.TrimSuffix(w.filename, ext)             // logs/app
	timestamp := time.Now().Format("20060102-150405")       // 时间戳
	backupName := fmt.Sprintf("%s-%s%s", base, timestamp, ext) // logs/app-20260325-150714.log

	// 重命名当前文件为备份
	_ = os.Rename(w.filename, backupName)

	// 清理多余的备份文件
	w.removeOldBackups()
}

// removeOldBackups 删除超出 maxBackups 限制的旧备份文件。
// 备份文件按修改时间排序，保留最新的 maxBackups 个，删除其余。
func (w *prependWriter) removeOldBackups() {
	dir := filepath.Dir(w.filename)
	ext := filepath.Ext(w.filename)
	baseName := strings.TrimSuffix(filepath.Base(w.filename), ext) // 如 "app"

	// 扫描目录，找出所有备份文件（匹配 app-*.log 模式）
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// 备份文件信息列表
	type backupInfo struct {
		path    string   // 备份文件完整路径
		modTime time.Time // 修改时间
	}
	var backups []backupInfo

	prefix := baseName + "-" // 备份文件前缀，如 "app-"
	for _, entry := range entries {
		name := entry.Name()
		// 过滤：必须以 prefix 开头、以 ext 结尾，且不是当前活跃日志文件
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ext) && name != filepath.Base(w.filename) {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			backups = append(backups, backupInfo{
				path:    filepath.Join(dir, name),
				modTime: info.ModTime(),
			})
		}
	}

	// 如果备份数量未超限，无需清理
	if len(backups) <= w.maxBackups {
		return
	}

	// 按修改时间升序排列（最旧的在前面）
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].modTime.Before(backups[j].modTime)
	})

	// 删除最旧的备份，只保留最新的 maxBackups 个
	removeCount := len(backups) - w.maxBackups
	for i := 0; i < removeCount; i++ {
		_ = os.Remove(backups[i].path)
	}
}
