package diff

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestCompareImages(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "diff_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create Image 1 (Red square)
	img1 := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img1.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	p1 := filepath.Join(tmpDir, "img1.png")
	f1, _ := os.Create(p1)
	png.Encode(f1, img1)
	f1.Close()

	// Create Image 2 (Red square with a Blue box inside)
	img2 := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if x >= 20 && x < 40 && y >= 20 && y < 40 {
				img2.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255}) // 20x20 = 400 pixels changed
			} else {
				img2.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			}
		}
	}
	p2 := filepath.Join(tmpDir, "img2.png")
	f2, _ := os.Create(p2)
	png.Encode(f2, img2)
	f2.Close()

	// Compare with threshold 1.0%
	res, diffBytes, err := CompareImages(p1, p2, 1.0)
	if err != nil {
		t.Fatalf("CompareImages failed: %v", err)
	}

	if len(diffBytes) == 0 {
		t.Errorf("expected non-empty diff PNG bytes")
	}

	// 400 pixels out of 10000 = 4.0%
	if res.DiffPercent < 3.5 || res.DiffPercent > 4.5 {
		t.Errorf("expected ~4.0%% diff, got %.2f%%", res.DiffPercent)
	}

	if !res.IsDrifted {
		t.Errorf("expected IsDrifted to be true since 4.0%% > 1.0%% threshold")
	}
}
