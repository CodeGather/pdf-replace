package pdf

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/signintech/gopdf"
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

// TableSpec 表格配置（含字体路径）
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

// RenderTableAsImage 渲染表格为 RGBA 图片（保留备用）
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
	if flexCount == 0 && fixedTotal > usableW {
		ratio := usableW / fixedTotal
		for i := range spec.Columns {
			if spec.Columns[i].Width != nil {
				w := *spec.Columns[i].Width * ratio
				spec.Columns[i].Width = &w
			}
		}
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

// convertColor 将 color.Color 转为 gopdf 色值
func convertColor(c color.Color) (uint8, uint8, uint8) {
	r, g, b, _ := c.RGBA()
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
}

// calcColWidths 计算列宽
func calcColWidths(cols []ColumnDef, usableW float64) []float64 {
	widths := make([]float64, len(cols))
	var fixedTotal float64
	var flexCount int
	for _, col := range cols {
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
	for i, col := range cols {
		if col.Width != nil {
			widths[i] = *col.Width
		} else {
			widths[i] = flexW
		}
	}
	return widths
}

// calcAlignX 根据对齐方式计算文字 x 起始坐标
func calcAlignX(cellX, cellW, textW float64, align string) float64 {
	switch align {
	case "left":
		return cellX + 3
	case "right":
		return cellX + cellW - textW - 3
	default: // center
		return cellX + (cellW-textW)/2
	}
}

// WriteTableToPDF 用 gopdf 生成原生 PDF 表格（文本可搜索），输出到 outputPath
// 返回表格高度
func WriteTableToPDF(outputPath string, columns []ColumnDef, rows []TableRow, fontPath string, pageW float64) (float64, error) {
	marginL := 0.0
	usableW := pageW - marginL*2
	colWidths := calcColWidths(columns, usableW)

	headerH := 28.0
	bodyH := 22.0
	totalH := headerH + float64(len(rows))*bodyH

	// 品牌红色（body 文字颜色）
	tR, tG, tB := uint8(160), uint8(30), uint8(30)

	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: gopdf.Rect{W: pageW, H: totalH + 20}})
	pdf.AddTTFFont("hyzdx", fontPath)
	pdf.AddPage()

	// 表头：灰色背景 + 黑色加粗文字
	pdf.SetFillColor(180, 180, 180)
	pdf.RectFromUpperLeftWithStyle(marginL, 0, usableW, headerH, "F")
	pdf.SetTextColor(0, 0, 0)
	x := marginL
	for i, col := range columns {
		pdf.SetFont("hyzdx", "", 11)
		cellW := colWidths[i]
		textW, _ := pdf.MeasureTextWidth(col.Label)
		tx := calcAlignX(x, cellW, textW, col.Align)
		// 文字在 28pt 灰色背景中上下居中
		// 11pt CJK 视觉中心偏移=11*0.38=4.18pt
		// center=14, baseline=14+4.18=18.18 → gopdf y=18
		pdf.SetXY(tx, 18)
		pdf.Text(col.Label)
		x += cellW
	}

	// 表体：品牌红色文字（无背景）
	// 每行22pt，10pt CJK 文字视觉中心居中：
	//   gopdf baseline = 行顶 + bodyH/2 + fontSize*0.38
	pdf.SetTextColor(tR, tG, tB)
	y := headerH + 10
	for _, row := range rows {
		// 10pt CJK in 22pt row: baseline = rowTop + bodyH/2 + fontSize*0.38
		yBase := y + bodyH/2 + 10*0.38
		x = marginL
		for i, col := range columns {
			val := row[col.Key]
			pdf.SetFont("hyzdx", "", 10)
			textW, _ := pdf.MeasureTextWidth(val)
			tx := calcAlignX(x, colWidths[i], textW, col.Align)
			pdf.SetXY(tx, yBase)
			pdf.Text(val)
			x += colWidths[i]
		}
		y += bodyH
	}

	if err := pdf.WritePdf(outputPath); err != nil {
		return 0, fmt.Errorf("写入表格 PDF 失败: %w", err)
	}

	return totalH, nil
}

