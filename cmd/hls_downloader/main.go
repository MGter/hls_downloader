package main  // 声明这是 main 包，表示这是一个可执行程序

import (
	"log"      // 标准日志包，用于输出错误信息
	"os"       // 操作系统功能包，可以获取命令行参数等
	"path"     // 路径处理包，这里用来获取程序名

	"github.com/MGter/hls_downloader/internal/downloader"  // 自己写的下载器核心代码
	"github.com/MGter/hls_downloader/pkg/logger"           // 自己写的日志工具
)

// printHelp 显示帮助信息
func printHelp() {
	// path.Base() 获取程序名
	app := path.Base(os.Args[0])
	
	// 使用自定义的日志工具输出信息
	logger.Info.Printf("HLS 直播流下载器\n\n")
	logger.Info.Printf("用法: %s <M3U8_URL>\n\n", app)  // %s 会被 app 替换
	logger.Info.Printf("示例: %s https://example.com/live/stream/playlist.m3u8\n", app)
}

// main 函数是程序的入口点，程序从这里开始执行
func main() {
	// 检查命令行参数数量，os.Args[0] 是程序名，os.Args[1] 才是第一个参数
	if len(os.Args) < 2 {
		// 如果用户没有输入 M3U8 地址，显示帮助信息
		printHelp()
		os.Exit(1)  // 退出程序，1 表示异常退出
	}

	// 获取用户输入的 M3U8 直播流地址
	hlsURL := os.Args[1]
	
	// 创建下载器实例
	dl := downloader.New()
	
	// 开始下载直播流
	if err := dl.Start(hlsURL); err != nil {
		// 如果下载出错，输出错误信息并退出程序
		log.Fatalf("下载器意外退出: %v", err)  // %v 会显示错误详情
	}
}