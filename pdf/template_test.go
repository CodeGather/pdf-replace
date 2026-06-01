package pdf

import (
	"testing"
)

func TestOpenTemplate(t *testing.T) {
	pdfPath := "/Users/Yau/Library/Application Support/com.lorealchina.mplus/guide/Ysl/上海七宝领展广场BD精品店_164.pdf"

	tmpl, err := OpenTemplate(pdfPath)
	if err != nil {
		t.Fatalf("OpenTemplate failed: %v", err)
	}
	defer tmpl.Close()

	imgs := tmpl.ImagePositions()
	if len(imgs) == 0 {
		t.Error("no images found")
	} else {
		t.Logf("Found %d images", len(imgs))
	}
}

func TestFindImageByRect(t *testing.T) {
	pdfPath := "/Users/Yau/Library/Application Support/com.lorealchina.mplus/guide/Ysl/上海七宝领展广场BD精品店_164.pdf"
	tmpl, err := OpenTemplate(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	defer tmpl.Close()

	// 匹配 V1: image=(736, 213, 33, 61), page=(841.89, 595.276)
	// 预期 PDF 中 Im10: x=736.43 y=213.49 w=32.69 h=60.93
	img := tmpl.FindImageByRect(736, 213, 33, 61, 5)
	if img == nil {
		t.Error("V1 image not found")
	} else {
		t.Logf("V1 matched: obj=%d name=%s", img.ObjNr, img.Name)
	}
}
