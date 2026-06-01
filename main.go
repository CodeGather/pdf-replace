package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pdf-replace/cmd"
)

func main() {
	var outputPath string
	flag.StringVar(&outputPath, "o", "", "输出 PDF 路径（默认：输入文件同目录，文件名同 JSON 名）")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: pdf-replace <input.json> [-o output.pdf]\n")
		fmt.Fprintf(os.Stderr, "\n将店铺灯位画面素材替换到平面图 PDF 模板中\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	inputPath := flag.Arg(0)

	// 默认输出路径
	if outputPath == "" {
		ext := filepath.Ext(inputPath)
		base := strings.TrimSuffix(inputPath, ext)
		outputPath = base + ".pdf"
	}

	if err := cmd.Run(inputPath, outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}
