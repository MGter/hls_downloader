# HLS下载器修订文档

## 修订日期：2026-04-16

## 修订版本：v1.1.0

## 修复的问题

### 1. URL查询参数丢失问题

**问题描述：**
当输入的M3U8 URL包含查询参数时（如 `http://192.166.62.37:7070/record/vod.m3u8?starttime=1776214800&endtime=1776215400`），程序在解析子播放列表和TS片段URL时会丢失这些参数，导致下载失败或下载到错误的内容。

**原因分析：**
- 原代码在处理嵌套M3U8时，使用当前URL作为baseURL，而非原始输入URL
- 当master playlist指向media playlist时，media playlist URL没有继承原始查询参数

**修复方案：**
- 在 `HLSDownloader` 结构体中新增 `originalURL` 字段，保存用户输入的完整URL
- 在解析M3U8时，始终使用 `originalURL` 作为baseURL进行相对路径解析
- 在 `parseRelativeURL` 函数中，确保子URL继承基础URL的查询参数

**修改文件：**
- `internal/downloader/downloader.go`: 新增 `originalURL` 字段，修改解析逻辑
- `internal/parser/m3u8_parser.go`: 优化 `parseRelativeURL` 参数继承逻辑

---

### 2. Master Playlist嵌套问题

**问题描述：**
当M3U8是master playlist（包含多个子播放列表选项）时，解析出的media playlist URL没有正确继承父URL的查询参数，导致后续TS片段下载失败。

**原因分析：**
- Master playlist中的子播放列表URL通常是相对路径
- 解析这些URL时需要正确继承原始URL的scheme、host和query参数

**修复方案：**
- 确保 `parseRelativeURL` 在解析相对URL时继承查询参数
- 递归处理media playlist时，继续使用 `originalURL` 确保参数一致性

---

### 3. 点播文件(VOD)无法正常结束问题

**问题描述：**
当下载的是点播文件（VOD，即完整的视频文件而非直播流）时，程序在下载完成后仍会无限循环等待新片段，不会自动停止。

**原因分析：**
- 原代码只设计用于直播流，没有检测点播文件的结束标签
- HLS点播文件包含 `#EXT-X-ENDLIST` 标签表示播放列表结束

**修复方案：**
- 在 `Playlist` 结构体中新增 `IsVOD` 字段
- 解析时检测 `#EXT-X-ENDLIST` 标签判断是否为点播文件
- 主循环中：点播文件下载完成后自动停止，跳过下载间隔等待

---

## 代码变更摘要

### internal/parser/m3u8_parser.go

```go
// Playlist结构体新增字段
type Playlist struct {
    URLs          []string
    IsMaster      bool
    IsVOD         bool      // 新增：是否是点播文件
    MediaSequence int
}

// Parse函数新增VOD检测
isVOD := strings.Contains(content, "#EXT-X-ENDLIST")

// parseRelativeURL优化参数继承逻辑
// 确保子URL继承基础URL的查询参数
if baseURL.RawQuery != "" && finalURL.RawQuery == "" {
    finalURL.RawQuery = baseURL.RawQuery
}
```

### internal/downloader/downloader.go

```go
// HLSDownloader结构体新增字段
type HLSDownloader struct {
    ...
    originalURL string  // 新增：保存原始输入URL（包含查询参数）
}

// Start函数保存原始URL
d.originalURL = m3u8URL

// loopDownloadHLS新增VOD处理逻辑
var isVOD bool
var vodCompleted bool
// 点播文件下载完成后自动停止
if vodCompleted {
    log.Printf("点播文件下载完成，自动停止")
    return nil
}

// processM3U8WithContext返回Playlist以判断是否为VOD
func (d *HLSDownloader) processM3U8WithContext(...) (*parser.Playlist, error)
```

---

## 使用说明

修改后的程序支持以下场景：

1. **带查询参数的URL**
   ```
   hls_downloader "http://example.com/vod.m3u8?starttime=xxx&endtime=xxx"
   ```
   所有子播放列表和TS片段都会正确继承查询参数。

2. **Master Playlist嵌套**
   ```
   hls_downloader "http://example.com/master.m3u8?token=abc"
   ```
   程序会自动选择第一个media playlist，并保持参数继承。

3. **点播文件**
   ```
   hls_downloader "http://example.com/vod.m3u8"
   ```
   点播文件下载完成后程序会自动停止，无需手动Ctrl+C。

---

## 测试建议

使用以下命令测试修复效果：

```bash
# 测试带参数的URL
./bin/hls_downloader "http://192.166.62.37:7070/record/vod.m3u8?starttime=1776214800&endtime=1776215400"

# 测试master playlist
./bin/hls_downloader "http://example.com/master.m3u8?token=xxx"

# 测试点播文件
./bin/hls_downloader "http://example.com/complete_video.m3u8"
```