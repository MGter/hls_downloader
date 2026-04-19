# HLS Downloader

HLS (HTTP Live Streaming) 直播流和点播文件下载器，支持持续监控直播流并自动下载新的媒体片段。

## 功能特性

### 核心功能
- **直播流下载**：持续监控M3U8播放列表，自动下载新出现的媒体片段
- **点播文件下载**：自动识别VOD文件，下载完成后停止
- **事件流支持**：支持EVENT类型播放列表，持续检查直到出现ENDLIST

### HLS规范支持
- **播放列表类型**：Master Playlist、Media Playlist
- **协议版本**：支持HLS v1-v7的主要特性
- **媒体格式**：TS (.ts)、fMP4 (.m4s, .mp4)
- **字节范围请求**：支持 `#EXT-X-BYTERANGE`
- **初始化片段**：支持fMP4格式的 `#EXT-X-MAP`
- **查询参数继承**：子URL自动继承父URL的查询参数

### 技术特性
- **并发下载**：支持多线程并发下载，提高下载效率
- **重试机制**：下载失败自动重试，确保可靠性
- **优雅退出**：支持Ctrl+C优雅停止，不会丢失下载进度
- **进度追踪**：记录已下载片段，避免重复下载
- **日志统一**：所有日志带时间戳，格式规范

## 安装

### 系统要求
- Go 1.16+ 
- Linux / macOS / Windows

### 编译

```bash
# 使用Make编译
make build

# 或直接使用Go编译
go build -o bin/hls_downloader ./cmd/hls_downloader
```

编译后的可执行文件位于 `bin/hls_downloader`

## 使用方法

### 基本用法

```bash
# 下载直播流
./bin/hls_downloader https://example.com/live/stream.m3u8

# 下载点播文件（带查询参数）
./bin/hls_downloader "http://192.168.1.100:8080/vod.m3u8?starttime=1776214800&endtime=1776215400"

# 下载Master Playlist
./bin/hls_downloader https://example.com/master.m3u8
```

### 输出目录

下载的媒体片段保存在以M3U8文件名命名的目录中，例如：
- 输入URL: `https://example.com/vod.m3u8`
- 输出目录: `vod_hls_segments/`

文件命名格式：`时间戳_序号_原文件名`
例如：`20260416_031000_00000_segment.ts`

### 退出方式

- **Ctrl+C**：发送SIGINT信号，优雅退出
- **kill命令**：发送SIGTERM信号，优雅退出
- **点播文件**：下载完成后自动停止

## 日志输出

所有日志带时间戳，便于追踪：

```
2026/04/16 03:00:00 开始下载 HLS 流: https://example.com/vod.m3u8
2026/04/16 03:00:00 媒体片段保存目录: vod_hls_segments
2026/04/16 03:00:00 按 Ctrl+C 可优雅退出程序
2026/04/16 03:00:01 HLS协议版本: 3, 目标时长: 10秒
2026/04/16 03:00:01 播放列表类型: VOD
2026/04/16 03:00:01 发现 5 个新片段，开始下载
2026/04/16 03:00:02 下载完成: 20260416_030001_00000_segment0.ts (时长: 10.00秒)
2026/04/16 03:00:02 下载完成: 20260416_030001_00001_segment1.ts (时长: 8.50秒)
2026/04/16 03:00:03 点播文件下载完成，自动停止
```

## 配置参数

默认配置（可在代码中修改）：

| 参数 | 默认值 | 说明 |
|------|--------|------|
| MaxConcurrentDownloads | 8 | 最大并发下载数 |
| DownloadInterval | 5秒 | 直播流检查间隔 |
| MaxRetryAttempts | 3 | 下载失败重试次数 |
| RetryDelayBase | 1秒 | 重试等待时间基数 |

## 项目结构

```
hls_downloader/
├── cmd/hls_downloader/     # 入口点，CLI处理
│   └── main.go
├── internal/
│   ├── downloader/         # 核心下载逻辑
│   │   └── downloader.go   # 主循环、片段过滤、并发下载
│   ├── parser/             # M3U8解析器
│   │   └ m3u8_parser.go    # 播放列表解析、URL处理
│   └ storage/              # 文件存储管理
│   │   └ file_manager.go   # 并发下载、字节范围请求
├── pkg/
│   ├── utils/              # HTTP工具
│   │   └ http_utils.go     # HTTP GET请求
│   └ logger/               # 日志工具
│       └ logger.go         # 日志级别定义
├── docs/
│   └ HLS_Specification.md  # HLS规范文档
├── bin/                    # 编译输出目录
└── Makefile                # 编译脚本
```

## 支持的M3U8标签

### 基本标签
- `#EXTM3U` - 文件头验证
- `#EXT-X-VERSION` - 协议版本
- `#EXTINF` - 片段时长（支持浮点数）
- `#EXT-X-ENDLIST` - 点播结束标记

### 媒体播放列表标签
- `#EXT-X-TARGETDURATION` - 目标时长
- `#EXT-X-MEDIA-SEQUENCE` - 媒体序列号
- `#EXT-X-PLAYLIST-TYPE` - 播放列表类型 (VOD/EVENT)
- `#EXT-X-DISCONTINUITY` - 不连续性标记
- `#EXT-X-BYTERANGE` - 字节范围请求
- `#EXT-X-MAP` - 初始化片段 (fMP4)

### 主播放列表标签
- `#EXT-X-STREAM-INF` - 流信息
- 自动选择第一个媒体播放列表

## 测试

```bash
# 运行测试（如果有测试文件）
make test

# 或直接使用Go
go test ./... -v
```

## 代码检查

```bash
# 格式化代码 + 运行go vet
make check
```

## 常见问题

### Q: 带查询参数的URL下载失败？
A: 已支持查询参数继承，子播放列表和媒体片段会自动继承原始URL的查询参数。

### Q: Master Playlist如何处理？
A: 自动选择第一个媒体播放列表进行下载。

### Q: 点播文件下载不停止？
A: 检查M3U8是否包含 `#EXT-X-ENDLIST` 或 `#EXT-X-PLAYLIST-TYPE:VOD` 标签。

### Q: fMP4格式如何下载？
A: 支持fMP4格式，会自动下载初始化片段 (#EXT-X-MAP)。

## 参考资料

- [HLS规范文档](docs/HLS_Specification.md)
- [RFC 8216 - HTTP Live Streaming](https://datatracker.ietf.org/doc/html/rfc8216)
- [Apple HLS Documentation](https://developer.apple.com/documentation/http_live_streaming)

## License

MIT License