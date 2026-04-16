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

// PlaylistType 播放列表类型枚举
type PlaylistType string

const (
	PlaylistTypeNone   PlaylistType = ""      // 未指定类型（直播）
	PlaylistTypeEvent  PlaylistType = "EVENT" // 事件直播
	PlaylistTypeVOD    PlaylistType = "VOD"   // 点播
)

// Segment 媒体片段结构体，包含详细的片段信息
type Segment struct {
	URL           string     // 片段URL
	Duration      float64    // 片段时长（秒），从 #EXTINF 提取
	ByteRange     *ByteRange // 字节范围（可选），从 #EXT-X-BYTERANGE 提取
	Discontinuity bool       // 是否有不连续性标记
	Title         string     // 片段标题（可选）
}

// ByteRange 字节范围结构体
type ByteRange struct {
	Length int64 // 字节长度
	Offset int64 // 起始偏移（可选，-1表示使用隐式偏移）
}

// MapInfo 初始化片段信息（用于fMP4格式）
type MapInfo struct {
	URL       string     // 初始化片段URL
	ByteRange *ByteRange // 字节范围（可选）
}

// Playlist 播放列表结构体，存储解析结果
type Playlist struct {
	Segments       []Segment    // 媒体片段列表（包含详细信息）
	URLs           []string     // 提取出的所有URL（简化版本，兼容旧逻辑）
	IsMaster       bool         // 是否是主播放列表（master playlist）
	IsVOD          bool         // 是否是点播文件（包含 #EXT-X-ENDLIST 或 PLAYLIST-TYPE:VOD）
	IsEvent        bool         // 是否是事件直播（PLAYLIST-TYPE:EVENT）
	Version        int          // HLS协议版本号
	TargetDuration float64      // 最大片段时长（秒）
	MediaSequence  int          // 媒体序列号，用于片段排序
	PlaylistType   PlaylistType // 播放列表类型
	MapInfo        *MapInfo     // 初始化片段信息（fMP4）
}

// M3U8Parser M3U8文件解析器
type M3U8Parser struct {
	segmentNumberRegex  *regexp.Regexp // 正则表达式：从文件名提取数字
	mediaSequenceRegex  *regexp.Regexp // 正则表达式：提取媒体序列号
	targetDurationRegex *regexp.Regexp // 正则表达式：提取目标时长
	versionRegex        *regexp.Regexp // 正则表达式：提取协议版本
	playlistTypeRegex   *regexp.Regexp // 正则表达式：提取播放列表类型
	extinfRegex         *regexp.Regexp // 正则表达式：提取片段时长和标题
	byteRangeRegex      *regexp.Regexp // 正则表达式：提取字节范围
	mapUriRegex         *regexp.Regexp // 正则表达式：提取初始化片段URI
}

// NewM3U8Parser 创建新的解析器
func NewM3U8Parser() *M3U8Parser {
	// 编译所有正则表达式，用于后续匹配
	return &M3U8Parser{
		// 匹配文件名末尾的数字，例如 "segment123.ts" 中的 "123"
		segmentNumberRegex: regexp.MustCompile(`(\d+)$`),
		// 匹配 M3U8 文件中的媒体序列号标签
		mediaSequenceRegex: regexp.MustCompile(`#EXT-X-MEDIA-SEQUENCE:(\d+)`),
		// 匹配目标时长标签
		targetDurationRegex: regexp.MustCompile(`#EXT-X-TARGETDURATION:(\d+)`),
		// 匹配协议版本标签
		versionRegex: regexp.MustCompile(`#EXT-X-VERSION:(\d+)`),
		// 匹配播放列表类型标签
		playlistTypeRegex: regexp.MustCompile(`#EXT-X-PLAYLIST-TYPE:(EVENT|VOD)`),
		// 匹配片段时长标签（支持浮点数）
		extinfRegex: regexp.MustCompile(`#EXTINF:([\d.]+),?(.*)`),
		// 匹配字节范围标签
		byteRangeRegex: regexp.MustCompile(`#EXT-X-BYTERANGE:(\d+)(?:@(\d+))?`),
		// 匹配初始化片段URI
		mapUriRegex: regexp.MustCompile(`#EXT-X-MAP:URI="([^"]+)"`),
	}
}

