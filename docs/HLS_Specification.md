# HLS (HTTP Live Streaming) 协议规范文档

## 文档版本
- 基于规范: RFC 8216 (HLS)
- 整理日期: 2026-04-16
- 适用版本: HLS v2-v7

---

## 1. HLS 协议概述

HLS (HTTP Live Streaming) 是 Apple 开发的基于 HTTP 的自适应流媒体传输协议。它将媒体内容分割成一系列小的、基于 HTTP 的文件片段（通常为 .ts 或 .fmp4 格式），并通过 M3U8 播放列表文件进行索引管理。

### 1.1 核心组成
- **播放列表文件 (M3U8)**: 素引文件，描述媒体片段的位置和元信息
- **媒体片段文件**: 实际的媒体数据（TS 或 fMP4）
- **初始化片段**: fMP4 格式需要的初始化数据

### 1.2 播放列表类型
| 类型 | 标签 | 描述 |
|------|------|------|
| Master Playlist | `#EXT-X-STREAM-INF` | 包含多个媒体播放列表的引用 |
| Media Playlist | `#EXTINF` | 包含媒体片段的详细列表 |

---

## 2. M3U8 标签分类

### 2.1 基本标签 (Basic Tags)

| 标签 | 格式 | 说明 | 版本要求 |
|------|------|------|----------|
| `#EXTM3U8` | 必须作为文件第一行 | 标识文件为 M3U8 播放列表 | 无 |
| `#EXT-X-VERSION` | `#EXT-X-VERSION:<n>` | 指定协议版本号 | 无 |
| `#EXTINF` | `#EXTINF:<duration>,[<title>]` | 指定片段时长（秒）和可选标题 | 无 |

### 2.2 媒体播放列表标签 (Media Playlist Tags)

| 标签 | 格式 | 说明 | 版本要求 |
|------|------|------|----------|
| `#EXT-X-TARGETDURATION` | `#EXT-X-TARGETDURATION:<s>` | 最大片段时长（秒） | 无 |
| `#EXT-X-MEDIA-SEQUENCE` | `#EXT-X-MEDIA-SEQUENCE:<number>` | 第一个片段的序列号 | 无 |
| `#EXT-X-ENDLIST` | 单独标签 | 标识点播文件结束 | 无 |
| `#EXT-X-PLAYLIST-TYPE` | `#EXT-X-PLAYLIST-TYPE:<type>` | 类型：EVENT 或 VOD | v3+ |
| `#EXT-X-I-FRAMES-ONLY` | 单独标签 | 仅包含 I 帧（用于快进/快退） | v4+ |
| `#EXT-X-ALLOW-CACHE` | `#EXT-X-ALLOW-CACHE:<YES|NO>` | 是否允许缓存（已废弃） | v6- |

### 2.3 媒体片段标签 (Media Segment Tags)

| 标签 | 格式 | 说明 | 版本要求 |
|------|------|------|----------|
| `#EXT-X-BYTERANGE` | `#EXT-X-BYTERANGE:<n>[@<o>]` | 字节范围请求 | v4+ |
| `#EXT-X-DISCONTINUITY` | 单独标签 | 标记不连续点（编码变化等） | v3+ |
| `#EXT-X-KEY` | `#EXT-X-KEY:<attribute-list>` | 加密密钥信息 | 无 |
| `#EXT-X-MAP` | `#EXT-X-MAP:<attribute-list>` | 初始化片段（fMP4） | v5+ |
| `#EXT-X-PROGRAM-DATE-TIME` | `#EXT-X-PROGRAM-DATE-TIME:<datetime>` | 片段的绝对时间 | v3+ |
| `#EXT-X-DATERANGE` | `#EXT-X-DATERANGE:<attribute-list>` | 日期范围 | v7+ |

### 2.4 主播放列表标签 (Master Playlist Tags)