// InjectTableContent 将表格 PDF 的内容流和字体资源注入到主 PDF 页面
func InjectTableContent(tmpl *Template, tempTablePath string, pageW, extraH float64) error {
	// 1. 读取表格 PDF
	tableCtx, err := api.ReadContextFile(tempTablePath)
	if err != nil {
		return fmt.Errorf("读取表格 PDF 失败: %w", err)
	}

	tp, _, _, err := tableCtx.PageDict(1, false)
	if err != nil {
		return fmt.Errorf("获取表格页面字典失败: %w", err)
	}

	// 2. 获取主页面字典
	mainPd, _, _, err := tmpl.ctx.PageDict(1, false)
	if err != nil {
		return fmt.Errorf("获取主页面字典失败: %w", err)
	}

	// 3. 复制字体资源（递归深拷贝所有间接对象）
	tRes, found := tp.Find("Resources")
	if !found {
		return fmt.Errorf("表格页面无 Resources")
	}
	tResDict, err := tableCtx.DereferenceDict(tRes)
	if err != nil {
		return fmt.Errorf("解引用表格 Resources 失败: %w", err)
	}

	tFont, found := tResDict.Find("Font")
	if !found {
		return fmt.Errorf("表格页面无 Font 资源")
	}
	tFontDict, err := tableCtx.DereferenceDict(tFont)
	if err != nil {
		return fmt.Errorf("解引用表格 Font 失败: %w", err)
	}

	// 获取主页面 Font dict
	mRes, found := mainPd.Find("Resources")
	if !found {
		return fmt.Errorf("主页面无 Resources")
	}
	mResDict, err := tmpl.ctx.DereferenceDict(mRes)
	if err != nil {
		return fmt.Errorf("解引用主 Resources 失败: %w", err)
	}

	mFontObj, found := mResDict.Find("Font")
	var mFontDict types.Dict
	if found {
		mFontDict, err = tmpl.ctx.DereferenceDict(mFontObj)
		if err != nil {
			return fmt.Errorf("解引用主 Font 失败: %w", err)
		}
	} else {
		mFontDict = types.NewDict()
	}

	// 递归深拷贝：将字体及其所有依赖对象从 tableCtx 复制到 tmpl.ctx
	fontNameMap := make(map[string]string)
	nameIdx := 0
	for k, v := range tFontDict {
		newName := fmt.Sprintf("FT%d", nameIdx)
		nameIdx++
		if ir, ok := v.(types.IndirectRef); ok {
			newRef, err := deepCopyRef(tableCtx.XRefTable, tmpl.ctx.XRefTable, ir)
			if err != nil {
				return fmt.Errorf("复制字体 %s 失败: %w", k, err)
			}
			mFontDict[newName] = *newRef
			fontNameMap[k] = newName
		}
	}
	mResDict["Font"] = mFontDict

	// 4. 获取表格内容流并替换字体名称
	tContents, found := tp.Find("Contents")
	if !found {
		return fmt.Errorf("表格页面无 Contents")
	}

	var contentBytes []byte
	switch obj := tContents.(type) {
	case types.StreamDict:
		if err := obj.Decode(); err != nil {
			return fmt.Errorf("解码表格内容流失败: %w", err)
		}
		contentBytes = obj.Content
	case types.IndirectRef:
		sd, _, err := tableCtx.DereferenceStreamDict(obj)
		if err != nil {
			return fmt.Errorf("解引用表格内容流失败: %w", err)
		}
		if err := sd.Decode(); err != nil {
			return fmt.Errorf("解码表格内容流失败: %w", err)
		}
		contentBytes = sd.Content
	default:
		return fmt.Errorf("不支持的内容流类型: %T", tContents)
	}

	// 替换字体名称（gopdf 使用 /F1 等）
	content := string(contentBytes)
	for oldName, newName := range fontNameMap {
		content = strings.ReplaceAll(content, "/"+oldName, "/"+newName)
	}
	contentBytes = []byte(content)

	// 5. 追加到主页面内容流
	if err := tmpl.ctx.XRefTable.AppendContent(mainPd, contentBytes); err != nil {
		return fmt.Errorf("追加表格内容流失败: %w", err)
	}

	return nil
}

// deepCopyRef 递归复制一个间接对象及其所有依赖的间接对象到目标 xref 表
func deepCopyRef(srcXRef, dstXRef *model.XRefTable, ir types.IndirectRef) (*types.IndirectRef, error) {
	objNr := ir.ObjectNumber.Value()
	genNr := ir.GenerationNumber.Value()

	entry, found := srcXRef.FindTableEntry(objNr, genNr)
	if !found || entry.Object == nil {
		// 无法找到源对象，直接引用（可能不完整但尽力而为）
		return &ir, nil
	}

	// 深拷贝对象，替换内部的 IndirectRef
	copiedObj, err := deepCopyObject(srcXRef, dstXRef, entry.Object)
	if err != nil {
		return nil, err
	}

	// 注册到目标 xref 表
	return dstXRef.IndRefForNewObject(copiedObj)
}

// deepCopyObject 递归遍历对象，将内部的 IndirectRef 全部替换为新上下文中的副本
func deepCopyObject(srcXRef, dstXRef *model.XRefTable, obj interface{}) (types.Object, error) {
	if obj == nil {
		return nil, nil
	}

	switch o := obj.(type) {
	case types.Dict:
		newDict := types.NewDict()
		for k, v := range o {
			cp, err := deepCopyValue(srcXRef, dstXRef, v)
			if err != nil {
				return nil, fmt.Errorf("copy dict key %s: %w", k, err)
			}
			newDict[k] = cp
		}
		return newDict, nil

	case types.StreamDict:
		// Deep copy embedded dict, then use StreamDict.Clone() which handles all fields
		cp := o.Clone().(types.StreamDict)
		// Deep copy the dict entries (replace IndirectRefs)
		for k, v := range o.Dict {
			replaced, err := deepCopyValue(srcXRef, dstXRef, v)
			if err != nil {
				return nil, fmt.Errorf("copy stream dict key %s: %w", k, err)
			}
			cp.Dict[k] = replaced
		}
		return cp, nil

	case types.IndirectRef:
		newRef, err := deepCopyRef(srcXRef, dstXRef, o)
		if err != nil {
			return nil, err
		}
		return *newRef, nil

	case types.Array:
		newArr := make(types.Array, len(o))
		for i, item := range o {
			cp, err := deepCopyValue(srcXRef, dstXRef, item)
			if err != nil {
				return nil, fmt.Errorf("copy array [%d]: %w", i, err)
			}
			newArr[i] = cp
		}
		return newArr, nil

	default:
		// Simple types (Name, Integer, Float, Boolean, StringLiteral, HexLiteral, Null etc.)
		// All implement Clone() so we can use that
		if objClone, ok := obj.(types.Object); ok {
			return objClone.Clone(), nil
		}
		// Fallback for unknown types
		return nil, fmt.Errorf("unsupported type for deep copy: %T", obj)
	}
}

// deepCopyValue 是 deepCopyObject 的别名，用于统一处理各种类型的复制
func deepCopyValue(srcXRef, dstXRef *model.XRefTable, v interface{}) (types.Object, error) {
	return deepCopyObject(srcXRef, dstXRef, v)
}