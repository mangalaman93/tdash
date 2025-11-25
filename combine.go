package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	numRows     = 21 // y
	numCols     = 15 // x
	imageWidth  = 1280
	imageHeight = 800

	imageToLeaveOnLeft   = 170
	imageToLeaveOnTop    = 230
	imageToLeaveOnRight  = 260
	imageToLeaveOnBottom = 50

	imageWidthWithLeaveOuts  = imageWidth - imageToLeaveOnLeft - imageToLeaveOnRight
	imageHeightWithLeaveOuts = imageHeight - imageToLeaveOnTop - imageToLeaveOnBottom
)

func combineScreenshots(prefix string) error {
	log.Printf("combining screenshots for Jaipur at %v...", prefix)
	return combineImage(ssFolder, ssCombFolder, prefix)
}

func combineMasks(prefix string) error {
	log.Printf("combining masks for Jaipur at %v...", prefix)
	return combineImage(maskFolder, maskCombFolder, prefix)
}

func combineImage(imgFolder, combFolder, prefix string) error {
	combinedImage := image.NewRGBA(image.Rect(0, 0, numCols*imageWidthWithLeaveOuts, numRows*imageHeightWithLeaveOuts))
	for x := range numCols {
		for y := range numRows {
			fileName := fmt.Sprintf(fileNameFmt, imgFolder, prefix, x, y)

			img, err := readImage(fileName)
			if err != nil {
				log.Printf("[combine image] %v", err)
				continue
			}

			minX := x * imageWidthWithLeaveOuts
			minY := y * imageHeightWithLeaveOuts
			rect := image.Rect(minX, minY, minX+imageWidthWithLeaveOuts, minY+imageHeightWithLeaveOuts)
			draw.Draw(combinedImage, rect, img, image.Point{imageToLeaveOnLeft, imageToLeaveOnTop}, draw.Over)
			addCoordinatesToImage(combinedImage, x, y)
		}
	}

	return savePNG(fmt.Sprintf(combFileNameFmt, combFolder, prefix), combinedImage)
}

func readImage(fileName string) (image.Image, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("error opening file [%v]: %w", fileName, err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("error decoding image [%v]: %w", fileName, err)
	}

	return img, nil
}

func savePNG(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("error creating file [%v]: %w", path, err)
	}
	defer file.Close()

	enc := png.Encoder{CompressionLevel: png.BestCompression}
	return enc.Encode(file, img)
}

func cropImage(img *image.Gray) *image.Gray {
	cropRect := image.Rect(imageToLeaveOnLeft, imageToLeaveOnTop, imageToLeaveOnLeft+imageWidthWithLeaveOuts,
		imageToLeaveOnTop+imageHeightWithLeaveOuts)
	return img.SubImage(cropRect).(*image.Gray)
}

func addCoordinatesToImage(img *image.RGBA, x, y int) {
	minX := x * imageWidthWithLeaveOuts
	minY := y * imageHeightWithLeaveOuts

	draw.Draw(img, image.Rect(minX, minY, minX+50, minY+20),
		&image.Uniform{color.RGBA{255, 255, 255, 255}}, image.Point{}, draw.Src)
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.RGBA{0, 0, 0, 255}),
		Face: basicfont.Face7x13,
		Dot:  fixed.Point26_6{X: fixed.Int26_6((minX + 2) * 64), Y: fixed.Int26_6((minY + 13) * 64)},
	}
	d.DrawString(fmt.Sprintf("%v, %v", y, x))
}
