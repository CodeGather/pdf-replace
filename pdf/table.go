package pdf

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ColumnDef 表格列定义
type ColumnDef struct {
	Label string
	Key   string
	Width *float64
	Align string
}

// TableRow 表格行数据
type TableRow map[string]string

// TableSpec 表格配置
type TableSpec struct {
	Columns      []ColumnDef
	Rows         []TableRow
	HeaderColor  color.Color
	BodyColor    color.Color
	HeaderFontSz float64
	BodyFontSz   float64
	FontPath     string
	PageWidth    float64
}

// RenderTableAsImage 渲染表格为 RGBA 图片
func RenderTableAsImage(spec TableSpec) (*image.RGBA, float64, error) {
	fontData, err := os.ReadFile(spec.FontPath)
	if err != nil {
		return nil, 0, fmt.Errorf("读取字体失败: %w", err)
	}
	f, err := sfnt.Parse(fontData)
	if err != nil {
		return nil, 0, fmt.Errorf("解析字体失败: %w", err)
	}

	marginL, marginR := 20.0, 20.0
	usableW := spec.PageWidth - marginL - marginR

	var fixedTotal float64
	var flexCount int
	for _, col := range spec.Columns {
		if col.Width != nil {
			fixedTotal += *col.Width
		} else {
			flexCount++
		}
	}
	flexW := (usableW - fixedTotal) / float64(flexCount)
	if flexW < 30 {
		flexW = 30
	}

	colWidths := make([]float64, len(spec.Columns))
	totalW := marginL + marginR
	for i, col := range spec.Columns {
		if col.Width != nil {
			colWidths[i] = *col.Width
		} else {
			colWidths[i] = flexW
		}
		totalW += colWidths[i]
	}

	headerH := spec.HeaderFontSz + 12
	bodyH := spec.BodyFontSz + 10
	padding := 4.0
	totalH := headerH + float64(len(spec.Rows))*bodyH + 4

	canvasW := int(math.Ceil(totalW))
	canvasH := int(math.Ceil(totalH))
	canvas := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)

	yPos := 2.0
	// 表头
	draw.Draw(canvas, image.Rect(0, int(yPos), canvasW, int(yPos+headerH)),
		&image.Uniform{spec.HeaderColor}, image.Point{}, draw.Src)
	xPos := marginL
	for i, col := range spec.Columns {
		cw := colWidths[i]
		drawCellText(canvas, f, col.Label, spec.HeaderFontSz,
			xPos+padding, yPos+padding, cw-padding*2, headerH-padding*2, color.White)
		xPos += cw
	}
	yPos += headerH

	// 表体
	for _, row := range spec.Rows {
		draw.Draw(canvas, image.Rect(0, int(yPos), canvasW, int(yPos+bodyH)),
			&image.Uniform{spec.BodyColor}, image.Point{}, draw.Src)
		xPos = marginL
		for i, col := range spec.Columns {
			cw := colWidths[i]
			val := row[col.Key]
			cellColor := color.RGBA{160, 30, 30, 255}
			drawCellText(canvas, f, val, spec.BodyFontSz,
				xPos+padding, yPos+padding, cw-padding*2, bodyH-padding*2, cellColor)
			xPos += cw
		}
		yPos += bodyH
	}
	return canvas, totalH, nil
}

func drawCellText(canvas *image.RGBA, f *sfnt.Font, text string, sizePt, x, y, maxW, maxH float64, clr color.Color) {
	if text == "" {
		return
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: sizePt, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return
	}
	defer face.Close()
	var totalW fixed.Int26_6
	var buf sfnt.Buffer
	for _, r := range text {
		gi, err := f.GlyphIndex(&buf, r)
		if err != nil {
			continue
		}
		ppem := fixed.I(int(sizePt))
		adv, err := f.GlyphAdvance(&buf, gi, ppem, font.HintingNone)
		if err != nil {
			continue
		}
		totalW += adv
	}
	startX := fixed.I(int(x + (maxW-float64(totalW)/64.0)/2))
	startY := fixed.I(int(y + (maxH-sizePt)/2 + sizePt))
	d := font.Drawer{
		Dst: canvas, Src: image.NewUniform(clr), Face: face,
		Dot: fixed.Point26_6{X: startX, Y: startY},
	}
	d.DrawString(text)
}

// StampTableImage 通过两阶段写入（temp文件→水印叠加）将表格加入 PDF 底部
// 自动扩展页面高度并居中放置表格
// tempDir: 临时文件的目录（如输出文件所在目录），空字符串使用 os.TempDir()
func StampTableImage(tmpl *Template, tableImg *image.RGBA, tempDir string) error {
	if tableImg == nil || tableImg.Bounds().Dy() == 0 {
		return nil
	}

	imgW := float64(tableImg.Bounds().Dx())
	imgH := float64(tableImg.Bounds().Dy())
	gap := 20.0

	// 1. 获取当前页面尺寸
	_, _, err := tmpl.PageSize(1)
	if err != nil {
		return fmt.Errorf("获取页面尺寸失败: %w", err)
	}

	// 2. 扩展 MediaBox 高度
	tmpl.ExtendPageHeight(imgH + gap)

	if tempDir == "" {
		tempDir = os.TempDir()
	}

	// 3. 写出到 temp 文件
	tempPath := tempDir + "/pdf-replace-table-tmp.pdf"
	outPath := tempDir + "/pdf-replace-table-out.pdf"
	if err := tmpl.WriteToFile(tempPath); err != nil {
		return fmt.Errorf("写出临时 PDF 失败: %w", err)
	}

	// 4. 编码表格为 PNG
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, tableImg); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("编码表格 PNG 失败: %w", err)
	}

	// 5. 创建水印：底部居中放置
	wm := model.DefaultWatermarkConfig()
	wm.OnTop = true
	wm.Mode = model.WMImage
	wm.FileName = "table.png"
	wm.Image = bytes.NewReader(pngBuf.Bytes())
	wm.Pos = types.BottomCenter
	wm.Dx = 0
	wm.Dy = gap
	wm.Scale = 1.0
	wm.ScaleAbs = true
	wm.Width = int(imgW)
	wm.Height = int(imgH)

	// 6. 水印叠加（将表格 stamp 到扩展后的页面底部）
	if err := api.AddWatermarksFile(tempPath, outPath, []string{"1"}, wm, nil); err != nil {
		os.Remove(tempPath)
		os.Remove(outPath)
		return fmt.Errorf("叠加表格失败: %w", err)
	}

	// 7. 替换原始 PDF 内容
	newCtx, err := api.ReadContextFile(outPath)
	if err != nil {
		os.Remove(tempPath)
		os.Remove(outPath)
		return fmt.Errorf("重新读取 PDF 失败: %w", err)
	}

	// 替换 tmpl 的 ctx
	tmpl.SetContext(newCtx)

	os.Remove(tempPath)
	os.Remove(outPath)
	return nil
}