// Parse 解析M3U8文件内容
func (p *M3U8Parser) Parse(content, baseURL string) (*Playlist, error) {
	// 检查是否是有效的M3U8文件（必须以 #EXTM3U 或 #EXTM3U8 开头）
	if !strings.HasPrefix(strings.TrimSpace(content), "#EXTM3U") {
		return nil, fmt.Errorf("不是有效的 M3U8 文件（缺少 #EXTM3U 标签）")
	}

	// 判断播放列表类型
	isMasterPlaylist := strings.Contains(content, "#EXT-X-STREAM-INF") // 包含主列表标签
	isMediaPlaylist := strings.Contains(content, "#EXTINF")            // 包含媒体列表标签
	hasEndList := strings.Contains(content, "#EXT-X-ENDLIST")          // 点播结束标签

	// 处理特殊情况
	if isMasterPlaylist && isMediaPlaylist {
		// 如果同时包含两种标签，按媒体列表处理
		fmt.Printf("警告: M3U8 文件同时包含 Master/Media 标签，按 Media 列表处理")
		isMasterPlaylist = false
	} else if !isMasterPlaylist && !isMediaPlaylist {
		// 如果没有识别到任何标签，返回错误
		return nil, fmt.Errorf("无法识别 M3U8 列表类型")
	}

	// 提取各种元数据
	mediaSeq := p.extractMediaSequence(content)
	version := p.extractVersion(content)
	targetDur := p.extractTargetDuration(content)
	playlistType := p.extractPlaylistType(content)
	mapInfo := p.extractMapInfo(content, baseURL)

	// 判断是否是点播或事件流
	// 1. 有 #EXT-X-ENDLIST 标签 → VOD
	// 2. PLAYLIST-TYPE:VOD → VOD
	// 3. PLAYLIST-TYPE:EVENT → EVENT（可追加新片段）
	isVOD := hasEndList || playlistType == PlaylistTypeVOD
	isEvent := playlistType == PlaylistTypeEvent && !hasEndList

	// 解析基础URL，用于后续相对路径转换
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("解析基础 URL 失败: %w", err)
	}

	// 从内容中提取片段信息（详细信息）
	segments, urls, err := p.extractSegmentsFromContent(content, base)
	if err != nil {
		return nil, err
	}

	// 返回解析结果
	return &Playlist{
		Segments:       segments,
		URLs:           urls,
		IsMaster:       isMasterPlaylist,
		IsVOD:          isVOD,
		IsEvent:        isEvent,
		Version:        version,
		TargetDuration: targetDur,
		MediaSequence:  mediaSeq,
		PlaylistType:   playlistType,
		MapInfo:        mapInfo,
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

// extractVersion 从M3U8内容中提取HLS协议版本
func (p *M3U8Parser) extractVersion(content string) int {
	match := p.versionRegex.FindStringSubmatch(content)
	if len(match) > 1 {
		if ver, err := strconv.Atoi(match[1]); err == nil {
			return ver
		}
	}
	return 1 // 默认版本1
}

// extractTargetDuration 从M3U8内容中提取目标时长
func (p *M3U8Parser) extractTargetDuration(content string) float64 {
	match := p.targetDurationRegex.FindStringSubmatch(content)
	if len(match) > 1 {
		if dur, err := strconv.ParseFloat(match[1], 64); err == nil {
			return dur
		}
	}
	return 0
}

// extractPlaylistType 从M3U8内容中提取播放列表类型
func (p *M3U8Parser) extractPlaylistType(content string) PlaylistType {
	match := p.playlistTypeRegex.FindStringSubmatch(content)
	if len(match) > 1 {
		return PlaylistType(match[1])
	}
	return PlaylistTypeNone
}

// extractMapInfo 从M3U8内容中提取初始化片段信息（fMP4）
func (p *M3U8Parser) extractMapInfo(content string, baseURL string) *MapInfo {
	// 检查是否有 #EXT-X-MAP 标签
	if !strings.Contains(content, "#EXT-X-MAP") {
		return nil
	}

	// 提取 URI
	match := p.mapUriRegex.FindStringSubmatch(content)
	if len(match) < 2 {
		return nil
	}

	// 解析基础URL
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	// 解析相对URL为绝对URL
	absoluteURL, err := p.parseRelativeURL(match[1], base)
	if err != nil {
		return nil
	}

	// 提取字节范围（可选）
	byteRange := p.extractByteRangeFromLine(content)

	return &MapInfo{
		URL:       absoluteURL,
		ByteRange: byteRange,
	}
}

// extractByteRangeFromLine 从行中提取字节范围信息
func (p *M3U8Parser) extractByteRangeFromLine(line string) *ByteRange {
	match := p.byteRangeRegex.FindStringSubmatch(line)
	if len(match) < 2 {
		return nil
	}

	length, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return nil
	}

	br := &ByteRange{Length: length, Offset: -1} // -1 表示隐式偏移
	if len(match) > 2 && match[2] != "" {
		offset, err := strconv.ParseInt(match[2], 10, 64)
		if err == nil {
			br.Offset = offset
		}
	}

	return br
}

