package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxRowsForSync = 100
	requestTimeout = time.Minute

	createTablePGDDL = `CREATE TABLE IF NOT EXISTS traffic(ss_path TEXT PRIMARY KEY,
		yellow INTEGER, red INTEGER, dark_red INTEGER, ts TIMESTAMP, x INTEGER, y INTEGER);`
	latestTimestampSQL = `SELECT ts FROM traffic ORDER BY ts DESC LIMIT 1`
	insertTrafficPGSQL = `INSERT INTO traffic(ss_path, yellow, red, dark_red, ts, x, y) VALUES($1, $2, $3, $4, $5, $6, $7)`
)

func periodicSyncToPG(db *sql.DB, hint chan struct{}, quit <-chan os.Signal, wg *sync.WaitGroup) error {
	defer wg.Done()

	pgpool, err := pgxpool.New(context.Background(), os.Getenv("POSTGRES_URL"))
	if err != nil {
		return fmt.Errorf("error in opening postgres: %w", err)
	}

	if _, err := pgpool.Exec(context.Background(), createTablePGDDL); err != nil {
		return fmt.Errorf("error in creating table [traffic]: %w", err)
	}

	for {
		select {
		case <-quit:
			log.Println("shutting down PG sync!")
			return nil

		case <-hint:
			log.Println("syncing to PG!")
			if err := syncLatestSqliteToPG(pgpool, db, quit); err != nil {
				log.Printf("error while syncing sqlite DB to postgres; %v", err)
			}
		}
	}
}

func syncLatestSqliteToPG(pgpool *pgxpool.Pool, db *sql.DB, quit <-chan os.Signal) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	var latestTS time.Time
	if err := pgpool.QueryRow(ctx, latestTimestampSQL).Scan(&latestTS); err != nil {
		if err == pgx.ErrNoRows {
			latestTS = time.Time{}
		} else {
			return fmt.Errorf("error in getting latest timestamp: %w", err)
		}
	}

	rows, err := getRecentTraffic(db, latestTS.Format("2006-01-02 15:04:05"))
	if err != nil {
		return fmt.Errorf("error in getting recent traffic: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("error in closing rows: %v", err)
		}
	}()

	tx, err := pgpool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error starting pg transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	rowsCount := 0
loop:
	for rows.Next() {
		select {
		case <-quit:
			break loop
		default:
		}

		if err := rows.Err(); err != nil {
			return fmt.Errorf("sqlite rows iteration error: %w", err)
		}

		var ssPath string
		var x, y, yellow, red, darkRed int
		var ts string
		if err = rows.Scan(&ssPath, &yellow, &red, &darkRed, &ts, &x, &y); err != nil {
			return fmt.Errorf("error scanning sqlite row: %w", err)
		}

		if _, err := tx.Exec(ctx, insertTrafficPGSQL, ssPath, yellow, red, darkRed, ts, x, y); err != nil {
			return fmt.Errorf("error inserting into postgres: %w", err)
		}

		rowsCount++
		if rowsCount < maxRowsForSync {
			continue
		}

		ctx, cancel = context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("error committing pg transaction: %w", err)
		}

		tx, err = pgpool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("error starting pg transaction: %w", err)
		}

		log.Printf("synced [%v] rows to postgres", rowsCount)
		rowsCount = 0
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing pg transaction: %w", err)
	}
	return nil
}
