package diff

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"

	"k8s-chromedp-synthetics/pkg/types"
)

// CompareImages compares a baseline PNG file against a current PNG file
func CompareImages(baselinePath, currentPath string, toleranceThreshold float64) (*types.VisualDiffResult, []byte, error) {
	baselineFile, err := os.Open(baselinePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open baseline image %s: %w", baselinePath, err)
	}
	defer baselineFile.Close()

	currentFile, err := os.Open(currentPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open current image %s: %w", currentPath, err)
	}
	defer currentFile.Close()

	imgBaseline, _, err := image.Decode(baselineFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode baseline PNG: %w", err)
	}

	imgCurrent, _, err := image.Decode(currentFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode current PNG: %w", err)
	}

	b1 := imgBaseline.Bounds()
	b2 := imgCurrent.Bounds()

	// Calculate maximum bounding dimensions
	maxWidth := int(math.Max(float64(b1.Dx()), float64(b2.Dx())))
	maxHeight := int(math.Max(float64(b1.Dy()), float64(b2.Dy())))

	if maxWidth == 0 || maxHeight == 0 {
		return nil, nil, fmt.Errorf("empty image dimensions")
	}

	diffImg := image.NewRGBA(image.Rect(0, 0, maxWidth, maxHeight))
	draw.Draw(diffImg, diffImg.Bounds(), &image.Uniform{color.RGBA{240, 240, 240, 255}}, image.Point{}, draw.Src)

	var differingPixels int64
	totalPixels := int64(maxWidth * maxHeight)

	// Pixel comparison loop
	for y := 0; y < maxHeight; y++ {
		for x := 0; x < maxWidth; x++ {
			var r1, g1, b1Val, a1 uint32 = 0, 0, 0, 255
			var r2, g2, b2Val, a2 uint32 = 0, 0, 0, 255

			if x < b1.Dx() && y < b1.Dy() {
				r1, g1, b1Val, a1 = imgBaseline.At(b1.Min.X+x, b1.Min.Y+y).RGBA()
			}
			if x < b2.Dx() && y < b2.Dy() {
				r2, g2, b2Val, a2 = imgCurrent.At(b2.Min.X+x, b2.Min.Y+y).RGBA()
			}

			// Calculate RGB Delta
			diffR := math.Abs(float64(r1>>8) - float64(r2>>8))
			diffG := math.Abs(float64(g1>>8) - float64(g2>>8))
			diffB := math.Abs(float64(b1Val>>8) - float64(b2Val>>8))
			diffA := math.Abs(float64(a1>>8) - float64(a2>>8))

			delta := (diffR + diffG + diffB + diffA) / 4.0

			if delta > 15.0 { // Sensitivity threshold
				differingPixels++
				// Highlight differing pixels in vibrant Red / Magenta
				diffImg.Set(x, y, color.RGBA{R: 255, G: 0, B: 100, A: 255})
			} else {
				// Gray out identical pixels for context background
				c := imgCurrent.At(b2.Min.X+x, b2.Min.Y+y)
				r, g, b, _ := c.RGBA()
				gray := uint8((float64(r>>8)*0.299 + float64(g>>8)*0.587 + float64(b>>8)*0.114) * 0.4)
				diffImg.Set(x, y, color.RGBA{R: gray, G: gray, B: gray, A: 180})
			}
		}
	}

	diffPercent := (float64(differingPixels) / float64(totalPixels)) * 100.0

	var buf bytes.Buffer
	if err := png.Encode(&buf, diffImg); err != nil {
		return nil, nil, fmt.Errorf("failed to encode diff image: %w", err)
	}

	isDrifted := diffPercent > toleranceThreshold

	result := &types.VisualDiffResult{
		BaselinePath: baselinePath,
		CurrentPath:  currentPath,
		DiffPercent:  math.Round(diffPercent*100) / 100,
		IsDrifted:    isDrifted,
	}

	return result, buf.Bytes(), nil
}
