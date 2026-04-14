# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
make build          # Build executable to bin/hls_downloader
make test           # Run all unit tests with verbose output
make check          # Format code + run go vet
make run ARGS="https://example.com/playlist.m3u8"  # Run with M3U8 URL
make clean          # Remove build artifacts
```

Or use Go directly:
```bash
go build -o bin/hls_downloader ./cmd/hls_downloader
go test ./... -v
go run ./cmd/hls_downloader <M3U8_URL>
```

## Architecture

This is an HLS (HTTP Live Streaming) downloader that continuously monitors a live M3U8 playlist and downloads new TS segments.

### Package Structure
- `cmd/hls_downloader/` - Entry point, CLI handling
- `internal/downloader/` - Core orchestration loop, segment filtering
- `internal/parser/` - M3U8 playlist parsing (master/media playlist detection)
- `internal/storage/` - Concurrent file download with retry logic
- `pkg/utils/` - HTTP utilities
- `pkg/logger/` - Logging utilities

### Key Flow
1. `downloader.Start()` creates output directory and enters main loop
2. Main loop downloads M3U8, parses it, filters new segments, downloads them
3. `downloaded map[string]bool` tracks already-downloaded segments (protected by `sync.RWMutex`)
4. Supports graceful shutdown via SIGINT/SIGTERM signals with context cancellation

### Concurrency Model
- `FileManager.ConcurrentDownload()` uses semaphore channel for max concurrency limit
- Each segment download runs in a goroutine with retry support
- Context cancellation propagates to stop all downloads gracefully