| 标签 | 格式 | 说明 | 版本要求 |
|------|------|------|----------|
| `#EXT-X-MEDIA` | `#EXT-X-MEDIA:<attribute-list>` | 替代媒体轨道 | v4+ |
| `#EXT-X-STREAM-INF` | `#EXT-X-STREAM-INF:<attribute-list>` | 流信息 | 无 |
| `#EXT-X-I-FRAME-STREAM-INF` | `#EXT-X-I-FRAME-STREAM-INF:<attribute-list>` | I帧流信息 | v4+ |
| `#EXT-X-SESSION-DATA` | `#EXT-X-SESSION-DATA:<attribute-list>` | 会话数据 | v7+ |
| `#EXT-X-SESSION-KEY` | `#EXT-X-SESSION-KEY:<attribute-list>` | 会话密钥 | v7+ |

### 2.5 其他标签 (Other Tags)

| 标签 | 格式 | 说明 | 版本要求 |
|------|------|------|----------|
| `#EXT-X-INDEPENDENT-SEGMENTS` | 单独标签 | 所有片段可独立解码 | v1+ |
| `#EXT-X-START` | `#EXT-X-START:<attribute-list>` | 指定播放起始位置 | v1+ |

---

## 3. 协议版本特性详解

### 3.1 Version 1-2
- 基本播放列表结构
- TS 片段支持
- 简单加密支持

### 3.2 Version 3
- 新增 `#EXT-X-PLAYLIST-TYPE` (EVENT/VOD)
- 新增 `#EXT-X-DISCONTINUITY` 不连续标记
- 新增 `#EXT-X-PROGRAM-DATE-TIME`
- 浮点数时长支持

### 3.3 Version 4
- 新增 `#EXT-X-BYTERANGE` 字节范围
- 新增 `#EXT-X-I-FRAMES-ONLY` I帧播放列表
- 新增 `#EXT-X-MEDIA` 替代轨道
- 新增 `#EXT-X-I-FRAME-STREAM-INF`
- Master playlist 多轨道支持

### 3.4 Version 5
- 新增 `#EXT-X-MAP` fMP4 初始化片段
- 支持 fMP4 (Fragmented MP4) 格式
- KEY 中新增 KEYFORMAT 和 KEYFORMATVERSIONS

### 3.5 Version 6
- 废弃 `#EXT-X-ALLOW-CACHE`
- 支持 BYTERANGE 的隐式偏移

### 3.6 Version 7
- 新增 `#EXT-X-DATERANGE` 日期范围
- 新增 `#EXT-X-SESSION-DATA` 会话数据
- 新增 `#EXT-X-SESSION-KEY` 会话密钥
- 支持多密钥加密

---

## 4. 加密支持 (#EXT-X-KEY)

### 4.1 加密方法
| 方法 | 说明 |
|------|------|
| `NONE` | 无加密 |
| `AES-128` | AES-128 位加密 |
| `SAMPLE-AES` | SAMPLE-AES 加密（用于内容保护） |

### 4.2 属性列表
```
#EXT-X-KEY:METHOD=<method>,URI="<uri>",IV=<iv>,KEYFORMAT=<format>,KEYFORMATVERSIONS=<versions>
```

| 属性 | 说明 |
|------|------|
| METHOD | 加密方法（必需） |
| URI | 密钥获取地址 |
| IV | 初始化向量（16字节十六进制） |
| KEYFORMAT | 密钥格式标识 |
| KEYFORMATVERSIONS | 密钥格式版本 |

---

## 5. 初始化片段 (#EXT-X-MAP)

fMP4 格式需要初始化片段，包含解码所需的元数据（如 codec 初始化数据）。

### 5.1 格式
```
#EXT-X-MAP:URI="<uri>",BYTERANGE="<n>[@<o>]"
```

| 属性 | 说明 |
|------|------|
| URI | 初始化片段地址（必需） |
| BYTERANGE | 字节范围（可选） |

---

## 6. 不连续性标记 (#EXT-X-DISCONTINUITY)

用于标记以下情况：
- 文件格式变化
- 编码参数变化
- 时间戳跳跃
- 内容切换

---

## 7. 播放列表类型 (#EXT-X-PLAYLIST-TYPE)

| 类型 | 说明 | 特性 |
|------|------|------|
| VOD | 点播 | 内容固定，不可修改，有 ENDLIST |
| EVENT | 事件直播 | 可追加新片段，但已有片段不可修改 |

---

