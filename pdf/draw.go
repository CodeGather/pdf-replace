package pdf

import (
	"bytes"
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
)

// loadFontFace 加载中文字体文件，返回 sfnt.Font
func loadFontFace(fontPath string) (*sfnt.Font, error) {
	data, err := os.ReadFile(fontPath)
	if err != nil {
		return nil, err
	}
	return sfnt.Parse(data)
}

// rgbaToColor 将 0-1 浮点数 RGBA 转为 color.Color
func rgbaToColor(r, g, b, a float64) color.Color {
	return color.RGBA{
		R: uint8(math.Round(r * 255)),
		G: uint8(math.Round(g * 255)),
		B: uint8(math.Round(b * 255)),
		A: uint8(math.Round(a * 255)),
	}
}

// DrawBorder 在图片四周绘制边框，返回新图片
func DrawBorder(img image.Image, borderSize int, r, g, b, a float64) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	newW := w + borderSize*2
	newH := h + borderSize*2
	canvas := image.NewRGBA(image.Rect(0, 0, newW, newH))

	borderColor := rgbaToColor(r, g, b, a)
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{borderColor}, image.Point{}, draw.Src)
	draw.Draw(canvas,
		image.Rect(borderSize, borderSize, borderSize+w, borderSize+h),
		img, image.Point{}, draw.Src)

	return canvas
}

// DrawTextOnTop 在图片上方绘制文本，自动缩字号以适应图片宽度
func DrawTextOnTop(img image.Image, text string, r, g, b, a float64, fontSizePt float64, fontPath string) image.Image {
	if text == "" {
		return img
	}

	bounds := img.Bounds()
	imgW := bounds.Dx()

	f, err := loadFontFace(fontPath)
	if err != nil {
		return img
	}

	// 搜索合适字号
	var sfntBuf sfnt.Buffer
	buf := &sfntBuf
	adjustedSize := fontSizePt
	for adjustedSize >= 4 {
		totalW := measureSimple(f, buf, text, adjustedSize)
		if totalW <= float64(imgW) {
			break
		}
		adjustedSize -= 0.5
	}
	if adjustedSize < 4 {
		adjustedSize = 4
	}

	lineHeight := adjustedSize + 4
	paddingTop := 2.0
	textAreaH := int(math.Ceil(lineHeight + paddingTop))

	newH := bounds.Dy() + textAreaH
	canvas := image.NewRGBA(image.Rect(0, 0, imgW, newH))

	draw.Draw(canvas,
		image.Rect(0, textAreaH, imgW, newH),
		img, image.Point{}, draw.Src)

	// 绘制文字
	drawTextAt(canvas, f, buf, text, adjustedSize, paddingTop, imgW, rgbaToColor(r, g, b, a))

	return canvas
}

// measureSimple 测量文本宽度（像素）
func measureSimple(f *sfnt.Font, buf *sfnt.Buffer, text string, sizePt float64) float64 {
	ppem := fixed.I(int(sizePt))
	var totalW fixed.Int26_6
	for _, r := range text {
		gi, err := f.GlyphIndex(buf, r)
		if err != nil || gi == 0 {
			totalW += ppem / 2 // fallback
			continue
		}
		advance, err := f.GlyphAdvance(buf, gi, ppem, font.HintingNone)
		if err != nil {
			totalW += ppem / 2
			continue
		}
		totalW += advance
	}
	return float64(totalW) / 64.0
}

// drawTextAt 在画布上水平居中绘制文本
func drawTextAt(canvas *image.RGBA, f *sfnt.Font, buf *sfnt.Buffer, text string, sizePt float64, paddingTop float64, canvasW int, clr color.Color) {
	ppem := fixed.I(int(sizePt))

	// 计算总宽
	var totalW fixed.Int26_6
	for _, r := range text {
		gi, err := f.GlyphIndex(buf, r)
		if err != nil {
			continue
		}
		adv, err := f.GlyphAdvance(buf, gi, ppem, font.HintingNone)
		if err != nil {
			continue
		}
		totalW += adv
	}

	startX := fixed.I(canvasW/2) - totalW/2
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: sizePt, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return
	}
	defer face.Close()

	d := font.Drawer{
		Dst:  canvas,
		Src:  image.NewUniform(clr),
		Face: face,
		Dot: fixed.Point26_6{
			X: startX,
			Y: fixed.I(int(paddingTop)) + ppem,
		},
	}
	d.DrawString(text)
}

// EncodePNG 将图片编码为 PNG bytes
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}