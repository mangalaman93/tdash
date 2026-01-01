package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const (
	tmpRodFolder = "/tmp/rod"
	ssTimePeriod = 10 * time.Minute
)

var (
	ssFolder       = "ss"
	maskFolder     = "mask"
	dbFolder       = "db"
	ssCombFolder   = "ss-comb"
	maskCombFolder = "mask-comb"
	isolateFolder  = "ss-iso"
)

func main() {
	ssFolderVar := flag.String("ss-folder", "", "directory storing temp screenshots")
	maskFolderVar := flag.String("mask-folder", "", "directory storing temp masks")
	dbFolderVar := flag.String("db-folder", "", "directory storing db files")
	ssCombFolderVar := flag.String("ss-comb-folder", "", "directory storing combined screenshots")
	maskCombFolderVar := flag.String("mask-comb-folder", "", "directory storing combined masks")
	isolateFolderVar := flag.String("isolate-folder", "", "directory storing isolated grids")

	ss := flag.Bool("ss", false, "take screenshots once and analyze")
	analyzePrefix := flag.String("analyze", "", "analyze existing screenshots with prefix")
	isolate := flag.String("isolate", "", "isolate a particular grid from the map e.g. 0,0")
	flag.Parse()

	ssFolder = getNonEmpty(*ssFolderVar, ssFolder)
	maskFolder = getNonEmpty(*maskFolderVar, maskFolder)
	dbFolder = getNonEmpty(*dbFolderVar, dbFolder)
	ssCombFolder = getNonEmpty(*ssCombFolderVar, ssCombFolder)
	maskCombFolder = getNonEmpty(*maskCombFolderVar, maskCombFolder)
	isolateFolder = getNonEmpty(*isolateFolderVar, isolateFolder)

	if err := createFolders(); err != nil {
		panic(err)
	}

	db, closeDB, err := openDB()
	if err != nil {
		panic(err)
	}
	defer closeDB()

	if err := initDB(db); err != nil {
		panic(err)
	}

	ctrlC := make(chan os.Signal, 1)
	signal.Notify(ctrlC, os.Interrupt)
	defer signal.Stop(ctrlC)

	switch {
	case *ss:
		prefix, err := takeGridScreenshots(ctrlC)
		if err != nil {
			panic(err)
		}
		if err := analyzeScreenshots(prefix, db); err != nil {
			panic(err)
		}

	case *analyzePrefix != "":
		if err := analyzeScreenshots(*analyzePrefix, db); err != nil {
			panic(err)
		}

	case *isolate != "":
		if err := isolateGrid(isolateFolder, *isolate, ctrlC); err != nil {
			panic(err)
		}

	default:
		runPeriodicSync(ctrlC, db)
	}
}

func getNonEmpty(val, defaultVal string) string {
	if val != "" {
		return val
	}
	return defaultVal
}

func createFolders() error {
	if err := os.MkdirAll(ssFolder, 0755); err != nil {
		return fmt.Errorf("error in creating screenshot folder [%v]: %w", ssFolder, err)
	}
	if err := os.MkdirAll(maskFolder, 0755); err != nil {
		return fmt.Errorf("error in creating mask folder [%v]: %w", maskFolder, err)
	}
	if err := os.MkdirAll(dbFolder, 0755); err != nil {
		return fmt.Errorf("error in creating db folder [%v]: %w", dbFolder, err)
	}
	if err := os.MkdirAll(ssCombFolder, 0755); err != nil {
		return fmt.Errorf("error in creating ss comb folder [%v]: %w", ssCombFolder, err)
	}
	if err := os.MkdirAll(maskCombFolder, 0755); err != nil {
		return fmt.Errorf("error in creating mask comb folder [%v]: %w", maskCombFolder, err)
	}
	if err := os.MkdirAll(isolateFolder, 0755); err != nil {
		return fmt.Errorf("error in creating isolate folder [%v]: %w", isolateFolder, err)
	}
	return nil
}

func runPeriodicSync(ctrlC <-chan os.Signal, db *sql.DB) {
	hint := make(chan struct{}, 10)
	quit := make(chan os.Signal, 10)
	hint <- struct{}{}

	var wg sync.WaitGroup
	wg.Add(2)
	go periodicSyncToPG(db, hint, quit, &wg)
	go takePeriodicScreenshots(db, hint, quit, &wg)

	<-ctrlC
	close(quit)
	wg.Wait()
}

