package matcher

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"pdf-replace/model"
)

// ---------- 方向判定 ----------

// ImgDirection 表示图片方向
type ImgDirection int

const (
	DirLandscape ImgDirection = iota // 横图: W/H > 1
	DirPortrait                      // 竖图: W/H < 1
	DirSquare                        // 正方形: W/H ≈ 1
)

const squareTolerance = 0.01

// ClassifyDirection 根据宽高比判定方向
func ClassifyDirection(ratio float64) ImgDirection {
	if math.Abs(ratio-1) <= squareTolerance {
		return DirSquare
	}
	if ratio > 1 {
		return DirLandscape
	}
	return DirPortrait
}

// ---------- 核心匹配算法 ----------

// MatchResult 素材匹配结果
type MatchResult struct {
	Image      model.ImageMeta // 选中的素材图片
	Found      bool            // 是否找到
	TargetW    float64         // 目标占位框宽
	TargetH    float64         // 目标占位框高
	ImgRatio   float64         // 选中图片的宽高比
	TargetRatio float64        // 目标宽高比
}

// SelectBestImage 从素材图片列表中选出最合适的图片
// targetW, targetH: 占位框尺寸（可见宽/可见长）
// images: 素材图片列表
func SelectBestImage(targetW, targetH float64, images []model.ImageMeta) MatchResult {
	result := MatchResult{
		TargetW:     targetW,
		TargetH:     targetH,
		TargetRatio: targetW / targetH,
	}

	if len(images) == 0 {
		return result
	}

	// 规则: 只有一张图片直接使用
	if len(images) == 1 {
		img := images[0]
		result.Image = img
		result.Found = true
		result.ImgRatio = img.Width / img.Height
		return result
	}

	targetRatio := targetW / targetH
	targetDir := ClassifyDirection(targetRatio)

	var candidates []model.ImageMeta

	switch targetDir {
	case DirLandscape:
		// 只匹配横图
		for _, img := range images {
			if img.Width/img.Height > 1 {
				candidates = append(candidates, img)
			}
		}
	case DirPortrait:
		// 只匹配竖图
		for _, img := range images {
			if img.Width/img.Height < 1 {
				candidates = append(candidates, img)
			}
		}
	case DirSquare:
		// 先找正方形
		for _, img := range images {
			imgRatio := img.Width / img.Height
			if ClassifyDirection(imgRatio) == DirSquare {
				candidates = append(candidates, img)
			}
		}
		if len(candidates) > 0 {
			return pickClosest(candidates, targetRatio, result)
		}
		// 正方形但无正方形素材：横图和竖图各选最接近的
		var lands, ports []model.ImageMeta
		for _, img := range images {
			imgRatio := img.Width / img.Height
			if imgRatio > 1 {
				lands = append(lands, img)
			} else if imgRatio < 1 {
				ports = append(ports, img)
			}
		}
		if len(lands) > 0 && len(ports) > 0 {
			bestLand := pickClosest(lands, targetRatio, MatchResult{})
			bestPort := pickClosest(ports, targetRatio, MatchResult{})
			// 比较横竖哪个更接近
			landDiff := math.Abs(bestLand.ImgRatio - targetRatio)
			portDiff := math.Abs(bestPort.ImgRatio - targetRatio)
			if landDiff <= portDiff {
				return bestLand
			}
			return bestPort
		}
		// 只有一个方向有素材，降级到该方向
		if len(lands) > 0 {
			candidates = lands
		} else if len(ports) > 0 {
			candidates = ports
		}
	}

	// 无同方向候选：所有图片参与（兜底）
	if len(candidates) == 0 {
		candidates = images
	}

	return pickClosest(candidates, targetRatio, result)
}

// pickClosest 从候选中选宽高比最接近的一张
func pickClosest(candidates []model.ImageMeta, targetRatio float64, base MatchResult) MatchResult {
	best := base
	bestDiff := math.MaxFloat64

	for _, img := range candidates {
		imgRatio := img.Width / img.Height
		diff := math.Abs(imgRatio - targetRatio)
		if diff < bestDiff {
			bestDiff = diff
			best.Image = img
			best.Found = true
			best.ImgRatio = imgRatio
		}
	}
	return best
}

// ---------- 灯位备注 → file-data key 模糊匹配 ----------

// cleanKey 清理字符串：去除空格、全角转半角、转小写
func cleanKey(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '　': // 全角空格
			return ' '
		case r >= '０' && r <= '９':
			return '０' + (r - '０')
		case r >= 'Ａ' && r <= 'Ｚ':
			return 'Ａ' + (r - 'Ａ')
		case r >= 'ａ' && r <= 'ｚ':
			return 'ａ' + (r - 'ａ')
		case unicode.Is(unicode.Han, r):
			return r
		case unicode.IsSpace(r):
			return -1 // 移除空格
		default:
			return r
		}
	}, s)
	return strings.ToLower(strings.ReplaceAll(s, " ", ""))
}

// MatchFileDataKey 将灯位备注匹配到 file-data 的 key
// 例如: "SKINCARE-人脸-2605" → "SKIN CARE-人脸-2605.pdf"
// 先尝试精确匹配（忽略大小写+忽略空格），再尝试模糊
func MatchFileDataKey(lampNote string, fileData model.FileData) (string, bool) {
	cleanedNote := cleanKey(lampNote)

	for key := range fileData {
		cleanedKey := cleanKey(key)
		if cleanedKey == cleanedNote {
			return key, true
		}
		// PDF 文件名以 .pdf 结尾，有种情况 key 可能已包含 note
		keyNoExt := strings.TrimSuffix(cleanedKey, ".pdf")
		if keyNoExt == cleanedNote {
			return key, true
		}
	}

	// 第二次尝试：Levenshtein 距离 ≤ 2
	for key := range fileData {
		cleanedKey := cleanKey(key)
		keyNoExt := strings.TrimSuffix(cleanedKey, ".pdf")
		if levenshtein(keyNoExt, cleanedNote) <= 2 {
			return key, true
		}
	}

	return "", false
}

// levenshtein 计算编辑距离
func levenshtein(a, b string) int {
	la, lb := utf8.RuneCountInString(a), utf8.RuneCountInString(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	ra := make([]rune, 0, la)
	for _, r := range a {
		ra = append(ra, r)
	}
	rb := make([]rune, 0, lb)
	for _, r := range b {
		rb = append(rb, r)
	}

	// 优化为 O(min(la,lb)) 空间
	if la < lb {
		ra, rb = rb, ra
		la, lb = lb, la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}
