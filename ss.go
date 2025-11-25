package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"time"

	"github.com/luabagg/orcgen/v2"
	"golang.org/x/sync/errgroup"
)

const (
	fileNameFmt  = "%v/%v-x%v-y%v.png"
	ssTimePeriod = 10 * time.Minute

	jaipurNorthWestLatitude  = 26.99
	jaipurSouthEastLatitude  = 26.78
	jaipurNorthWestLongitude = 75.65
	jaipurSouthEastLongitude = 75.92

	maxRoutine      = 10
	metersPerDegree = 111320

	//
	//	26.99 ------------
	//	  |
	//	  |    LATITUDE (y)
	//	  |
	//	26.78 ------------
	//
	//	  |                   |
	//	75.65 LONGITUDE (x) 75.92
	//	  |                   |
	//
)

func takeScreenshotsForJaipur() {
	// Register signal handler
	ctrlC := make(chan os.Signal, 1)
	signal.Notify(ctrlC, os.Interrupt)
	defer signal.Stop(ctrlC)

	t := time.NewTicker(ssTimePeriod)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			// no screenshot during 23:30 to 6:30 IST, that is 18 to 1 UTC
			if time.Now().UTC().Hour() >= 18 || time.Now().UTC().Hour() <= 0 {
				log.Println("skipping screenshot during 23:30 to 6:30 IST")
				continue
			}

			// Take screenshot for Yadgaar junction
			if err := takeGridScreenshots(); err != nil {
				log.Println(err)
			}

		case <-ctrlC:
			log.Println("shutting down...")
			return
		}
	}
}

func takeGridScreenshots() error {
	nowStr := time.Now().Format("20060102-150405")
	log.Printf("---- taking screenshots for Jaipur at %v ----\n", nowStr)
	defer log.Println("---- screenshots taken ----")

	// latitude is vertical => y, longitude is horizontal => x
	var x, y int
	lat := addMetersInLatitude(jaipurNorthWestLatitude, metersInImageHeight/2)
	long := addMetersInLongitude(lat, jaipurNorthWestLongitude, metersInImageWidth/2)

	var g errgroup.Group
	g.SetLimit(maxRoutine)

loop:
	for {
		for {
			latTemp := lat
			longTemp := long
			xTemp := x
			yTemp := y
			g.Go(func() error {
				return takeScreenshot(latTemp, longTemp, xTemp, yTemp, nowStr)
			})

			x += 1
			long = addMetersInLongitude(lat, long, metersInImageWidth)
			if long > jaipurSouthEastLongitude {
				y += 1
				x = 0
				lat = addMetersInLatitude(lat, metersInImageHeight)
				long = addMetersInLongitude(lat, jaipurNorthWestLongitude, metersInImageWidth/2)
			}
			if lat < jaipurSouthEastLatitude {
				break loop
			}
		}
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("error in taking screenshot: %w", err)
	}

	return nil
}

func takeScreenshot(latitude, longitude float64, x, y int, nowStr string) error {
	mapsURL := fmt.Sprintf(mapsURLForYadgaar, latitude, longitude)
	log.Printf("taking screenshot for [y:%v, x:%v] latitude: %f, longitude: %f at [%v]\n",
		y, x, latitude, longitude, mapsURL)

	defer func() {
		if r := recover(); r != nil {
			log.Printf("error: recovered panic: %v\n", r)
		}
	}()

	if dryRun {
		return nil
	}

	h := orcgen.NewHandler(orcgen.ScreenshotConfig{FromSurface: true})
	pngPass, err := orcgen.ConvertWebpage(h, mapsURL)
	if err != nil {
		return fmt.Errorf("error while loading the webpage: %w", err)
	}

	fileName := fmt.Sprintf(fileNameFmt, screenshotFolder, nowStr, x, y)
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
