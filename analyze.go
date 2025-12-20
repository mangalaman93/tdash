package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	baseDarkRed = color.RGBA{169, 39, 39, 255}  // #A92727
	baseRed     = color.RGBA{242, 78, 66, 255}  // #F24E42
	baseYellow  = color.RGBA{255, 207, 67, 255} // #FFCF43
	threshold   = uint8(10)

	yellowValueInMask  = uint8(100)
	redValueInMask     = uint8(178)
	darkRedValueInMask = uint8(255)
)

func analyzeScreenshots(prefix string, db *sql.DB) error {
	log.Printf("---- analyzing screenshots for Jaipur at %v ----", prefix)
	defer log.Println("---- screenshots analyzed ----")

	if err := filepath.WalkDir(ssFolder, func(ssPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error in walking dir [%v]: %w", ssFolder, err)
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("error in getting file info [%v]: %w", ssPath, err)
		}

		if !strings.HasSuffix(info.Name(), ".png") {
			return nil
		}

		if !strings.Contains(info.Name(), prefix) {
			return nil
		}

		maskPath := filepath.Join(maskFolder, info.Name())
		return analyzeScreenshot(ssPath, maskPath, db)
	}); err != nil {
		return fmt.Errorf("error in walking dir for analyzing screenshots [%v]: %w", ssFolder, err)
	}

	if err := combineScreenshots(prefix); err != nil {
		return fmt.Errorf("error in combining screenshots [%v]: %w", prefix, err)
	}

	if err := combineMasks(prefix); err != nil {
		return fmt.Errorf("error in combining masks [%v]: %w", prefix, err)
	}

	return nil
}

func analyzeScreenshot(ssPath, maskPath string, db *sql.DB) error {
	pngData, err := os.ReadFile(ssPath)
	if err != nil {
		return fmt.Errorf("error in reading the screenshot file [%v]: %w", ssPath, err)
	}

	maskImg, err := computeMask(pngData)
	if err != nil {
		return fmt.Errorf("error in computing mask for [%v]: %w", ssPath, err)
	}

	if err := saveMaskImage(maskPath, maskImg); err != nil {
		return fmt.Errorf("error in saving mask [%v]: %w", maskPath, err)
	}

	yellowCount, redCount, darkRedCount := computeTraffic(maskImg)
	if err := insertTraffic(db, ssPath, yellowCount, redCount, darkRedCount); err != nil {
		return fmt.Errorf("error in inserting traffic [%v]: %w", ssPath, err)
	}

	return nil
}

func computeMask(pngFile []byte) (*image.Gray, error) {
	img, _, err := image.Decode(bytes.NewReader(pngFile))
	if err != nil {
		return nil, fmt.Errorf("error decoding png image: %w", err)
	}

	mask := image.NewGray(img.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)
			if colorClose(r8, g8, b8, baseDarkRed, threshold) {
				mask.SetGray(x, y, color.Gray{darkRedValueInMask})
			} else if colorClose(r8, g8, b8, baseRed, threshold) {
				mask.SetGray(x, y, color.Gray{redValueInMask})
			} else if colorClose(r8, g8, b8, baseYellow, threshold) {
				mask.SetGray(x, y, color.Gray{yellowValueInMask})
			} else {
				mask.SetGray(x, y, color.Gray{0})
			}
		}
	}

	return mask, nil
}

func saveMaskImage(maskPath string, mask *image.Gray) error {
	outfile, err := os.Create(maskPath)
	if err != nil {
		return fmt.Errorf("error in creating mask file [%v]: %w", maskPath, err)
	}
	defer outfile.Close()

	if err = png.Encode(outfile, mask); err != nil {
		return fmt.Errorf("error in encoding mask [%v]: %w", maskPath, err)
	}

	return nil
}

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

func computeTraffic(img *image.Gray) (int, int, int) {
	croppedImg := cropImage(img)

	yellowCount := 0
	redCount := 0
	darkRedCount := 0
	for x := range croppedImg.Bounds().Dx() {
		for y := range croppedImg.Bounds().Dy() {
			gray := croppedImg.GrayAt(x, y).Y
			switch gray {
			case yellowValueInMask:
				yellowCount++
			case redValueInMask:
				redCount++
			case darkRedValueInMask:
				darkRedCount++
			}
		}
	}
	return yellowCount, redCount, darkRedCount
}
