package main

import (
	"log"
	"os"
	"path"

	"github.com/MGter/hls_downloader/internal/downloader"
	"github.com/MGter/hls_downloader/pkg/logger"
)

// printHelp 显示帮助信息
func printHelp() {
	app := path.Base(os.Args[0])
	logger.Info.Printf("HLS 直播流下载器\n\n")
	logger.Info.Printf("用法: %s <M3U8_URL>\n\n", app)
	logger.Info.Printf("示例: %s https://example.com/live/stream/playlist.m3u8\n", app)
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	hlsURL := os.Args[1]
	
	// 创建下载器实例
	dl := downloader.New()
	
	// 开始下载
	if err := dl.Start(hlsURL); err != nil {
		log.Fatalf("下载器意外退出: %v", err)
	}
}