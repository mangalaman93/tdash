package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	screenshotFolder = "ss"
	maskFolder       = "mask"

	mapsURLForYadgaar   = "https://www.google.com/maps/@%7f,%7f,1200m/data=!3m1!1e3!5m1!1e1"
	metersInImageHeight = 1100
	metersInImageWidth  = 1800
)

var (
	dryRun = false
)

func main() {
	processExistingFiles := flag.Bool("process-existing-files", false, "process existing screenshot files")
	dryRunParam := flag.Bool("dry-run", false, "dry run")
	runOnce := flag.Bool("run-once", false, "run once")
	flag.Parse()
	dryRun = *dryRunParam

	switch {
	case *processExistingFiles:
		if err := processScreenshots(); err != nil {
			panic(err)
		}

	case *runOnce:
		if err := takeGridScreenshots(); err != nil {
			panic(err)
		}

	default:
		takeScreenshotsForJaipur()
	}
}

func processScreenshots() error {
	return filepath.WalkDir(screenshotFolder, func(ssPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error in walking dir [%v]: %w", screenshotFolder, err)
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

		maskPath := filepath.Join(maskFolder, info.Name())
		if exists, err := fileExists(maskPath); err != nil {
			return fmt.Errorf("error checking file [%v] exists: %w", maskPath, err)
		} else if exists {
			return nil
		}

		return processScreenshot(ssPath, maskPath)
	})
}

func fileExists(filePath string) (bool, error) {
	if _, err := os.Open(filePath); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, fmt.Errorf("error in finding file [%v]: %w", filePath, err)
	}
}

func processScreenshot(ssPath, maskPath string) error {
	log.Printf("Processing screenshot [%v]...\n", ssPath)

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

	return nil
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
