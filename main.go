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
	screenshotFolder = "ss"
	mapsURL          = "https://www.google.com/maps/@26.9170646,75.8158598,600m/data=!3m1!1e3!5m1!1e1"
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
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	t := time.NewTicker(5 * time.Minute)
	for {
		select {
		case <-t.C:
			if err := takeScreenshot(); err != nil {
				log.Print(err)
			}

		case <-signalChan:
			log.Println("Shutting down...")
			return
		}
	}
}

func takeScreenshot() error {
	h := orcgen.NewHandler(orcgen.ScreenshotConfig{Clip: &gViewportConfig})
	pngPass, err := orcgen.ConvertWebpage(h, mapsURL)
	if err != nil {
		return fmt.Errorf("error while loading the webpage: %w", err)

	}

	fileName := fmt.Sprintf("%v/yadgaar_%v.png", screenshotFolder, time.Now().Format("20060102-150405"))
	if err := os.WriteFile(fileName, pngPass.File, 0644); err != nil {
		return fmt.Errorf("error while writing the screenshot file: %w", err)
	}

	return nil
}