## 8. 字节范围请求 (#EXT-X-BYTERANGE)

允许从单个 HTTP URL 请求部分内容：
```
#EXT-X-BYTERANGE:1024@0
segment.mp4
#EXT-X-BYTERANGE:1024@1024
segment.mp4
```

---

## 9. 替代媒体轨道 (#EXT-X-MEDIA)

Master playlist 中定义替代音频、视频、字幕轨道：
```
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="English",DEFAULT=YES,URI="audio_eng.m3u8"
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="English",URI="subs_eng.m3u8"
```

| 属性 | 说明 |
|------|------|
| TYPE | AUDIO, VIDEO, SUBTITLES, CLOSED-CAPTIONS |
| GROUP-ID | 轨道组标识 |
| NAME | 显示名称 |
| DEFAULT | 是否默认选择 |
| AUTOSELECT | 是否自动选择 |
| LANGUAGE | 语言代码 |
| URI | 轨道播放列表地址 |

---

## 10. 流信息 (#EXT-X-STREAM-INF)

定义变体流（不同码率/分辨率）：
```
#EXT-X-STREAM-INF:BANDWIDTH=1280000,RESOLUTION=1920x1080,CODECS="avc1.640028,mp4a.40.2"
stream_1080.m3u8
```

| 属性 | 说明 |
|------|------|
| BANDWIDTH | 带宽（bps） |
| AVERAGE-BANDWIDTH | 平均带宽 |
| CODECS | 编解码器列表 |
| RESOLUTION | 分辨率 |
| FRAME-RATE | 帧率 |
| AUDIO | 音频组 ID |
| VIDEO | 视频组 ID |
| SUBTITLES | 字幕组 ID |
| CLOSED-CAPTIONS | 闭路字幕组 ID |

---

## 11. 直播 vs 点播

### 11.1 直播流 (Live)
- 无 `#EXT-X-ENDLIST`
- 无 `#EXT-X-PLAYLIST-TYPE:VOD`
- 持续更新，新片段追加
- 播放器需要定期刷新播放列表

### 11.2 点播流 (VOD)
- 有 `#EXT-X-ENDLIST`
- 或 `#EXT-X-PLAYLIST-TYPE:VOD`
- 内容固定，不可修改
- 播放器知道总时长

### 11.3 事件流 (EVENT)
- `#EXT-X-PLAYLIST-TYPE:EVENT`
- 可追加新片段
- 已有片段不可修改
- 结束时添加 `#EXT-X-ENDLIST`

---

## 12. fMP4 格式支持

HLS v5+ 支持 Fragmented MP4 格式：

### 12.1 特点
- 比 TS 更高效的容器格式
- 需要初始化片段 (#EXT-X-MAP)
- 支持字节范围请求

### 12.2 播放列表示例
```m3u8
#EXTM3U8
#EXT-X-VERSION:7
#EXT-X-TARGETDURATION:10
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-MAP:URI="init.mp4"
#EXTINF:10.0,
0.m4s
#EXTINF:10.0,
1.m4s
#EXT-X-ENDLIST
```

---

## 13. URL 解析注意事项

### 13.1 相对 URL 解析
- 相对 URL 需相对于播放列表 URL 解析
- 查询参数继承规则：
  - 子 URL 无查询参数时，继承父 URL 查询参数
  - 子 URL 有查询参数时，使用自身参数

### 13.2 字节范围请求
- 需在 HTTP 请求中添加 Range 头
- `Range: bytes=<start>-<end>`

---

## 14. 错误处理建议

### 14.1 必须处理的情况
- 播放列表刷新失败
- 片段下载失败
- 加密密钥获取失败
- 解密失败
- 不连续性处理

### 14.2 重试策略
- 网络错误应重试
- 404 错误可能表示片段未生成（直播）
- 服务器错误应有限重试

---

## 参考资料

- [RFC 8216 - HTTP Live Streaming](https://datatracker.ietf.org/doc/html/rfc8216)
- [Apple HLS Documentation](https://developer.apple.com/documentation/http_live_streaming)
- [HLS Authoring Specification for Apple Devices](https://developer.apple.com/documentation/http_live_streaming/hls_authoring_specification_for_apple_devices)