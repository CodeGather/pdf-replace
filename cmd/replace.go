package cmd

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"

	"pdf-replace/config"
	"pdf-replace/matcher"
	"pdf-replace/model"
	"pdf-replace/pdf"
)

const (
	fontPath   = "/Users/Yau/work/1.Resources/2.AI/pdf-replace/assets/hyzdx.ttf"
	tolerance  = 5.0
	basePDFDir = "/Users/Yau/Library/Application Support/com.lorealchina.mplus"
)

type tableRow struct {
	num   string
	item  model.LampItem
	image model.ImageMeta
}

type lampJob struct {
	numStr   string
	lampItem model.LampItem
	objNr    int
	srcPath  string
	pngData  []byte
	isNew    bool
}

// Run 程序主入口
func Run(inputPath, outputPath string) error {
	cfg, err := config.LoadConfig(inputPath)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	log.Printf("店铺: %s", cfg.ShopName)
	log.Printf("灯位数量: %d", len(cfg.ExcelData))
	log.Printf("素材数量: %d", len(cfg.FileData))

	// 1. 确定模板 PDF 路径
	if len(cfg.DbData.Lamps) == 0 {
		return fmt.Errorf("db-data.lamp 为空")
	}
	tmplPath := filepath.Join(basePDFDir, cfg.DbData.Lamps[0].File)
	log.Printf("模板 PDF: %s", tmplPath)

	// 2. 打开模板
	tmpl, err := pdf.OpenTemplate(tmplPath)
	if err != nil {
		return fmt.Errorf("打开模板失败: %w", err)
	}
	defer tmpl.Close()

	// 3. 准备所有灯位的任务
	type prepJob struct {
		numStr   string
		lampItem model.LampItem
		objNr    int
		srcPath  string
		isNew    bool
		err      error
	}
	var prepJobs []prepJob

	for _, entry := range cfg.DbData.Lamps[0].NumList {
		// 3a. 灯位编号
		numStr := ""
		if len(entry.Nums) > 0 {
			numStr = entry.Nums[0].Str
		} else if entry.Num.Str != "" {
			numStr = entry.Num.Str
		}
		if numStr == "" {
			log.Printf("  [跳过] 无灯位编号")
			continue
		}

		// 3b. excel-data
		lampItem, ok := cfg.ExcelData[numStr]
		if !ok {
			log.Printf("  [跳过] 灯位 %s 不在 excel-data 中", numStr)
			continue
		}

		// 3c. 匹配 PDF 图片位置
		imgX := entry.Image.OriginalTransform.X
		imgY := entry.Image.OriginalTransform.Y
		imgW := entry.Image.OriginalTransform.Width
		imgH := entry.Image.OriginalTransform.Height
		if imgW <= 0 || imgH <= 0 {
			imgX = entry.Image.X
			imgY = entry.Image.Y
			imgW = entry.Image.Width
			imgH = entry.Image.Height
		}
		ip := tmpl.FindImageByRect(imgX, imgY, imgW, imgH, tolerance)
		if ip == nil {
			log.Printf("  [跳过] 灯位 %s 未找到匹配的 PDF 图片位置", numStr)
			continue
		}
		log.Printf("  灯位 %s: obj=%d", numStr, ip.ObjNr)

		// 3d. 匹配素材
		fileKey, found := matcher.MatchFileDataKey(lampItem.LampNote, cfg.FileData)
		if !found {
			log.Printf("  [跳过] 灯位 %s 无匹配素材", numStr)
			continue
		}
		fileEntry := cfg.FileData[fileKey]

		var allImages []model.ImageMeta
		for _, page := range fileEntry.Pages {
			allImages = append(allImages, page.Images...)
		}
		if len(allImages) == 0 {
			log.Printf("  [跳过] 灯位 %s 素材无图片", numStr)
			continue
		}

		targetW := lampItem.VisibleW
		targetH := lampItem.VisibleH
		if targetW <= 0 || targetH <= 0 {
			targetW = entry.Image.Width
			targetH = entry.Image.Height
		}
		match := matcher.SelectBestImage(targetW, targetH, allImages)
		if !match.Found {
			log.Printf("  [跳过] 灯位 %s 无法匹配图片", numStr)
			continue
		}
		log.Printf("  选中图片: %s (%.0fx%.0f)", match.Image.Path, match.Image.Width, match.Image.Height)

		prepJobs = append(prepJobs, prepJob{
			numStr:   numStr,
			lampItem: lampItem,
			objNr:    ip.ObjNr,
			srcPath:  match.Image.Path,
			isNew:    lampItem.IsNewValue(),
		})
	}

	// 4. 并行处理图片（打开→解码→绘制→编码）
	log.Printf("并行处理 %d 张图片...", len(prepJobs))
	type processed struct {
		numStr string
		objNr  int
		data   []byte
		isNew  bool
		err    error
	}

	jobs := make(chan prepJob, len(prepJobs))
	results := make(chan processed, len(prepJobs))
	var wg sync.WaitGroup

	workerCount := len(prepJobs)
	if workerCount > 4 {
		workerCount = 4
	}
	if workerCount < 1 {
		workerCount = 1
	}

	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				data, err := processImage(job.srcPath, job.lampItem, cfg.BrandConf)
				results <- processed{numStr: job.numStr, objNr: job.objNr, data: data, isNew: job.isNew, err: err}
			}
		}()
	}

	for _, j := range prepJobs {
		jobs <- j
	}
	close(jobs)
	wg.Wait()
	close(results)

	// 5. 串行替换 PDF（pdfcpu 非线程安全）
	var newRows []tableRow
	for r := range results {
		if r.err != nil {
			log.Printf("  [错误] 灯位 %s: %v", r.numStr, r.err)
			continue
		}
		if err := tmpl.ReplaceImage(r.objNr, r.data); err != nil {
			log.Printf("  [错误] 灯位 %s 替换失败: %v", r.numStr, err)
			continue
		}
		log.Printf("  [替换] 灯位 %s (obj=%d)", r.numStr, r.objNr)
		if r.isNew {
			newRows = append(newRows, tableRow{num: r.numStr})
		}
	}

	// 6. 构建 isNew 表格
	if _, err := renderTable(tmpl, cfg, newRows); err != nil {
		return fmt.Errorf("渲染表格失败: %w", err)
	}

	// 7. 写入
	if err := tmpl.WriteToFile(outputPath); err != nil {
		return fmt.Errorf("写入 PDF 失败: %w", err)
	}
	log.Printf("完成: %s", outputPath)
	return nil
}

