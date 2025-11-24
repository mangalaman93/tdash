package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
)

var (
	baseColor = color.RGBA{169, 39, 39, 255} // #A92727
	threshold = uint8(15)
)

func computeMask(pngFile []byte) (*image.Gray, error) {
	img, _, err := image.Decode(bytes.NewReader(pngFile))
	if err != nil {
		return nil, fmt.Errorf("error decoding png image: %w", err)
	}

	// Create a new binary mask (same bounds as original)
	mask := image.NewGray(img.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)
			if colorClose(r8, g8, b8, baseColor, threshold) {
				mask.SetGray(x, y, color.Gray{255})
			} else {
				mask.SetGray(x, y, color.Gray{0})
			}
		}
	}

	return mask, nil
}

// colorClose checks if the pixel is close to the target color within the threshold
func colorClose(r, g, b uint8, target color.RGBA, threshold uint8) bool {
	return absdiff(r, target.R) <= threshold &&
		absdiff(g, target.G) <= threshold &&
		absdiff(b, target.B) <= threshold
}

func absdiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}
