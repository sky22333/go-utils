package main

import (
	"embed"
	"flag"
	"log"
)

//go:embed assets/*
var assets embed.FS

func main() {
	logoPath := flag.String("logo", "", "Logo图片路径，支持PNG/JPEG格式，留空使用内嵌logo")
	port := flag.String("port", "9000", "服务器端口")
	flag.Parse()

	server := NewServer(assets)
	server.LoadLogo(*logoPath)

	if err := server.Run(*port); err != nil {
		log.Fatal("服务器启动失败:", err)
	}
} 