func takePeriodicScreenshots(db *sql.DB, hint chan struct{}, quit <-chan os.Signal, wg *sync.WaitGroup) {
	defer wg.Done()

	t := time.NewTicker(ssTimePeriod)
	defer t.Stop()
	skipCount := 0
	for {
		select {
		case <-quit:
			log.Println("shutting down screenshot thread!")
			return

		case <-t.C:
			currentHour := time.Now().UTC().Hour()
			// no screenshot during 2:30 to 6:30 IST, that is 21 to 1 UTC
			if currentHour >= 21 || currentHour <= 1 {
				skipCount = 0
				log.Println("skipping screenshot during 2:30AM to 6:30AM IST")
				continue
			}
			// between 11:30PM to 1:30 AM, take screenshot every half an hour (every 3rd 10min tick)
			// Note that 1:29AM has currentHour=19 in UTC, 1:30AM has currentHour=20 in UTC.
			if currentHour == 18 || currentHour == 19 {
				skipCount++
				if skipCount%3 != 0 {
					log.Printf("skipping screenshot between 11:30PM to 1:30AM IST (skipCount: %d)", skipCount)
					continue
				}
				skipCount = 0 // reset after taking a screenshot
			}

			if err := makeSpaceIfNeeded(); err != nil {
				log.Println(err)
				continue
			}

			prefix, err := takeGridScreenshots(quit)
			if err != nil {
				log.Println(err)
				return // because this means ctrl+c is pressed
			}

			if err := analyzeScreenshots(prefix, db); err != nil {
				log.Println(err)
				continue
			}

			if err := deleteScreenshots(prefix); err != nil {
				log.Println(err)
				continue
			}

			hint <- struct{}{}
		}
	}
}

func deleteScreenshots(prefix string) error {
	log.Printf("deleting screenshots with prefix [%v]...", prefix)

	return filepath.WalkDir(ssFolder, func(ssPath string, d fs.DirEntry, err error) error {
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

		if !strings.Contains(info.Name(), prefix) {
			return nil
		}

		if err := os.Remove(filepath.Join(maskFolder, info.Name())); err != nil {
			return fmt.Errorf("error in removing mask [%v]: %w", info.Name(), err)
		}
		if err := os.Remove(ssPath); err != nil {
			return fmt.Errorf("error in removing screenshot [%v]: %w", info.Name(), err)
		}
		return nil
	})
}

func cleanupTmpRod() error {
	return os.RemoveAll(tmpRodFolder)
}

func makeSpaceIfNeeded() error {
	if err := cleanupTmpRod(); err != nil {
		log.Printf("error in cleaning up tmp rod: %v", err)
	}

	var stat unix.Statfs_t
	if err := unix.Statfs(ssCombFolder, &stat); err != nil {
		return fmt.Errorf("error in getting disk stats for [%v]: %w", ssCombFolder, err)
	}

	availableSpace := uint64(stat.Bavail) * uint64(stat.Bsize) / 1024 / 1024 / 1024
	log.Printf("available space: %vGB", availableSpace)
	if availableSpace > 5 {
		return nil
	}

	entries, err := os.ReadDir(ssCombFolder)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	var filenames []string
	for _, entry := range entries {
		if !strings.Contains(entry.Name(), ".png") {
			continue
		}
		filenames = append(filenames, entry.Name())
	}

	if len(filenames) == 0 {
		return nil
	}

	sort.Strings(filenames)

	fileToDelete := filenames[0]
	log.Printf("deleting screenshots from folders [%v, %v]: [%v]", ssCombFolder, maskCombFolder, fileToDelete)
	if err := os.Remove(filepath.Join(ssCombFolder, fileToDelete)); err != nil {
		return fmt.Errorf("failed to delete file %s: %w", fileToDelete, err)
	}
	if err := os.Remove(filepath.Join(maskCombFolder, fileToDelete)); err != nil {
		return fmt.Errorf("failed to delete file %s: %w", fileToDelete, err)
	}
	return nil
}
