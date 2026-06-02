package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pdf-replace/cmd"
)

const usage = `用法: pdf-replace <input.json> [-o output.pdf] [-cpu N]
       pdf-replace -merge <目录> [-o 输出目录] [-cpu N]

单文件模式：
  将店铺灯位画面素材替换到平面图 PDF 模板中
  -o string    输出 PDF 路径（默认：输入文件同目录，文件名同 JSON 名）
  -cpu int     并发 worker 数（默认：自动，最多4个）

批量合成模式：
  -merge string  合成目录路径（包含 001.json, 002.json ...）
  -o string      输出目录（默认：合成目录下的 _output_）
  -cpu int       并发 worker 数
`

func main() {
	var inputPath, outputPath, mergeDir string
	cpu := 0

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-merge" && i+1 < len(args):
			mergeDir = args[i+1]
			i++
		case args[i] == "-o" && i+1 < len(args):
			outputPath = args[i+1]
			i++
		case args[i] == "-cpu" && i+1 < len(args):
			fmt.Sscanf(args[i+1], "%d", &cpu)
			i++
		case !strings.HasPrefix(args[i], "-"):
			inputPath = args[i]
		default:
			fmt.Fprint(os.Stderr, usage)
			os.Exit(1)
		}
	}

	// 批量合成模式
	if mergeDir != "" {
		if err := cmd.RunMerge(mergeDir, outputPath, cpu); err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 单文件模式
	if inputPath == "" {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

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
