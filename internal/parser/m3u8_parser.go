package parser

import (
	"bufio"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

type Playlist struct {
	URLs          []string
	IsMaster      bool
	MediaSequence int
}

type M3U8Parser struct {
	segmentNumberRegex *regexp.Regexp
	mediaSequenceRegex *regexp.Regexp
}

func NewM3U8Parser() *M3U8Parser {
	return &M3U8Parser{
		segmentNumberRegex: regexp.MustCompile(`(\d+)$`),
		mediaSequenceRegex: regexp.MustCompile(`#EXT-X-MEDIA-SEQUENCE:(\d+)`),
	}
}

func (p *M3U8Parser) Parse(content, baseURL string) (*Playlist, error) {
	isMasterPlaylist := strings.Contains(content, "#EXT-X-STREAM-INF")
	isMediaPlaylist := strings.Contains(content, "#EXTINF")

	if isMasterPlaylist && isMediaPlaylist {
		fmt.Printf("警告: M3U8 文件同时包含 Master/Media 标签，按 Media 列表处理")
		isMasterPlaylist = false
	} else if !isMasterPlaylist && !isMediaPlaylist {
		return nil, fmt.Errorf("无法识别 M3U8 列表类型")
	}

	mediaSeq := p.extractMediaSequence(content)

	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("解析基础 URL 失败: %w", err)
	}

	urls, err := p.extractURLsFromContent(content, base)
	if err != nil {
		return nil, err
	}

	return &Playlist{
		URLs:          urls,
		IsMaster:      isMasterPlaylist,
		MediaSequence: mediaSeq,
	}, nil
}

// extractMediaSequence 从M3U8内容中提取媒体序列号
func (p *M3U8Parser) extractMediaSequence(content string) int {
	match := p.mediaSequenceRegex.FindStringSubmatch(content)
	if len(match) > 1 {
		if seq, err := strconv.Atoi(match[1]); err == nil {
			return seq
		}
	}
	return 0
}

// extractURLsFromContent 从M3U8内容中提取URL
func (p *M3U8Parser) extractURLsFromContent(content string, baseURL *url.URL) ([]string, error) {
	var urls []string
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		parsedURL, err := p.parseRelativeURL(line, baseURL)
		if err != nil {
			fmt.Printf("警告: 无法解析 URL '%s': %v", line, err)
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
func (p *M3U8Parser) parseRelativeURL(relativeURL string, baseURL *url.URL) (string, error) {
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

// ExtractSegmentID 生成唯一的片段标识符
func (p *M3U8Parser) ExtractSegmentID(urlStr string, mediaSeq, index int) (string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}

	baseName := path.Base(parsedURL.Path)
	if baseName == "" || baseName == "." || baseName == "/" {
		return "", fmt.Errorf("invalid filename")
	}

	baseNameNoExt := strings.TrimSuffix(baseName, path.Ext(baseName))
	return p.generateSegmentID(baseNameNoExt, mediaSeq, index), nil
}

// extractSegmentNumber 从文件名中提取末尾的数字部分
func (p *M3U8Parser) extractSegmentNumber(name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}

	match := p.segmentNumberRegex.FindStringSubmatch(name)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

// generateSegmentID 生成唯一的片段标识符
func (p *M3U8Parser) generateSegmentID(baseNameNoExt string, mediaSeq, index int) string {
	if numStr := p.extractSegmentNumber(baseNameNoExt); numStr != "" {
		if _, err := strconv.Atoi(numStr); err == nil {
			return numStr
		}
	}
	return fmt.Sprintf("%d_%d", mediaSeq, index)
}