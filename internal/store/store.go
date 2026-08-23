// Package store 提供基于 SQLite 的持久化实现。
// 采用 modernc.org/sqlite 纯 Go 驱动（CGO 无关），支持建表迁移、
// 事务封装与重启恢复语义（游标续传、幂等键）。
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store 数据库访问层。
type Store struct {
	db *sql.DB
}

// Open 打开（或创建）SQLite 数据库并执行迁移。
func Open(path string) (*Store, error) {
	if path == "" {
		path = "echotrack.db"
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close 关闭数据库。
func (s *Store) Close() error {
	return s.db.Close()
}

// DB 返回底层句柄（供事务工具使用）。
func (s *Store) DB() *sql.DB { return s.db }

// migrate 执行建表迁移（幂等）。
func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS arrays (
			id            TEXT PRIMARY KEY,
			name          TEXT NOT NULL,
			sample_rate_hz REAL NOT NULL,
			created_at    TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS elements (
			array_id   TEXT NOT NULL,
			element_no INTEGER NOT NULL,
			name       TEXT NOT NULL,
			x          REAL NOT NULL,
			y          REAL NOT NULL,
			delay_us   REAL NOT NULL,
			PRIMARY KEY (array_id, element_no),
			FOREIGN KEY (array_id) REFERENCES arrays(id)
		)`,
		`CREATE TABLE IF NOT EXISTS batches (
			id               TEXT PRIMARY KEY,
			array_id         TEXT NOT NULL,
			status           TEXT NOT NULL,
			reference_element INTEGER NOT NULL,
			window_cursor    INTEGER NOT NULL DEFAULT 0,
			sample_rate_hz   REAL NOT NULL,
			created_at       TEXT NOT NULL,
			updated_at       TEXT NOT NULL,
			FOREIGN KEY (array_id) REFERENCES arrays(id)
		)`,
		`CREATE TABLE IF NOT EXISTS windows (
			id         TEXT PRIMARY KEY,
			batch_id   TEXT NOT NULL,
			element_no INTEGER NOT NULL,
			seq_no     INTEGER NOT NULL,
			i_json     TEXT NOT NULL,
			q_json     TEXT NOT NULL,
			sample_rate REAL NOT NULL,
			status     TEXT NOT NULL,
			checksum   TEXT NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE (batch_id, element_no, seq_no),
			FOREIGN KEY (batch_id) REFERENCES batches(id)
		)`,
		`CREATE TABLE IF NOT EXISTS corrections (
			id          TEXT PRIMARY KEY,
			batch_id    TEXT NOT NULL,
			window_id   TEXT NOT NULL,
			element_no  INTEGER NOT NULL,
			delay_us    REAL NOT NULL,
			rotation_rad REAL NOT NULL,
			unwrapped_json TEXT NOT NULL,
			applied_at  TEXT NOT NULL,
			FOREIGN KEY (window_id) REFERENCES windows(id)
		)`,
		`CREATE TABLE IF NOT EXISTS tracks (
			id          TEXT PRIMARY KEY,
			batch_id    TEXT NOT NULL,
			element_no  INTEGER NOT NULL,
			status      TEXT NOT NULL,
			phase_json  TEXT NOT NULL,
			time_json   TEXT NOT NULL,
			broken_json TEXT NOT NULL,
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL,
			FOREIGN KEY (batch_id) REFERENCES batches(id)
		)`,
		`CREATE TABLE IF NOT EXISTS segments (
			id         TEXT PRIMARY KEY,
			track_id   TEXT NOT NULL,
			start_idx  INTEGER NOT NULL,
			end_idx    INTEGER NOT NULL,
			label      TEXT NOT NULL,
			author     TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY (track_id) REFERENCES tracks(id)
		)`,
		`CREATE TABLE IF NOT EXISTS annotations (
			id          TEXT PRIMARY KEY,
			batch_id    TEXT NOT NULL,
			target_type TEXT NOT NULL,
			target_id   TEXT NOT NULL,
			note        TEXT NOT NULL,
			author      TEXT NOT NULL,
			created_at  TEXT NOT NULL,
			FOREIGN KEY (batch_id) REFERENCES batches(id)
		)`,
		`CREATE TABLE IF NOT EXISTS interpretations (
			id          TEXT PRIMARY KEY,
			batch_id    TEXT NOT NULL,
			version     INTEGER NOT NULL,
			status      TEXT NOT NULL,
			title       TEXT NOT NULL,
			snapshot_ref TEXT NOT NULL,
			track_count INTEGER NOT NULL,
			created_at  TEXT NOT NULL,
			published_at TEXT,
			UNIQUE (batch_id, version),
			FOREIGN KEY (batch_id) REFERENCES batches(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_windows_batch ON windows(batch_id, element_no)`,
		`CREATE INDEX IF NOT EXISTS idx_tracks_batch ON tracks(batch_id)`,
		`CREATE INDEX IF NOT EXISTS idx_annotations_batch ON annotations(batch_id)`,
		`CREATE INDEX IF NOT EXISTS idx_interpretations_batch ON interpretations(batch_id, status)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec migration: %w (stmt=%s)", err, stmt)
		}
	}
	return nil
}
