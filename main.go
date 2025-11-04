package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/go-rod/rod/lib/proto"
	"github.com/luabagg/orcgen/v2"
)

const (
	screenshotFolder  = "ss"
	mapsURLForYadgaar = "https://www.google.com/maps/@26.9170646,75.8158598,600m/data=!3m1!1e3!5m1!1e1"
)

var (
	gViewportConfig = proto.PageViewport{
		X:      0,
		Y:      0,
		Width:  1920.0,
		Height: 1080.0,
		Scale:  1,
	}
)

func main() {
	// Register signal handler
	ctrlC := make(chan os.Signal, 1)
	signal.Notify(ctrlC, os.Interrupt)
	defer signal.Stop(ctrlC)

	t := time.NewTicker(10 * time.Minute)
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
			if err := takeScreenshotForYadgaar(); err != nil {
				log.Println(err)
			}

		case <-ctrlC:
			log.Println("shutting down...")
			return
		}
	}
}

func takeScreenshotForYadgaar() error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("error: recovered panic: %v\n", r)
		}
	}()

	h := orcgen.NewHandler(orcgen.ScreenshotConfig{Clip: &gViewportConfig})
	pngPass, err := orcgen.ConvertWebpage(h, mapsURLForYadgaar)
	if err != nil {
		return fmt.Errorf("error while loading the webpage: %w", err)
	}

	fileName := fmt.Sprintf("%v/yadgaar_%v.png", screenshotFolder, time.Now().Format("20060102-150405"))
	if err := os.WriteFile(fileName, pngPass.File, 0644); err != nil {
		return fmt.Errorf("error while writing the screenshot file: %w", err)
	}

	return nil
}
