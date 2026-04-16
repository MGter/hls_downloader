package storage // 存储包，负责文件的下载和存储管理

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/MGter/hls_downloader/internal/parser"
)

// FileManager 文件管理器结构体
type FileManager struct {
	mu sync.RWMutex // 读写锁，用于保护并发访问
}

// NewFileManager 创建新的文件管理器
func NewFileManager() *FileManager {
	return &FileManager{}
}

// ConcurrentDownload 并发下载多个文件（简化版本，兼容旧逻辑）
func (fm *FileManager) ConcurrentDownload(ctx context.Context, urls []string, tempDir string, maxConcurrent, maxRetries int) error {
	// 转换为简单的Segment结构
	segments := make([]parser.Segment, len(urls))
	for i, u := range urls {
		segments[i] = parser.Segment{URL: u}
	}
	return fm.ConcurrentDownloadSegments(ctx, segments, tempDir, maxConcurrent, maxRetries)
}

// ConcurrentDownloadSegments 并发下载多个片段（支持字节范围）
func (fm *FileManager) ConcurrentDownloadSegments(ctx context.Context, segments []parser.Segment, tempDir string, maxConcurrent, maxRetries int) error {
	var wg sync.WaitGroup                     // 等待组，用于等待所有goroutine完成
	sem := make(chan struct{}, maxConcurrent) // 信号量，控制最大并发数
	errChan := make(chan error, len(segments)) // 错误通道，收集下载错误

	// 隐式字节范围偏移追踪（用于连续的字节范围请求）
	var implicitOffset int64 = 0

	// 遍历所有要下载的片段
	for i, segment := range segments {
		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			wg.Wait() // 等待已启动的 goroutine 完成
			close(errChan)
			return ctx.Err() // 返回取消原因
		default:
		}

		wg.Add(1)            // 等待组计数加1
		sem <- struct{}{}    // 获取一个信号量，如果已满则等待

		// 计算实际的字节范围偏移
		byteRange := segment.ByteRange
		if byteRange != nil && byteRange.Offset == -1 {
			// 使用隐式偏移
			byteRange.Offset = implicitOffset
			implicitOffset += byteRange.Length
		} else if byteRange != nil {
			// 显式偏移，更新隐式偏移追踪
			implicitOffset = byteRange.Offset + byteRange.Length
		}

		// 为每个片段启动一个goroutine进行下载
		go func(index int, seg parser.Segment, br *parser.ByteRange) {
			defer wg.Done()          // goroutine结束时减少等待组计数
			defer func() { <-sem }() // 释放信号量，允许其他goroutine执行

			// 检查 context 是否已取消
			if ctx.Err() != nil {
				return // 直接返回，不下载
			}

			// 生成要保存的文件名
			filename, err := fm.generateFilename(seg.URL, tempDir, index)
			if err != nil {
				errChan <- fmt.Errorf("生成文件名失败 [%s]: %w", seg.URL, err)
				return
			}

			// 下载片段（带重试机制，支持字节范围）
			if err := fm.downloadSegmentWithRetry(ctx, seg.URL, filename, br, maxRetries); err != nil {
				errChan <- fmt.Errorf("下载失败 [%s]: %w", seg.URL, err)
				return
			}

			// 下载成功，打印信息
			// 下载成功，打印信息
				if seg.Duration > 0 {
					fmt.Printf("下载完成: %s (时长: %.2f秒)\n", path.Base(filename), seg.Duration)
				} else {
					fmt.Printf("下载完成: %s\n", path.Base(filename))
				}
		}(i, segment, byteRange)
	}

	// 等待所有goroutine完成
	wg.Wait()
	close(errChan) // 关闭错误通道

	// 检查是否有错误发生
	for err := range errChan {
		return err // 如果有错误，返回第一个错误
	}

	return nil // 所有下载都成功
}

// DownloadSegment 下载单个片段（支持字节范围）
func (fm *FileManager) DownloadSegment(ctx context.Context, segment parser.Segment, filepath string, maxRetries int) error {
	return fm.downloadSegmentWithRetry(ctx, segment.URL, filepath, segment.ByteRange, maxRetries)
}

// downloadSegmentWithRetry 带重试机制的片段下载（支持字节范围）
func (fm *FileManager) downloadSegmentWithRetry(ctx context.Context, fileURL, filepath string, byteRange *parser.ByteRange, maxRetries int) error {
	// 尝试下载，最多重试maxRetries次
	for i := 0; i < maxRetries; i++ {
		// 检查 context 是否已取消
		if ctx.Err() != nil {
			return ctx.Err() // 返回取消错误
		}

		// 尝试下载单个片段
		if err := fm.downloadSingleSegment(fileURL, filepath, byteRange); err == nil {
			return nil // 下载成功
		}

		// 如果不是最后一次重试，等待一段时间（支持 context 取消）
		if i < maxRetries-1 {
			delay := time.Second * time.Duration(i+1) // 重试延迟时间逐渐增加
			select {
			case <-time.After(delay): // 等待延迟时间
			case <-ctx.Done(): // context 取消，立即返回
				return ctx.Err()
			}
		}
	}
	// 所有重试都失败
	return fmt.Errorf("达到最大重试次数: %s", fileURL)
}

