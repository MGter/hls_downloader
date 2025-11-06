package storage

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

type FileManager struct {
	mu sync.RWMutex
}

func NewFileManager() *FileManager {
	return &FileManager{}
}

// ConcurrentDownload 并发下载文件
func (fm *FileManager) ConcurrentDownload(ctx context.Context, urls []string, tempDir string, maxConcurrent, maxRetries int) error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrent)
	errChan := make(chan error, len(urls))

	for i, fileURL := range urls {
		wg.Add(1)
		sem <- struct{}{}

		go func(index int, currentURL string) {
			defer wg.Done()
			defer func() { <-sem }()

			filename, err := fm.generateFilename(currentURL, tempDir, index)
			if err != nil {
				errChan <- fmt.Errorf("生成文件名失败 [%s]: %w", currentURL, err)
				return
			}

			if err := fm.downloadFileWithRetry(currentURL, filename, maxRetries); err != nil {
				errChan <- fmt.Errorf("下载失败 [%s]: %w", currentURL, err)
				return
			}

			fmt.Printf("下载完成: %s", path.Base(filename))
		}(i, fileURL)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		return err
	}

	return nil
}

// generateFilename 生成唯一的文件名
func (fm *FileManager) generateFilename(tsURL, tempDir string, index int) (string, error) {
	parsedURL, err := url.Parse(tsURL)
	if err != nil {
		return "", err
	}

	baseFilename := path.Base(parsedURL.Path)
	if !strings.HasSuffix(baseFilename, ".ts") {
		baseFilename += ".ts"
	}

	uniqueFilename := fmt.Sprintf("%s_%05d_%s",
		time.Now().Format("20060102_150405"), index, baseFilename)

	return path.Join(tempDir, uniqueFilename), nil
}

// downloadFileWithRetry 带重试的下载
func (fm *FileManager) downloadFileWithRetry(fileURL, filepath string, maxRetries int) error {
	for i := 0; i < maxRetries; i++ {
		if err := fm.downloadSingleFile(fileURL, filepath); err == nil {
			return nil
		}
		if i < maxRetries-1 {
			delay := time.Second * time.Duration(i+1)
			time.Sleep(delay)
		}
	}
	return fmt.Errorf("达到最大重试次数: %s", fileURL)
}

// downloadSingleFile 下载单个文件
func (fm *FileManager) downloadSingleFile(fileURL, filepath string) error {
	resp, err := http.Get(fileURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP状态码: %d", resp.StatusCode)
	}

	if _, err := os.Stat(filepath); err == nil {
		return nil // 文件已存在
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// DeriveOutputDir 推导输出目录
func (fm *FileManager) DeriveOutputDir(hlsURL string) (string, error) {
	parsedURL, err := url.Parse(hlsURL)
	if err != nil {
		return "", fmt.Errorf("解析 URL 失败: %w", err)
	}

	filename := path.Base(parsedURL.Path)
	if filename == "" || filename == "." || filename == "/" {
		baseName := strings.ReplaceAll(parsedURL.Host, ".", "_")
		return fmt.Sprintf("%s_hls_segments", baseName), nil
	}

	baseName := strings.TrimSuffix(filename, path.Ext(filename))
	safeName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, baseName)

	return fmt.Sprintf("%s_hls_segments", safeName), nil
}