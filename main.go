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
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/sys/unix"
)

const (
	ssFolder       = "ss"
	maskFolder     = "mask"
	dbFolder       = "db"
	ssCombFolder   = "ss-comb"
	maskCombFolder = "mask-comb"
	tmpRodFolder   = "/tmp/rod"

	dbFile          = "traffic.db"
	trafficTableDDL = `CREATE TABLE IF NOT EXISTS traffic(ss_path VARCHAR PRIMARY KEY, yellow INTEGER, red INTEGER, dark_red INTEGER)`

	ssTimePeriod = 10 * time.Minute
)

func main() {
	ss := flag.Bool("ss", false, "take screenshots once and analyze")
	analyzePrefix := flag.String("analyze", "", "analyze existing screenshots with prefix")
	flag.Parse()

	if err := createFolders(); err != nil {
		panic(err)
	}

	if err := initDB(); err != nil {
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
		if err := analyzeScreenshots(prefix); err != nil {
			panic(err)
		}

	case *analyzePrefix != "":
		if err := analyzeScreenshots(*analyzePrefix); err != nil {
			panic(err)
		}

	default:
		takePeriodicScreenshots(ctrlC)
	}
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
	return nil
}

func initDB() error {
	db, err := sql.Open("sqlite3", filepath.Join(dbFolder, dbFile))
	if err != nil {
		return fmt.Errorf("error in opening db [%v]: %w", dbFile, err)
	}
	defer db.Close()

	if _, err := db.Exec(trafficTableDDL); err != nil {
		return fmt.Errorf("error in creating table [traffic]: %w", err)
	}

	return nil
}

func takePeriodicScreenshots(ctrlC <-chan os.Signal) {
	t := time.NewTicker(ssTimePeriod)
	defer t.Stop()
	for {
		select {
		case <-ctrlC:
			log.Println("shutting down...")
			return

		case <-t.C:
			// no screenshot during 23:30 to 6:30 IST, that is 18 to 1 UTC
			if time.Now().UTC().Hour() >= 18 || time.Now().UTC().Hour() <= 0 {
				log.Println("skipping screenshot during 23:30 to 6:30 IST")
				continue
			}

			if err := makeSpaceIfNeeded(); err != nil {
				log.Println(err)
				continue
			}

			prefix, err := takeGridScreenshots(ctrlC)
			if err != nil {
				log.Println(err)
				return // because this means ctrl+c is pressed
			}

			if err := analyzeScreenshots(prefix); err != nil {
				log.Println(err)
				continue
			}

			if err := deleteScreenshots(prefix); err != nil {
				log.Println(err)
				continue
			}
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
