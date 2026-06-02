package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pdf-replace/cmd"
)

const usage = `用法: pdf-replace <input.json> [-o output.pdf] [-cpu N]

将店铺灯位画面素材替换到平面图 PDF 模板中
  -o string    输出 PDF 路径（默认：输入文件同目录，文件名同 JSON 名）
  -cpu int     并发 worker 数（默认：自动，最多4个）
`

func main() {
	var inputPath, outputPath string
	cpu := 0 // 0 = auto (默认逻辑)

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		if args[i] == "-o" && i+1 < len(args) {
			outputPath = args[i+1]
			i++
		} else if args[i] == "-cpu" && i+1 < len(args) {
			fmt.Sscanf(args[i+1], "%d", &cpu)
			i++
		} else if !strings.HasPrefix(args[i], "-") {
			inputPath = args[i]
		} else {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(1)
		}
	}

	if inputPath == "" {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	// 默认输出路径
	if outputPath == "" {
		ext := filepath.Ext(inputPath)
		base := strings.TrimSuffix(inputPath, ext)
		outputPath = base + ".pdf"
	}

	if err := cmd.Run(inputPath, outputPath, cpu); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}