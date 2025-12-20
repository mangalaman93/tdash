package main

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const (
	dbFile = "traffic.db"

	trafficTableDDL  = `CREATE TABLE IF NOT EXISTS traffic(ss_path VARCHAR PRIMARY KEY, yellow INTEGER, red INTEGER, dark_red INTEGER)`
	insertTrafficSQL = `INSERT OR REPLACE INTO traffic(ss_path, yellow, red, dark_red) VALUES(?, ?, ?, ?)`
)

var (
	migrations = []string{
		"ALTER TABLE traffic ADD COLUMN ts TEXT;",
		"ALTER TABLE traffic ADD COLUMN x INTEGER;",
		"ALTER TABLE traffic ADD COLUMN y INTEGER;",
		`CREATE TRIGGER trg_parse_ss_path
			AFTER INSERT ON traffic
			FOR EACH ROW
			BEGIN
				UPDATE traffic
				SET
					ts =
						SUBSTR(NEW.ss_path, 1, 4) || '-' ||
						SUBSTR(NEW.ss_path, 5, 2) || '-' ||
						SUBSTR(NEW.ss_path, 7, 2) || ' ' ||
						SUBSTR(NEW.ss_path, 10, 2) || ':' ||
						SUBSTR(NEW.ss_path, 12, 2) || ':' ||
						SUBSTR(NEW.ss_path, 14, 2),

					x = CAST(
							SUBSTR(
								NEW.ss_path,
								INSTR(NEW.ss_path, '-x') + 2,
								INSTR(NEW.ss_path, '-y') - (INSTR(NEW.ss_path, '-x') + 2)
							) AS INTEGER
						),

					y = CAST(
							SUBSTR(
								NEW.ss_path,
								INSTR(NEW.ss_path, '-y') + 2,
								INSTR(NEW.ss_path, '.png') - (INSTR(NEW.ss_path, '-y') + 2)
							) AS INTEGER
						)

				WHERE ss_path = NEW.ss_path;
			END;`,
	}
)

func initDB(db *sql.DB) error {
	if _, err := db.Exec(trafficTableDDL); err != nil {
		return fmt.Errorf("error in creating table [traffic]: %w", err)
	}

	if err := migrateDB(db); err != nil {
		return fmt.Errorf("error in migrating db: %w", err)
	}

	return nil
}

func migrateDB(db *sql.DB) error {
	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil && !strings.Contains(err.Error(), "duplicate") &&
			!strings.Contains(err.Error(), "already exists") {

			return fmt.Errorf("error in migrating db: %w", err)
		}
	}
	return nil
}

func openDB() (*sql.DB, func(), error) {
	db, err := sql.Open("sqlite3", filepath.Join(dbFolder, dbFile))
	if err != nil {
		return nil, nil, fmt.Errorf("error in opening db [%v]: %w", dbFile, err)
	}

	closeDB := func() {
		if err := db.Close(); err != nil {
			log.Printf("error in closing db [%v]: %v", dbFile, err)
		}
	}

	return db, closeDB, nil
}

func insertTraffic(db *sql.DB, ssPath string, yellow, red, darkRed int) error {
	_, err := db.Exec(insertTrafficSQL, filepath.Base(ssPath), yellow, red, darkRed)
	return err
}