// downloadSingleSegment 下载单个片段（支持字节范围）
func (fm *FileManager) downloadSingleSegment(fileURL, filepath string, byteRange *parser.ByteRange) error {
	// 创建HTTP请求
	req, err := http.NewRequest("GET", fileURL, nil)
	if err != nil {
		return err
	}

	// 如果有字节范围，添加Range头
	if byteRange != nil && byteRange.Length > 0 {
		start := byteRange.Offset
		end := start + byteRange.Length - 1
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	}

	// 发送HTTP请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() // 确保响应体关闭

	// 检查HTTP状态码（支持206 Partial Content）
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("HTTP状态码: %d", resp.StatusCode)
	}

	// 检查文件是否已存在（避免重复下载）
	if _, err := os.Stat(filepath); err == nil {
		return nil // 文件已存在，直接返回成功
	}

	// 创建新文件
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close() // 确保文件关闭

	// 将HTTP响应体复制到文件中
	_, err = io.Copy(out, resp.Body)
	return err
}

// generateFilename 生成唯一的文件名
func (fm *FileManager) generateFilename(tsURL, tempDir string, index int) (string, error) {
	// 解析URL
	parsedURL, err := url.Parse(tsURL)
	if err != nil {
		return "", err
	}

	// 从URL路径中获取基础文件名
	baseFilename := path.Base(parsedURL.Path)

	// 确保文件有扩展名
	// 支持 .ts, .m4s, .mp4 等格式
	ext := path.Ext(baseFilename)
	if ext == "" {
		// 默认使用 .ts 扩展名
		baseFilename += ".ts"
	} else if ext != ".ts" && ext != ".m4s" && ext != ".mp4" {
		// 其他格式保持原扩展名
	}

	// 生成唯一文件名：时间戳_索引号_原文件名
	uniqueFilename := fmt.Sprintf("%s_%05d_%s",
		time.Now().Format("20060102_150405"), // 当前时间，格式：年月日_时分秒
		index,                                // 索引号，补零到5位
		baseFilename)                         // 原文件名

	// 拼接完整路径：临时目录/文件名
	return path.Join(tempDir, uniqueFilename), nil
}

// downloadFileWithRetry 带重试机制的下载（兼容旧逻辑）
func (fm *FileManager) downloadFileWithRetry(fileURL, filepath string, maxRetries int) error {
	return fm.downloadSegmentWithRetry(context.Background(), fileURL, filepath, nil, maxRetries)
}

// downloadFileWithRetryContext 带重试机制的下载（支持 context 取消）
func (fm *FileManager) downloadFileWithRetryContext(ctx context.Context, fileURL, filepath string, maxRetries int) error {
	return fm.downloadSegmentWithRetry(ctx, fileURL, filepath, nil, maxRetries)
}

// downloadSingleFile 下载单个文件（兼容旧逻辑）
func (fm *FileManager) downloadSingleFile(fileURL, filepath string) error {
	return fm.downloadSingleSegment(fileURL, filepath, nil)
}

// DeriveOutputDir 根据URL生成输出目录名
func (fm *FileManager) DeriveOutputDir(hlsURL string) (string, error) {
	// 解析URL
	parsedURL, err := url.Parse(hlsURL)
	if err != nil {
		return "", fmt.Errorf("解析 URL 失败: %w", err)
	}

	// 获取URL路径中的文件名部分
	filename := path.Base(parsedURL.Path)

	// 如果URL中没有文件名（如纯目录路径）
	if filename == "" || filename == "." || filename == "/" {
		// 使用主机名（域名）作为基础名称
		baseName := strings.ReplaceAll(parsedURL.Host, ".", "_") // 将点替换为下划线
		return fmt.Sprintf("%s_hls_segments", baseName), nil
	}

	// 去除文件扩展名
	baseName := strings.TrimSuffix(filename, path.Ext(filename))

	// 清理文件名，只保留安全字符（字母、数字、连字符、下划线）
	safeName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r // 保留安全字符
		}
		return '_' // 将不安全字符替换为下划线
	}, baseName)

	// 返回目录名：清理后的名称_hls_segments
	return fmt.Sprintf("%s_hls_segments", safeName), nil
}