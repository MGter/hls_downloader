package main
 
import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)
 
const (
	MaxConcurrentDownloads = 8
	DownloadInterval       = 5 * time.Second
	MaxRetryAttempts       = 3
	RetryDelayBase         = time.Second
)
 
var (
	downloadedSegments   map[string]bool
	downloadedMutex      sync.RWMutex
	segmentNumberRegex   = regexp.MustCompile(`(\d+)$`)
	mediaSequenceRegex   = regexp.MustCompile(`#EXT-X-MEDIA-SEQUENCE:(\d+)`)
)
 
// loopDownloadHLS 主循环：不断刷新M3U8并下载新分片
func loopDownloadHLS(m3u8URL, tempDir string) error {
	log.Printf("开始循环下载 HLS 流: %s", m3u8URL)
	log.Printf("媒体片段保存目录: %s", tempDir)
 
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("创建保存目录失败: %w", err)
	}
 
	for {
		if err := processM3U8(m3u8URL, tempDir); err != nil {
			log.Printf("处理 M3U8 文件时发生错误: %v，将在 %v 后重试", err, DownloadInterval)
		}
		time.Sleep(DownloadInterval)
	}
}
 
// processM3U8 处理 M3U8 文件
func processM3U8(m3u8URL, tempDir string) error {
	resp, err := http.Get(m3u8URL)
	if err != nil {
		return fmt.Errorf("下载 M3U8 文件失败: %w", err)
	}
	defer resp.Body.Close()
 
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("M3U8 下载返回非 200 状态码: %d", resp.StatusCode)
	}
 
	urls, isMasterPlaylist, mediaSeq, err := parseM3U8(resp.Body, m3u8URL)
	if err != nil {
		return fmt.Errorf("解析 M3U8 失败: %w", err)
	}
 
	if len(urls) == 0 {
		return fmt.Errorf("M3U8 中未找到任何有效链接")
	}
 
	if isMasterPlaylist {
		if len(urls) == 0 {
			return fmt.Errorf("主播放列表中未找到媒体列表")
		}
		selectedMediaURL := urls[0]
		log.Printf("发现主播放列表，切换到媒体列表: %s", selectedMediaURL)
		return processM3U8(selectedMediaURL, tempDir)
	}
 
	newTSURLs := filterNewSegments(urls, mediaSeq)
	if len(newTSURLs) == 0 {
		log.Printf("未发现新片段，等待下次检查")
		return nil
	}
 
	log.Printf("发现 %d 个新片段，开始下载", len(newTSURLs))
	if err := concurrentDownloadNew(newTSURLs, tempDir); err != nil {
		return fmt.Errorf("并发下载新 TS 文件失败: %w", err)
	}
 
	return nil
}
 
// parseM3U8 解析 M3U8 文件
func parseM3U8(body io.Reader, baseURL string) ([]string, bool, int, error) {
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, false, 0, fmt.Errorf("读取 M3U8 内容失败: %w", err)
	}
	content := string(bodyBytes)
 
	isMasterPlaylist := strings.Contains(content, "#EXT-X-STREAM-INF")
	isMediaPlaylist := strings.Contains(content, "#EXTINF")
 
	if isMasterPlaylist && isMediaPlaylist {
		log.Printf("警告: M3U8 文件同时包含 Master/Media 标签，按 Media 列表处理")
		isMasterPlaylist = false
	} else if !isMasterPlaylist && !isMediaPlaylist {
		return nil, false, 0, fmt.Errorf("无法识别 M3U8 列表类型")
	}
 
	mediaSeq := extractMediaSequence(content)
 
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, false, 0, fmt.Errorf("解析基础 URL 失败: %w", err)
	}
 
	urls, err := extractURLsFromContent(content, base)
	if err != nil {
		return nil, false, 0, err
	}
 
	return urls, isMasterPlaylist, mediaSeq, nil
}
 
// extractMediaSequence 从M3U8内容中提取媒体序列号
func extractMediaSequence(content string) int {
	match := mediaSequenceRegex.FindStringSubmatch(content)
	if len(match) > 1 {
		if seq, err := strconv.Atoi(match[1]); err == nil {
			return seq
		}
	}
	return 0
}
 
// extractURLsFromContent 从M3U8内容中提取URL
func extractURLsFromContent(content string, baseURL *url.URL) ([]string, error) {
	var urls []string
	scanner := bufio.NewScanner(strings.NewReader(content))
 
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
 
		parsedURL, err := parseRelativeURL(line, baseURL)
		if err != nil {
			log.Printf("警告: 无法解析 URL '%s': %v", line, err)
			continue
		}
 
		urls = append(urls, parsedURL)
	}
 
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("扫描 M3U8 内容失败: %w", err)
	}
 
	return urls, nil
}
 
// parseRelativeURL 解析相对URL
func parseRelativeURL(relativeURL string, baseURL *url.URL) (string, error) {
	u, err := url.Parse(relativeURL)
	if err != nil {
		return "", err
	}
 
	finalURL := baseURL.ResolveReference(u)
 
	// 如果分片没有query但base有query，则继承它
	if finalURL.RawQuery == "" && baseURL.RawQuery != "" {
		finalURL.RawQuery = baseURL.RawQuery
	}
 
	return finalURL.String(), nil
}
 
