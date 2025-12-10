package storage  // 存储包，负责文件的下载和存储管理

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
)

// FileManager 文件管理器结构体
type FileManager struct {
	mu sync.RWMutex  // 读写锁，用于保护并发访问
}

// NewFileManager 创建新的文件管理器
func NewFileManager() *FileManager {
	return &FileManager{}
}

// ConcurrentDownload 并发下载多个文件
func (fm *FileManager) ConcurrentDownload(ctx context.Context, urls []string, tempDir string, maxConcurrent, maxRetries int) error {
	var wg sync.WaitGroup          // 等待组，用于等待所有goroutine完成
	sem := make(chan struct{}, maxConcurrent)  // 信号量，控制最大并发数
	errChan := make(chan error, len(urls))     // 错误通道，收集下载错误

	// 遍历所有要下载的URL
	for i, fileURL := range urls {
		wg.Add(1)     // 等待组计数加1
		sem <- struct{}{}  // 获取一个信号量，如果已满则等待

		// 为每个URL启动一个goroutine进行下载
		go func(index int, currentURL string) {
			defer wg.Done()          // goroutine结束时减少等待组计数
			defer func() { <-sem }() // 释放信号量，允许其他goroutine执行

			// 生成要保存的文件名
			filename, err := fm.generateFilename(currentURL, tempDir, index)
			if err != nil {
				errChan <- fmt.Errorf("生成文件名失败 [%s]: %w", currentURL, err)
				return
			}

			// 下载文件（带重试机制）
			if err := fm.downloadFileWithRetry(currentURL, filename, maxRetries); err != nil {
				errChan <- fmt.Errorf("下载失败 [%s]: %w", currentURL, err)
				return
			}

			// 下载成功，打印信息
			fmt.Printf("下载完成: %s", path.Base(filename))
		}(i, fileURL)
	}

	// 等待所有goroutine完成
	wg.Wait()
	close(errChan)  // 关闭错误通道

	// 检查是否有错误发生
	for err := range errChan {
		return err  // 如果有错误，返回第一个错误
	}

	return nil  // 所有下载都成功
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
	
	// 确保文件扩展名为.ts
	if !strings.HasSuffix(baseFilename, ".ts") {
		baseFilename += ".ts"
	}

	// 生成唯一文件名：时间戳_索引号_原文件名
	uniqueFilename := fmt.Sprintf("%s_%05d_%s",
		time.Now().Format("20060102_150405"),  // 当前时间，格式：年月日_时分秒
		index,                                 // 索引号，补零到5位
		baseFilename)                          // 原文件名

	// 拼接完整路径：临时目录/文件名
	return path.Join(tempDir, uniqueFilename), nil
}

// downloadFileWithRetry 带重试机制的下载
func (fm *FileManager) downloadFileWithRetry(fileURL, filepath string, maxRetries int) error {
	// 尝试下载，最多重试maxRetries次
	for i := 0; i < maxRetries; i++ {
		// 尝试下载单个文件
		if err := fm.downloadSingleFile(fileURL, filepath); err == nil {
			return nil  // 下载成功
		}
		
		// 如果不是最后一次重试，等待一段时间
		if i < maxRetries-1 {
			delay := time.Second * time.Duration(i+1)  // 重试延迟时间逐渐增加
			time.Sleep(delay)
		}
	}
	// 所有重试都失败
	return fmt.Errorf("达到最大重试次数: %s", fileURL)
}

// downloadSingleFile 下载单个文件
func (fm *FileManager) downloadSingleFile(fileURL, filepath string) error {
	// 发送HTTP GET请求
	resp, err := http.Get(fileURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()  // 确保响应体关闭

	// 检查HTTP状态码是否为200 OK
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP状态码: %d", resp.StatusCode)
	}

	// 检查文件是否已存在（避免重复下载）
	if _, err := os.Stat(filepath); err == nil {
		return nil  // 文件已存在，直接返回成功
	}

	// 创建新文件
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()  // 确保文件关闭

	// 将HTTP响应体复制到文件中
	_, err = io.Copy(out, resp.Body)
	return err
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
		baseName := strings.ReplaceAll(parsedURL.Host, ".", "_")  // 将点替换为下划线
		return fmt.Sprintf("%s_hls_segments", baseName), nil
	}

	// 去除文件扩展名
	baseName := strings.TrimSuffix(filename, path.Ext(filename))
	
	// 清理文件名，只保留安全字符（字母、数字、连字符、下划线）
	safeName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r  // 保留安全字符
		}
		return '_'  // 将不安全字符替换为下划线
	}, baseName)

	// 返回目录名：清理后的名称_hls_segments
	return fmt.Sprintf("%s_hls_segments", safeName), nil
}