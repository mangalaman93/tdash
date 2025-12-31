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
	requestTimeout = time.Minute

	createTablePGDDL = `CREATE TABLE IF NOT EXISTS traffic(ss_path TEXT PRIMARY KEY,
		yellow INTEGER, red INTEGER, dark_red INTEGER, ts TIMESTAMP, x INTEGER, y INTEGER);`
	latestSsPathPGSQL  = `SELECT ss_path FROM traffic ORDER BY ss_path COLLATE "C" DESC LIMIT 1`
	insertTrafficPGSQL = `INSERT INTO traffic(ss_path, yellow, red, dark_red, ts, x, y) VALUES($1, $2, $3, $4, $5, $6, $7)`
)

var (
	createIndexesPGDDL = []string{
		// -- Critical
		`CREATE INDEX IF NOT EXISTS idx_traffic_xy_ts_desc ON traffic (x, y, ts DESC);`,
		// Time-range pruning
		`CREATE INDEX IF NOT EXISTS idx_traffic_ts ON traffic (ts);`,
		// Optional (only if history queries dominate)
		`CREATE INDEX IF NOT EXISTS idx_traffic_xy_ts_only ON traffic (x, y, ts);`,
	}
)

func periodicSyncToPG(db *sql.DB, hint chan struct{}, quit <-chan os.Signal, wg *sync.WaitGroup) {
	defer wg.Done()

	pgpool, err := pgxpool.New(context.Background(), os.Getenv("POSTGRES_URL"))
	if err != nil {
		panic(err)
	}

	if _, err := pgpool.Exec(context.Background(), createTablePGDDL); err != nil {
		panic(err)
	}

	for _, ddl := range createIndexesPGDDL {
		if _, err := pgpool.Exec(context.Background(), ddl); err != nil {
			panic(err)
		}
	}

	for {
		select {
		case <-quit:
			log.Println("shutting down PG sync!")
			return

		case <-hint:
			if err := syncLatestSqliteToPG(pgpool, db, hint, quit); err != nil {
				log.Printf("error while syncing sqlite DB to postgres; %v", err)
			}
		}
	}
}

func syncLatestSqliteToPG(pgpool *pgxpool.Pool, db *sql.DB,
	hint chan struct{}, quit <-chan os.Signal) error {

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	var latestSsPathNull sql.NullString
	if err := pgpool.QueryRow(ctx, latestSsPathPGSQL).Scan(&latestSsPathNull); err != nil && err != pgx.ErrNoRows {
		return fmt.Errorf("error in getting latest timestamp: %w", err)
	}
	var latestSsPath string
	if latestSsPathNull.Valid {
		latestSsPath = latestSsPathNull.String
	}

	rows, err := getRecentTraffic(db, latestSsPath)
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
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing pg transaction: %w", err)
	}

	if rowsCount == maxRecentRows {
		hint <- struct{}{}
	}

	log.Printf("synced [%v] rows to postgres", rowsCount)
	return nil
}
