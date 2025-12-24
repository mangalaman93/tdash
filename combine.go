package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"golang.org/x/sync/errgroup"
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
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("error in closing file [%v]: %v", fileName, err)
		}
	}()

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
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("error in closing file [%v]: %v", path, err)
		}
	}()

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

func isolateGrid(dstFolder, grid string, ctrlC <-chan os.Signal) error {
	gridParts := strings.Split(grid, ",")
	if len(gridParts) != 2 {
		return fmt.Errorf("invalid grid format: [%s]", grid)
	}
	x, err := strconv.Atoi(strings.TrimSpace(gridParts[0]))
	if err != nil {
		return fmt.Errorf("invalid x coordinate: %s", gridParts[0])
	}
	y, err := strconv.Atoi(strings.TrimSpace(gridParts[1]))
	if err != nil {
		return fmt.Errorf("invalid y coordinate: %s", gridParts[1])
	}

	files := []string{}
	log.Printf("isolating screenshots for grid [%v, %v]", x, y)
	if err := filepath.WalkDir(ssCombFolder, func(ssCombPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error in walking dir [%v]: %w", ssCombFolder, err)
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("error in getting file info [%v]: %w", ssCombPath, err)
		}

		if !strings.HasSuffix(info.Name(), ".png") {
			return nil
		}

		files = append(files, ssCombPath)
		return nil
	}); err != nil {
		return fmt.Errorf("error in walking dir for analyzing screenshots [%v]: %w", ssFolder, err)
	}

	var g errgroup.Group
	g.SetLimit(6)
	defer func() {
		if err := g.Wait(); err != nil {
			log.Printf("error in isolating grid [%v, %v]: %v", x, y, err)
		}
	}()

	log.Printf("found %d files for grid [%v, %v]", len(files), x, y)
	for _, file := range files {
		select {
		case <-ctrlC:
			log.Println("shutting down...")
			return nil
		default:
		}

		g.Go(func() error {
			return isolateGridFromCombinedImg(dstFolder, file, x, y)
		})
	}

	log.Printf("completed processing %d files for grid [%v, %v]\n", len(files), x, y)
	return nil
}

func isolateGridFromCombinedImg(dstFolder, combinedImgPath string, x, y int) error {
	log.Printf("processing file: %s", combinedImgPath)

	combinedImg, err := readImage(combinedImgPath)
	if err != nil {
		return fmt.Errorf("error reading combined image [%v]: %w", combinedImgPath, err)
	}

	isolatedImg := image.NewRGBA(image.Rect(0, 0, imageWidthWithLeaveOuts, imageHeightWithLeaveOuts))
	rect := image.Rect(0, 0, imageWidthWithLeaveOuts, imageHeightWithLeaveOuts)
	draw.Draw(isolatedImg, rect, combinedImg, image.Point{x * imageWidthWithLeaveOuts, y * imageHeightWithLeaveOuts}, draw.Over)

	outputPath := filepath.Join(dstFolder, filepath.Base(combinedImgPath))
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("error creating output file [%v]: %w", outputPath, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("error in closing file [%v]: %v", outputPath, err)
		}
	}()

	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(file, isolatedImg); err != nil {
		return fmt.Errorf("error encoding isolated image [%v]: %w", outputPath, err)
	}

	return nil
}
