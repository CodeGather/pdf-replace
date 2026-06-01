package cmd

import (
	"fmt"
	"log"

	"pdf-replace/config"
)

// Run 是程序主入口
func Run(inputPath, outputPath string) error {
	cfg, err := config.LoadConfig(inputPath)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	log.Printf("店铺: %s", cfg.ShopName)
	log.Printf("灯位数量: %d", len(cfg.ExcelData))
	log.Printf("素材数量: %d", len(cfg.FileData))
	log.Printf("输出: %s", outputPath)

	// TODO: 后续阶段接入替换逻辑
	log.Println("准备就绪，等待后续开发阶段...")

	return nil
}