// processImage 在 worker 中处理单张图片
func processImage(srcPath string, lampItem model.LampItem, bc model.BrandConfig) ([]byte, error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("打开图片失败: %w", err)
	}
	defer f.Close()

	srcImg, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("解码图片失败: %w", err)
	}

	img := srcImg

	// isNew 边框
	if lampItem.IsNewValue() {
		bs := int(math.Round(bc.BorderSize))
		if bs < 1 {
			bs = 2
		}
		img = pdf.DrawBorder(img, bs, bc.BorderColor.Red, bc.BorderColor.Green,
			bc.BorderColor.Blue, bc.BorderColor.Opacity)
	}

	// 上市备注文字
	if lampItem.LaunchNote != "" {
		fontPt := bc.DescFontSize
		if fontPt <= 0 {
			fontPt = 16
		}
		img = pdf.DrawTextOnTop(img, lampItem.LaunchNote,
			bc.DescColor.Red, bc.DescColor.Green, bc.DescColor.Blue, bc.DescColor.Opacity,
			fontPt, fontPath)
	}

	return pdf.EncodePNG(img)
}

// renderTable 渲染 isNew 表格为原生 PDF 文本（可搜索）并注入到页面底部
func renderTable(tmpl *pdf.Template, cfg *model.Config, rows []tableRow) (float64, error) {
	if len(rows) == 0 {
		return 0, nil
	}

	// 构建列定义
	var cols []pdf.ColumnDef
	for _, tc := range cfg.TableConf {
		col := pdf.ColumnDef{Label: tc.Label, Key: tc.Key, Align: tc.Align}
		if tc.Width != nil {
			col.Width = tc.Width
		}
		cols = append(cols, col)
	}

	// 构建行数据（key 必须与 table-config.key 一致）
	var tbRows []pdf.TableRow
	for _, r := range rows {
		row := pdf.TableRow{
			"柜台名称": cfg.ShopName,
			"灯位编号": r.num,
			"灯位位置": r.item.Position,
			"材质":   r.item.Material,
			"可见宽":  fmt.Sprintf("%.0f", r.item.VisibleW),
			"可见长":  fmt.Sprintf("%.0f", r.item.VisibleH),
			"灯位备注": r.item.LampNote,
			"画面内容": r.item.Content,
		}
		tbRows = append(tbRows, row)
	}

	pageW, _, err := tmpl.PageSize(1)
	if err != nil {
		return 0, fmt.Errorf("获取页面尺寸: %w", err)
	}

	// 预估表格高度 + gap，扩展页面 + 上移原有内容
	gap := 25.0
	estTableH := 28.0 + float64(len(rows))*22.0
	extraH := estTableH + gap
	if err := tmpl.ExtendPageHeight(extraH); err != nil {
		return 0, fmt.Errorf("扩展页面高度: %w", err)
	}

	// 生成临时表格 PDF（使用 gopdf，原生文本）
	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("pdf-replace-table-%d.pdf", os.Getpid()))
	tableH, err := pdf.WriteTableToPDF(tmpPath, cols, tbRows, fontPath, pageW)
	if err != nil {
		return 0, fmt.Errorf("生成表格 PDF: %w", err)
	}
	_ = tableH
	defer os.Remove(tmpPath)

	// 注入表格内容流到主 PDF
	if err := pdf.InjectTableContent(tmpl, tmpPath, pageW, extraH); err != nil {
		return 0, fmt.Errorf("注入表格内容: %w", err)
	}

	log.Printf("表格已注入 (行数=%d, 字体=%s)", len(rows), filepath.Base(fontPath))
	return 0, nil
}