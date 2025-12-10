package downloader  // 下载器包，负责HLS流下载的核心逻辑

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

// Config 下载器配置参数
type Config struct {
	MaxConcurrentDownloads int           // 最大并发下载数，同时下载几个文件
	DownloadInterval       time.Duration // 检查新片段的时间间隔
	MaxRetryAttempts       int           // 下载失败时的最大重试次数
	RetryDelayBase         time.Duration // 重试前的等待时间
}

// HLSDownloader HLS下载器结构体
type HLSDownloader struct {
	config    Config                  // 配置参数
	storage   *storage.FileManager    // 文件管理器，负责保存文件
	parser    *parser.M3U8Parser      // M3U8解析器，解析播放列表
	downloaded map[string]bool        // 记录已下载的片段，避免重复下载
}

// New 创建下载器实例
func New() *HLSDownloader {
	// 设置默认配置
	config := Config{
		MaxConcurrentDownloads: 8,           // 同时下载8个文件
		DownloadInterval:       5 * time.Second,  // 每5秒检查一次
		MaxRetryAttempts:       3,           // 最多重试3次
		RetryDelayBase:         time.Second, // 重试前等待1秒
	}

	// 创建并返回下载器对象
	return &HLSDownloader{
		config:    config,
		storage:   storage.NewFileManager(),  // 初始化文件管理器
		parser:    parser.NewM3U8Parser(),    // 初始化解析器
		downloaded: make(map[string]bool),    // 初始化已下载记录（空map）
	}
}

// Start 开始下载流程
func (d *HLSDownloader) Start(m3u8URL string) error {
	// 根据URL生成保存文件的目录名
	outputDir, err := d.deriveOutputDir(m3u8URL)
	if err != nil {
		return fmt.Errorf("无法确定下载目录: %w", err)
	}

	// 打印开始信息
	log.Printf("开始循环下载 HLS 流: %s", m3u8URL)
	log.Printf("媒体片段保存目录: %s", outputDir)

	// 进入主循环，开始不断下载
	return d.loopDownloadHLS(m3u8URL, outputDir)
}

// loopDownloadHLS 主循环：不断检查并下载新片段
func (d *HLSDownloader) loopDownloadHLS(m3u8URL, tempDir string) error {
	// 创建保存目录，权限0755表示：所有者可读写执行，其他人可读执行
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("创建保存目录失败: %w", err)
	}

	// 无限循环，直到程序被停止
	for {
		// 处理M3U8文件，检查并下载新片段
		if err := d.processM3U8(m3u8URL, tempDir); err != nil {
			// 如果出错，等待后重试
			log.Printf("处理 M3U8 文件时发生错误: %v，将在 %v 后重试", err, d.config.DownloadInterval)
		}
		// 等待指定时间再检查一次
		time.Sleep(d.config.DownloadInterval)
	}
}

// processM3U8 处理M3U8文件的主要逻辑
func (d *HLSDownloader) processM3U8(m3u8URL, tempDir string) error {
	// 步骤1：下载M3U8文件内容
	content, err := utils.HTTPGet(m3u8URL)
	if err != nil {
		return fmt.Errorf("下载 M3U8 文件失败: %w", err)
	}

	// 步骤2：解析M3U8内容
	playlist, err := d.parser.Parse(content, m3u8URL)
	if err != nil {
		return fmt.Errorf("解析 M3U8 失败: %w", err)
	}

	// 步骤3：如果是主播放列表（包含多个子播放列表）
	if playlist.IsMaster {
		// 检查是否有媒体播放列表
		if len(playlist.URLs) == 0 {
			return fmt.Errorf("主播放列表中未找到媒体列表")
		}
		// 选择第一个媒体播放列表继续处理
		selectedMediaURL := playlist.URLs[0]
		log.Printf("发现主播放列表，切换到媒体列表: %s", selectedMediaURL)
		// 递归处理媒体播放列表
		return d.processM3U8(selectedMediaURL, tempDir)
	}

	// 步骤4：过滤出新的片段（还没下载过的）
	newTSURLs := d.filterNewSegments(playlist.URLs, playlist.MediaSequence)
	if len(newTSURLs) == 0 {
		log.Printf("未发现新片段，等待下次检查")
		return nil  // 没有新片段，直接返回
	}

	// 步骤5：并发下载新片段
	log.Printf("发现 %d 个新片段，开始下载", len(newTSURLs))
	if err := d.concurrentDownload(newTSURLs, tempDir); err != nil {
		return fmt.Errorf("并发下载新 TS 文件失败: %w", err)
	}

	return nil
}

// filterNewSegments 过滤出新片段（还没下载过的）
func (d *HLSDownloader) filterNewSegments(tsURLs []string, mediaSeq int) []string {
	// 如果没有片段，返回空
	if len(tsURLs) == 0 {
		return nil
	}

	var newURLs []string  // 存储新片段的URL
	
	// 统计信息
	var stats = struct {
		invalidURL, invalidName, downloaded int
	}{}

	// 遍历所有片段URL
	for index, urlStr := range tsURLs {
		// 处理单个片段URL，获取片段ID
		segmentID, skip := d.processSegmentURL(urlStr, mediaSeq, index, &stats)
		if skip {
			continue  // 跳过这个片段
		}

		// 检查是否已下载
		if d.downloaded[segmentID] {
			stats.downloaded++  // 已下载计数
			continue
		}

		// 是新片段，添加到下载列表
		newURLs = append(newURLs, urlStr)
		// 标记为已下载，避免下次重复下载
		d.downloaded[segmentID] = true
	}

	// 打印过滤结果
	log.Printf("片段过滤完成: 总计%d个, 新增%d个, 无效URL%d个, 无效文件名%d个, 已下载%d个",
		len(tsURLs), len(newURLs), stats.invalidURL, stats.invalidName, stats.downloaded)

	return newURLs
}

// processSegmentURL 处理单个片段URL，提取片段ID
func (d *HLSDownloader) processSegmentURL(urlStr string, mediaSeq, index int, stats *struct{ invalidURL, invalidName, downloaded int }) (string, bool) {
	// 从URL中提取片段ID（唯一标识）
	segmentID, err := d.parser.ExtractSegmentID(urlStr, mediaSeq, index)
	if err != nil {
		log.Printf("无效URL已跳过 [索引%d]: %s, 错误: %v", index, urlStr, err)
		stats.invalidURL++  // 无效URL计数
		return "", true     // 返回true表示跳过
	}

	// 检查片段ID是否为空
	if segmentID == "" {
		log.Printf("无效文件名已跳过 [索引%d]: %s", index, urlStr)
		stats.invalidName++  // 无效文件名计数
		return "", true      // 返回true表示跳过
	}

	return segmentID, false  // 返回片段ID，false表示不跳过
}

// concurrentDownload 并发下载多个片段
func (d *HLSDownloader) concurrentDownload(tsURLs []string, tempDir string) error {
	ctx := context.Background()  // 创建上下文
	// 调用存储器的并发下载功能
	return d.storage.ConcurrentDownload(ctx, tsURLs, tempDir, d.config.MaxConcurrentDownloads, d.config.MaxRetryAttempts)
}

// deriveOutputDir 根据URL生成输出目录名
func (d *HLSDownloader) deriveOutputDir(hlsURL string) (string, error) {
	return d.storage.DeriveOutputDir(hlsURL)
}