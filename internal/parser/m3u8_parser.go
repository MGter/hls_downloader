package parser  // 解析器包，负责解析M3U8文件

import (
	"bufio"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Playlist 播放列表结构体，存储解析结果
type Playlist struct {
	URLs          []string  // 提取出的所有URL
	IsMaster      bool      // 是否是主播放列表（master playlist）
	IsVOD         bool      // 是否是点播文件（包含 #EXT-X-ENDLIST）
	MediaSequence int       // 媒体序列号，用于片段排序
}

// M3U8Parser M3U8文件解析器
type M3U8Parser struct {
	segmentNumberRegex *regexp.Regexp  // 正则表达式：从文件名提取数字
	mediaSequenceRegex *regexp.Regexp  // 正则表达式：提取媒体序列号
}

// NewM3U8Parser 创建新的解析器
func NewM3U8Parser() *M3U8Parser {
	// 编译两个正则表达式，用于后续匹配
	return &M3U8Parser{
		// 匹配文件名末尾的数字，例如 "segment123.ts" 中的 "123"
		segmentNumberRegex: regexp.MustCompile(`(\d+)$`),
		// 匹配 M3U8 文件中的媒体序列号标签
		mediaSequenceRegex: regexp.MustCompile(`#EXT-X-MEDIA-SEQUENCE:(\d+)`),
	}
}

// Parse 解析M3U8文件内容
func (p *M3U8Parser) Parse(content, baseURL string) (*Playlist, error) {
	// 判断播放列表类型
	isMasterPlaylist := strings.Contains(content, "#EXT-X-STREAM-INF")  // 包含主列表标签
	isMediaPlaylist := strings.Contains(content, "#EXTINF")             // 包含媒体列表标签
	isVOD := strings.Contains(content, "#EXT-X-ENDLIST")                // 点播文件有结束标签

	// 处理特殊情况
	if isMasterPlaylist && isMediaPlaylist {
		// 如果同时包含两种标签，按媒体列表处理
		fmt.Printf("警告: M3U8 文件同时包含 Master/Media 标签，按 Media 列表处理")
		isMasterPlaylist = false
	} else if !isMasterPlaylist && !isMediaPlaylist {
		// 如果没有识别到任何标签，返回错误
		return nil, fmt.Errorf("无法识别 M3U8 列表类型")
	}

	// 提取媒体序列号（如果存在）
	mediaSeq := p.extractMediaSequence(content)

	// 解析基础URL，用于后续相对路径转换
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("解析基础 URL 失败: %w", err)
	}

	// 从内容中提取所有URL，并确保继承基础URL的查询参数
	urls, err := p.extractURLsFromContent(content, base)
	if err != nil {
		return nil, err
	}

	// 返回解析结果
	return &Playlist{
		URLs:          urls,           // 提取的URL列表
		IsMaster:      isMasterPlaylist, // 播放列表类型
		IsVOD:         isVOD,          // 是否是点播文件
		MediaSequence: mediaSeq,       // 媒体序列号
	}, nil
}

// extractMediaSequence 从M3U8内容中提取媒体序列号
func (p *M3U8Parser) extractMediaSequence(content string) int {
	// 使用正则表达式查找媒体序列号标签
	match := p.mediaSequenceRegex.FindStringSubmatch(content)
	if len(match) > 1 {
		// 将匹配到的字符串转换为整数
		if seq, err := strconv.Atoi(match[1]); err == nil {
			return seq
		}
	}
	// 如果没有找到，返回0
	return 0
}

// extractURLsFromContent 从M3U8内容中提取URL
func (p *M3U8Parser) extractURLsFromContent(content string, baseURL *url.URL) ([]string, error) {
	var urls []string
	// 使用扫描器逐行读取内容
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		// 读取一行并去除空白字符
		line := strings.TrimSpace(scanner.Text())
		
		// 跳过注释行和空行
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// 解析相对URL为绝对URL
		parsedURL, err := p.parseRelativeURL(line, baseURL)
		if err != nil {
			// 如果解析失败，打印警告并继续处理下一行
			fmt.Printf("警告: 无法解析 URL '%s': %v", line, err)
			continue
		}

		// 将解析成功的URL添加到列表
		urls = append(urls, parsedURL)
	}

	// 检查扫描过程中是否有错误
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("扫描 M3U8 内容失败: %w", err)
	}

	return urls, nil
}