// extractSegmentsFromContent 从M3U8内容中提取片段信息（详细信息）
func (p *M3U8Parser) extractSegmentsFromContent(content string, baseURL *url.URL) ([]Segment, []string, error) {
	var segments []Segment
	var urls []string

	scanner := bufio.NewScanner(strings.NewReader(content))

	var currentDuration float64
	var currentTitle string
	var currentByteRange *ByteRange
	var hasDiscontinuity bool

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 处理 #EXTINF 标签
		if strings.HasPrefix(line, "#EXTINF:") {
			match := p.extinfRegex.FindStringSubmatch(line)
			if len(match) >= 2 {
				if dur, err := strconv.ParseFloat(match[1], 64); err == nil {
					currentDuration = dur
					// 打印切片时长日志
					fmt.Printf("读取切片时长: %.2f 秒\n", dur)
				}
				if len(match) >= 3 {
					currentTitle = strings.TrimSpace(match[2])
				}
			}
			continue
		}

		// 处理 #EXT-X-BYTERANGE 标签
		if strings.HasPrefix(line, "#EXT-X-BYTERANGE:") {
			currentByteRange = p.extractByteRangeFromLine(line)
			continue
		}

		// 处理 #EXT-X-DISCONTINUITY 标签
		if line == "#EXT-X-DISCONTINUITY" {
			hasDiscontinuity = true
			continue
		}

		// 跳过其他注释行和空行
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// 这是URL行，创建片段
		parsedURL, err := p.parseRelativeURL(line, baseURL)
		if err != nil {
			fmt.Printf("警告: 无法解析 URL '%s': %v\n", line, err)
			continue
		}

		segment := Segment{
			URL:           parsedURL,
			Duration:      currentDuration,
			ByteRange:     currentByteRange,
			Discontinuity: hasDiscontinuity,
			Title:         currentTitle,
		}

		segments = append(segments, segment)
		urls = append(urls, parsedURL)

		// 重置状态（字节范围需要使用隐式偏移）
		currentDuration = 0
		currentTitle = ""
		if currentByteRange != nil {
			// 下一个片段的字节范围将使用隐式偏移
			currentByteRange = &ByteRange{Length: 0, Offset: -1}
		}
		hasDiscontinuity = false
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("扫描 M3U8 内容失败: %w", err)
	}

	return segments, urls, nil
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
		return match[1] // 返回匹配到的数字部分
	}
	return "" // 没有匹配到数字
}

// generateSegmentID 生成唯一的片段标识符
func (p *M3U8Parser) generateSegmentID(baseNameNoExt string, mediaSeq, index int) string {
	// 首先尝试从文件名中提取数字
	if numStr := p.extractSegmentNumber(baseNameNoExt); numStr != "" {
		// 验证提取到的是有效数字
		if _, err := strconv.Atoi(numStr); err == nil {
			return numStr // 使用文件名中的数字作为ID
		}
	}
	// 如果文件名中没有数字，使用"媒体序列号_索引"的格式
	return fmt.Sprintf("%d_%d", mediaSeq, index)
}