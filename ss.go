package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/luabagg/orcgen/v2"
	"golang.org/x/sync/errgroup"
)

const (
	mapsURLForTraffic = "https://www.google.com/maps/@%7f,%7f,1200m/data=!3m1!1e3!5m1!1e1"
	ssHeightMeters    = 1100
	ssWidthMeters     = 1800

	/*
		26.99 ------------
		  |
		  |    LATITUDE (y)
		  |
		26.78 ------------

		  |                   |
		75.65 LONGITUDE (x) 75.92
		  |                   |
	*/
	jaipurNorthWestLatitude  = 26.99
	jaipurSouthEastLatitude  = 26.75
	jaipurNorthWestLongitude = 75.65
	jaipurSouthEastLongitude = 75.94

	maxRoutine      = 10
	metersPerDegree = 111320
	fileNameFmt     = "%v/%v-x%v-y%v.png"
	combFileNameFmt = "%v/%v.png"
)

func takeGridScreenshots(quit <-chan os.Signal) (string, error) {
	nowStr := time.Now().Format("20060102-150405")
	log.Printf("---- taking screenshots for Jaipur at %v ----", nowStr)
	defer log.Println("---- screenshots taken ----")

	// latitude is vertical => y, longitude is horizontal => x
	var x, y int
	lat := addMetersInLatitude(jaipurNorthWestLatitude, ssHeightMeters/2)
	long := addMetersInLongitude(lat, jaipurNorthWestLongitude, ssWidthMeters/2)

	var g errgroup.Group
	g.SetLimit(maxRoutine)
	defer func() {
		if err := g.Wait(); err != nil {
			log.Printf("error in taking screenshot: %v", err)
		}
	}()

	for {
		select {
		case <-quit:
			return "", fmt.Errorf("ctrl+c pressed")
		default:
		}

		latTemp := lat
		longTemp := long
		xTemp := x
		yTemp := y
		g.Go(func() error {
			return takeScreenshot(latTemp, longTemp, xTemp, yTemp, nowStr)
		})

		x += 1
		long = addMetersInLongitude(lat, long, ssWidthMeters)
		if long > jaipurSouthEastLongitude {
			y += 1
			x = 0
			lat = addMetersInLatitude(lat, ssHeightMeters)
			long = addMetersInLongitude(lat, jaipurNorthWestLongitude, ssWidthMeters/2)
		}
		if lat < jaipurSouthEastLatitude {
			break
		}
	}

	return nowStr, nil
}

func takeScreenshot(latitude, longitude float64, x, y int, nowStr string) error {
	if shouldSkip(x, y) {
		log.Printf("skipping screenshot for a low frequency cell [y:%v, x:%v]", y, x)
		return nil
	}

	mapsURL := fmt.Sprintf(mapsURLForTraffic, latitude, longitude)
	log.Printf("taking screenshot for [y:%v, x:%v] latitude: %f, longitude: %f at [%v]",
		y, x, latitude, longitude, mapsURL)

	defer func() {
		if r := recover(); r != nil {
			log.Printf("error: recovered panic: %v", r)
		}
	}()

	h := orcgen.NewHandler(orcgen.ScreenshotConfig{FromSurface: true})
	pngPass, err := orcgen.ConvertWebpage(h, mapsURL)
	if err != nil {
		return fmt.Errorf("error while loading the webpage: %w", err)
	}

	fileName := fmt.Sprintf(fileNameFmt, ssFolder, nowStr, x, y)
	if err := os.WriteFile(fileName, pngPass.File, 0644); err != nil {
		return fmt.Errorf("error while writing the screenshot file: %w", err)
	}

	return nil
}

func addMetersInLatitude(latitude float64, meter int) float64 {
	return latitude - float64(meter)/metersPerDegree
}

func addMetersInLongitude(latitude, longitude float64, meter int) float64 {
	return longitude + float64(meter)/(metersPerDegree*math.Cos(latitude*math.Pi/180))
}
