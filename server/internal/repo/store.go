// Package repo owns all SQL. Domain services never issue SQL directly.
package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
)

var tables = []string{"users", "classes", "class_members", "devices", "messages", "homeworks", "student_calls", "activation_codes", "collab_invites", "sessions", "phrases", "audio", "audit_logs", "settings", "schools"}

type Store struct {
	db    *sql.DB
	saved Rows // Last committed rows; never mutated in place.
}

func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, saved: Rows{}}
	for _, statement := range []string{"PRAGMA journal_mode=WAL", "PRAGMA synchronous=NORMAL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
		if _, err = db.Exec(statement); err != nil {
			db.Close()
			return nil, err
		}
	}
	for _, table := range tables {
		q := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (id TEXT PRIMARY KEY, body TEXT NOT NULL CHECK(json_valid(body)), updated_at TEXT NOT NULL DEFAULT (strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ','now')))`, table)
		if _, err = db.Exec(q); err != nil {
			db.Close()
			return nil, err
		}
		s.saved[table] = map[string]string{}
	}
	for _, q := range []string{`CREATE UNIQUE INDEX IF NOT EXISTS users_username ON users(json_extract(body,'$.username'))`, `CREATE INDEX IF NOT EXISTS classes_org ON classes(json_extract(body,'$.org_id'))`, `CREATE INDEX IF NOT EXISTS devices_class ON devices(json_extract(body,'$.class_id'))`, `CREATE INDEX IF NOT EXISTS messages_created ON messages(json_extract(body,'$.created_at'))`, `CREATE UNIQUE INDEX IF NOT EXISTS student_calls_request ON student_calls(json_extract(body,'$.request_id'))`, `CREATE INDEX IF NOT EXISTS student_calls_class ON student_calls(json_extract(body,'$.class_id'))`, `CREATE INDEX IF NOT EXISTS homeworks_class ON homeworks(json_extract(body,'$.class_id'))`, `CREATE INDEX IF NOT EXISTS members_class ON class_members(json_extract(body,'$.class_id'))`} {
		if _, err = db.Exec(q); err != nil {
			db.Close()
			return nil, err
		}
	}
	return s, nil
}

// Rows is a whole database: table name → row ID → JSON body.
type Rows map[string]map[string]string

// Load reads every table and decodes it into dst.
func (s *Store) Load(ctx context.Context, dst any) error {
	saved := Rows{}
	for _, table := range tables {
		saved[table] = map[string]string{}
		rows, err := s.db.QueryContext(ctx, "SELECT id,body FROM "+table)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id, body string
			if err = rows.Scan(&id, &body); err != nil {
				rows.Close()
				return err
			}
			saved[table][id] = body
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	s.saved = saved
	return Decode(saved, dst)
}

// Restore decodes the last committed state into dst without touching the
// database, which may be the reason a commit failed.
func (s *Store) Restore(dst any) error { return Decode(s.saved, dst) }

// Decode turns rows into the structure whose JSON fields name the tables.
func Decode(rows Rows, dst any) error {
	state := map[string]map[string]json.RawMessage{}
	for table, items := range rows {
		state[table] = make(map[string]json.RawMessage, len(items))
		for id, body := range items {
			state[table][id] = json.RawMessage(body)
		}
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

// Save writes only the rows that changed since the last commit.
func (s *Store) Save(ctx context.Context, rows Rows) error { return s.save(ctx, rows, false) }

// Replace atomically replaces every table, including rows whose unique keys
// belong to different IDs in the previous state.
func (s *Store) Replace(ctx context.Context, rows Rows) error { return s.save(ctx, rows, true) }

func (s *Store) save(ctx context.Context, rows Rows, replace bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	next := Rows{}
	for _, table := range tables {
		if replace {
			if _, err = tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
				return err
			}
		}
		next[table] = rows[table]
		if next[table] == nil {
			next[table] = map[string]string{}
		}
		for id, body := range next[table] {
			if !replace && s.saved[table][id] == body {
				continue
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO "+table+" (id,body) VALUES (?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')", id, body); err != nil {
				return err
			}
		}
		if replace {
			continue
		}
		for id := range s.saved[table] {
			if _, exists := next[table][id]; !exists {
				if _, err = tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE id=?", id); err != nil {
					return err
				}
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.saved = next
	return nil
}

func (s *Store) Close() error { return s.db.Close() }