// parseRelativeURL 解析相对URL为绝对URL，并确保继承基础URL的查询参数
func (p *M3U8Parser) parseRelativeURL(relativeURL string, baseURL *url.URL) (string, error) {
	// 解析相对URL
	u, err := url.Parse(relativeURL)
	if err != nil {
		return "", err
	}

	// 将相对URL与基础URL合并，得到绝对URL
	finalURL := baseURL.ResolveReference(u)

	// 关键修复：确保子URL继承基础URL的查询参数
	// 如果基础URL有查询参数，且子URL没有自己的查询参数，则继承
	if baseURL.RawQuery != "" && finalURL.RawQuery == "" {
		finalURL.RawQuery = baseURL.RawQuery
	}

	return finalURL.String(), nil
}

// ExtractSegmentID 生成唯一的片段标识符
// 使用 URL 的完整路径（包括查询参数）作为唯一标识，避免重复下载问题
func (p *M3U8Parser) ExtractSegmentID(urlStr string, mediaSeq, index int) (string, error) {
	// 解析URL字符串
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}

	// 从URL路径中提取文件名（不含扩展名）
	baseName := path.Base(parsedURL.Path)
	// 去除文件扩展名
	baseNameNoExt := strings.TrimSuffix(baseName, path.Ext(baseName))

	// 检查文件名是否有效
	if baseNameNoExt == "" || baseNameNoExt == "." || baseNameNoExt == "/" {
		return "", fmt.Errorf("invalid filename")
	}

	// 构建唯一 segmentID：
	// 1. 如果 URL 有查询参数，使用 "文件名_查询参数" 作为唯一标识
	// 2. 如果没有查询参数，尝试从文件名提取数字
	// 3. 如果文件名无数字，使用 "mediaSeq_index" 格式
	if parsedURL.RawQuery != "" {
		// URL 有查询参数时，文件名+查询参数组合作为唯一标识
		// 这样可以区分同一文件名但不同版本的片段
		return fmt.Sprintf("%s_%s", baseNameNoExt, parsedURL.RawQuery), nil
	}

	// 无查询参数时，尝试从文件名提取数字
	if numStr := p.extractSegmentNumber(baseNameNoExt); numStr != "" {
		return numStr, nil
	}

	// 文件名无数字，使用 mediaSeq_index 格式
	return fmt.Sprintf("%d_%d", mediaSeq, index), nil
}

// extractSegmentNumber 从文件名中提取末尾的数字部分
func (p *M3U8Parser) extractSegmentNumber(name string) string {
	// 检查名称是否为空或只有空白字符
	if strings.TrimSpace(name) == "" {
		return ""
	}

	// 使用正则表达式匹配末尾的数字
	match := p.segmentNumberRegex.FindStringSubmatch(name)
	if len(match) > 1 {
		return match[1]  // 返回匹配到的数字部分
	}
	return ""  // 没有匹配到数字
}

// generateSegmentID 生成唯一的片段标识符
func (p *M3U8Parser) generateSegmentID(baseNameNoExt string, mediaSeq, index int) string {
	// 首先尝试从文件名中提取数字
	if numStr := p.extractSegmentNumber(baseNameNoExt); numStr != "" {
		// 验证提取到的是有效数字
		if _, err := strconv.Atoi(numStr); err == nil {
			return numStr  // 使用文件名中的数字作为ID
		}
	}
	// 如果文件名中没有数字，使用"媒体序列号_索引"的格式
	return fmt.Sprintf("%d_%d", mediaSeq, index)
}