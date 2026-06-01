package matcher

import (
	"math"
	"testing"

	"pdf-replace/model"
)

func approx(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}

func imgMeta(w, h float64) model.ImageMeta {
	return model.ImageMeta{Width: w, Height: h}
}

// ---------- 方向判定 ----------

func TestClassifyDirection(t *testing.T) {
	tests := []struct {
		ratio float64
		want  ImgDirection
	}{
		{1.5, DirLandscape},
		{2.0, DirLandscape},
		{0.5, DirPortrait},
		{0.8, DirPortrait},
		{1.0, DirSquare},
		{1.005, DirSquare},
		{0.995, DirSquare},
		{1.02, DirLandscape}, // 超出容差
	}
	for _, tt := range tests {
		got := ClassifyDirection(tt.ratio)
		if got != tt.want {
			t.Errorf("ClassifyDirection(%.4f) = %v, want %v", tt.ratio, got, tt.want)
		}
	}
}

// ---------- 单图直接使用 ----------

func TestSingleImage(t *testing.T) {
	images := []model.ImageMeta{imgMeta(800, 600)}
	result := SelectBestImage(100, 50, images)
	if !result.Found {
		t.Fatal("should find the single image")
	}
	if !approx(result.ImgRatio, 800.0/600.0) {
		t.Errorf("ratio = %.4f, want %.4f", result.ImgRatio, 800.0/600.0)
	}
}

// ---------- 横图匹配横图 ----------

func TestLandscapeMatchLandscape(t *testing.T) {
	images := []model.ImageMeta{
		imgMeta(400, 500), // 竖图 0.8
		imgMeta(800, 400), // 横图 2.0
		imgMeta(600, 300), // 横图 2.0
		imgMeta(900, 600), // 横图 1.5
	}
	result := SelectBestImage(1200, 800, images) // 目标 1.5
	if !result.Found {
		t.Fatal("should find match")
	}
	// 期望选中横图 1.5（最接近）
	if !approx(result.ImgRatio, 1.5) {
		t.Errorf("got ratio %.4f, want 1.5", result.ImgRatio)
	}
}

// ---------- 竖图匹配竖图 ----------

func TestPortraitMatchPortrait(t *testing.T) {
	images := []model.ImageMeta{
		imgMeta(800, 400), // 横图 2.0
		imgMeta(500, 800), // 竖图 0.625
		imgMeta(300, 600), // 竖图 0.5
	}
	result := SelectBestImage(200, 400, images) // 目标 0.5
	if !result.Found {
		t.Fatal("should find match")
	}
	if !approx(result.ImgRatio, 0.5) {
		t.Errorf("got ratio %.4f, want 0.5", result.ImgRatio)
	}
}

// ---------- 正方形匹配 ----------

func TestSquareMatch(t *testing.T) {
	images := []model.ImageMeta{
		imgMeta(800, 400), // 横图 2.0
		imgMeta(400, 800), // 竖图 0.5
		imgMeta(100, 100), // 正方形 1.0
		imgMeta(200, 200), // 正方形 1.0
	}
	result := SelectBestImage(300, 300, images) // 目标 1.0
	if !result.Found {
		t.Fatal("should find match")
	}
	if !approx(result.ImgRatio, 1.0) {
		t.Errorf("got ratio %.4f, want 1.0", result.ImgRatio)
	}
}

// ---------- 正方形无正方形素材：选横竖中最接近的 ----------

func TestSquareNoSquare(t *testing.T) {
	images := []model.ImageMeta{
		imgMeta(800, 400), // 横图 2.0
		imgMeta(600, 300), // 横图 2.0
		imgMeta(400, 800), // 竖图 0.5
	}
	result := SelectBestImage(300, 300, images) // 目标 1.0
	if !result.Found {
		t.Fatal("should find match")
	}
	// 横图 2.0 偏差 1.0，竖图 0.5 偏差 0.5，选竖图
	if !approx(result.ImgRatio, 0.5) {
		t.Errorf("got ratio %.4f, want 0.5", result.ImgRatio)
	}
}

// ---------- 横图目标但素材全是竖图：兜底 ----------

func TestLandscapeTargetAllPortrait(t *testing.T) {
	images := []model.ImageMeta{
		imgMeta(400, 600), // 竖图
		imgMeta(300, 500), // 竖图
		imgMeta(500, 900), // 竖图
	}
	result := SelectBestImage(1600, 800, images) // 目标 2.0
	if !result.Found {
		t.Fatal("should find match via fallback")
	}
	// 所有候选参与，选 ratio 最大的（最接近 2.0）
	// 400/600=0.667, 300/500=0.6, 500/900=0.556 → 选 0.667
	if !approx(result.ImgRatio, 400.0/600.0) {
		t.Errorf("got ratio %.4f, want %.4f", result.ImgRatio, 400.0/600.0)
	}
}

// ---------- 无图片 ----------

func TestEmptyImages(t *testing.T) {
	result := SelectBestImage(100, 100, nil)
	if result.Found {
		t.Error("should not find any match")
	}
	result = SelectBestImage(100, 100, []model.ImageMeta{})
	if result.Found {
		t.Error("should not find any match")
	}
}

// ---------- 精确匹配 ----------

func TestClassifyDirectionExact(t *testing.T) {
	if got := ClassifyDirection(1.0); got != DirSquare {
		t.Errorf("1.0 should be square")
	}
	if got := ClassifyDirection(2.0); got != DirLandscape {
		t.Errorf("2.0 should be landscape")
	}
	if got := ClassifyDirection(0.5); got != DirPortrait {
		t.Errorf("0.5 should be portrait")
	}
}