// extractSegmentNumber 从文件名中提取末尾的数字部分
func extractSegmentNumber(name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}
 
	match := segmentNumberRegex.FindStringSubmatch(name)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}
 
// generateSegmentID 生成唯一的片段标识符
func generateSegmentID(baseNameNoExt string, mediaSeq, index int) string {
	if numStr := extractSegmentNumber(baseNameNoExt); numStr != "" {
		if _, err := strconv.Atoi(numStr); err == nil {
			return numStr
		}
	}
	return fmt.Sprintf("%d_%d", mediaSeq, index)
}
 
// filterNewSegments 提取尚未下载的片段
func filterNewSegments(tsURLs []string, mediaSeq int) []string {
	if len(tsURLs) == 0 {
		return nil
	}
 
	downloadedMutex.Lock()
	defer downloadedMutex.Unlock()
 
	if downloadedSegments == nil {
		downloadedSegments = make(map[string]bool)
	}
 
	var newURLs []string
	var stats = struct {
		invalidURL, invalidName, downloaded int
	}{}
 
	for index, urlStr := range tsURLs {
		segmentID, skip := processSegmentURL(urlStr, mediaSeq, index, &stats)
		if skip {
			continue
		}
 
		if downloadedSegments[segmentID] {
			stats.downloaded++
			continue
		}
 
		newURLs = append(newURLs, urlStr)
		downloadedSegments[segmentID] = true
	}
 
	logFilteringStats(len(tsURLs), len(newURLs), stats)
	return newURLs
}
 
// processSegmentURL 处理单个片段URL
func processSegmentURL(urlStr string, mediaSeq, index int, stats *struct{ invalidURL, invalidName, downloaded int }) (string, bool) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		log.Printf("无效URL已跳过 [索引%d]: %s, 错误: %v", index, urlStr, err)
		stats.invalidURL++
		return "", true
	}
 
	baseName := path.Base(parsedURL.Path)
	if baseName == "" || baseName == "." || baseName == "/" {
		log.Printf("无效文件名已跳过 [索引%d]: %s", index, urlStr)
		stats.invalidName++
		return "", true
	}
 
	baseNameNoExt := strings.TrimSuffix(baseName, path.Ext(baseName))
	return generateSegmentID(baseNameNoExt, mediaSeq, index), false
}
 
// logFilteringStats 记录过滤统计信息
func logFilteringStats(total, newCount int, stats struct{ invalidURL, invalidName, downloaded int }) {
	log.Printf("片段过滤完成: 总计%d个, 新增%d个, 无效URL%d个, 无效文件名%d个, 已下载%d个",
		total, newCount, stats.invalidURL, stats.invalidName, stats.downloaded)
}
 
// concurrentDownloadNew 并发下载新片段
func concurrentDownloadNew(tsURLs []string, tempDir string) error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, MaxConcurrentDownloads)
	errChan := make(chan error, len(tsURLs))
 
	for i, tsURL := range tsURLs {
		wg.Add(1)
		sem <- struct{}{}
 
		go func(index int, currentURL string) {
			defer wg.Done()
			defer func() { <-sem }()
 
			filename, err := generateFilename(currentURL, tempDir, index)
			if err != nil {
				errChan <- fmt.Errorf("生成文件名失败 [%s]: %w", currentURL, err)
				return
			}
 
			if err := downloadFileWithRetry(currentURL, filename); err != nil {
				errChan <- fmt.Errorf("下载失败 [%s]: %w", currentURL, err)
				return
			}
 
			log.Printf("下载完成: %s", path.Base(filename))
		}(i, tsURL)
	}
 
	wg.Wait()
	close(errChan)
 
	for err := range errChan {
		return err
	}
 
	return nil
}
 
// generateFilename 生成唯一的文件名
func generateFilename(tsURL, tempDir string, index int) (string, error) {
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
func downloadFileWithRetry(fileURL, filepath string) error {
	for i := 0; i < MaxRetryAttempts; i++ {
		if err := downloadSingleFile(fileURL, filepath); err == nil {
			return nil
		}
		if i < MaxRetryAttempts-1 {
			delay := RetryDelayBase * time.Duration(i+1)
			time.Sleep(delay)
		}
	}
	return fmt.Errorf("达到最大重试次数: %s", fileURL)
}
 
// downloadSingleFile 下载单个文件
func downloadSingleFile(fileURL, filepath string) error {
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
 
// deriveOutputDir 推导输出目录
func deriveOutputDir(hlsURL string) (string, error) {
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
 
// printHelp 显示帮助信息
func printHelp() {
	app := path.Base(os.Args[0])
	fmt.Printf("HLS 直播流下载器\n\n")
	fmt.Printf("用法: %s <M3U8_URL>\n\n", app)
	fmt.Printf("示例: %s https://example.com/live/stream/playlist.m3u8\n", app)
}
 
func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}
 
	hlsURL := os.Args[1]
	outputDir, err := deriveOutputDir(hlsURL)
	if err != nil {
		log.Fatalf("错误: 无法确定下载目录: %v", err)
	}
 
	if err := loopDownloadHLS(hlsURL, outputDir); err != nil {
		log.Fatalf("下载器意外退出: %v", err)
	}
}
