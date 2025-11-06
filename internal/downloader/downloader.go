package downloader

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/MGter/hls_downloader/internal/parser"
	"github.com/MGter/hls_downloader/internal/storage"
	"github.com/MGter/hls_downloader/pkg/utils"
)

type Config struct {
	MaxConcurrentDownloads int
	DownloadInterval       time.Duration
	MaxRetryAttempts       int
	RetryDelayBase         time.Duration
}

type HLSDownloader struct {
	config    Config
	storage   *storage.FileManager
	parser    *parser.M3U8Parser
	downloaded map[string]bool
}

func New() *HLSDownloader {
	config := Config{
		MaxConcurrentDownloads: 8,
		DownloadInterval:       5 * time.Second,
		MaxRetryAttempts:       3,
		RetryDelayBase:         time.Second,
	}

	return &HLSDownloader{
		config:    config,
		storage:   storage.NewFileManager(),
		parser:    parser.NewM3U8Parser(),
		downloaded: make(map[string]bool),
	}
}

func (d *HLSDownloader) Start(m3u8URL string) error {
	outputDir, err := d.deriveOutputDir(m3u8URL)
	if err != nil {
		return fmt.Errorf("无法确定下载目录: %w", err)
	}

	log.Printf("开始循环下载 HLS 流: %s", m3u8URL)
	log.Printf("媒体片段保存目录: %s", outputDir)

	return d.loopDownloadHLS(m3u8URL, outputDir)
}

// loopDownloadHLS 主循环：不断刷新M3U8并下载新分片
func (d *HLSDownloader) loopDownloadHLS(m3u8URL, tempDir string) error {
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("创建保存目录失败: %w", err)
	}

	for {
		if err := d.processM3U8(m3u8URL, tempDir); err != nil {
			log.Printf("处理 M3U8 文件时发生错误: %v，将在 %v 后重试", err, d.config.DownloadInterval)
		}
		time.Sleep(d.config.DownloadInterval)
	}
}

// processM3U8 处理 M3U8 文件
func (d *HLSDownloader) processM3U8(m3u8URL, tempDir string) error {
	content, err := utils.HTTPGet(m3u8URL)
	if err != nil {
		return fmt.Errorf("下载 M3U8 文件失败: %w", err)
	}

	playlist, err := d.parser.Parse(content, m3u8URL)
	if err != nil {
		return fmt.Errorf("解析 M3U8 失败: %w", err)
	}

	if playlist.IsMaster {
		if len(playlist.URLs) == 0 {
			return fmt.Errorf("主播放列表中未找到媒体列表")
		}
		selectedMediaURL := playlist.URLs[0]
		log.Printf("发现主播放列表，切换到媒体列表: %s", selectedMediaURL)
		return d.processM3U8(selectedMediaURL, tempDir)
	}

	newTSURLs := d.filterNewSegments(playlist.URLs, playlist.MediaSequence)
	if len(newTSURLs) == 0 {
		log.Printf("未发现新片段，等待下次检查")
		return nil
	}

	log.Printf("发现 %d 个新片段，开始下载", len(newTSURLs))
	if err := d.concurrentDownload(newTSURLs, tempDir); err != nil {
		return fmt.Errorf("并发下载新 TS 文件失败: %w", err)
	}

	return nil
}

// filterNewSegments 提取尚未下载的片段
func (d *HLSDownloader) filterNewSegments(tsURLs []string, mediaSeq int) []string {
	if len(tsURLs) == 0 {
		return nil
	}

	var newURLs []string
	var stats = struct {
		invalidURL, invalidName, downloaded int
	}{}

	for index, urlStr := range tsURLs {
		segmentID, skip := d.processSegmentURL(urlStr, mediaSeq, index, &stats)
		if skip {
			continue
		}

		if d.downloaded[segmentID] {
			stats.downloaded++
			continue
		}

		newURLs = append(newURLs, urlStr)
		d.downloaded[segmentID] = true
	}

	log.Printf("片段过滤完成: 总计%d个, 新增%d个, 无效URL%d个, 无效文件名%d个, 已下载%d个",
		len(tsURLs), len(newURLs), stats.invalidURL, stats.invalidName, stats.downloaded)

	return newURLs
}

// processSegmentURL 处理单个片段URL
func (d *HLSDownloader) processSegmentURL(urlStr string, mediaSeq, index int, stats *struct{ invalidURL, invalidName, downloaded int }) (string, bool) {
	segmentID, err := d.parser.ExtractSegmentID(urlStr, mediaSeq, index)
	if err != nil {
		log.Printf("无效URL已跳过 [索引%d]: %s, 错误: %v", index, urlStr, err)
		stats.invalidURL++
		return "", true
	}

	if segmentID == "" {
		log.Printf("无效文件名已跳过 [索引%d]: %s", index, urlStr)
		stats.invalidName++
		return "", true
	}

	return segmentID, false
}

// concurrentDownload 并发下载新片段
func (d *HLSDownloader) concurrentDownload(tsURLs []string, tempDir string) error {
	ctx := context.Background()
	return d.storage.ConcurrentDownload(ctx, tsURLs, tempDir, d.config.MaxConcurrentDownloads, d.config.MaxRetryAttempts)
}

// deriveOutputDir 推导输出目录
func (d *HLSDownloader) deriveOutputDir(hlsURL string) (string, error) {
	return d.storage.DeriveOutputDir(hlsURL)
}