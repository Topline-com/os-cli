package sync

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

const Schema = `
CREATE TABLE IF NOT EXISTS sync_state (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS contacts (
  id TEXT PRIMARY KEY,
  name TEXT,
  email TEXT,
  phone TEXT,
  raw_json TEXT NOT NULL,
  updated_at TEXT
);
CREATE TABLE IF NOT EXISTS opportunities (
  id TEXT PRIMARY KEY,
  contact_id TEXT,
  pipeline_id TEXT,
  pipeline_stage_id TEXT,
  name TEXT,
  status TEXT,
  monetary_value REAL,
  raw_json TEXT NOT NULL,
  updated_at TEXT
);
CREATE TABLE IF NOT EXISTS pipelines (
  id TEXT PRIMARY KEY,
  name TEXT,
  raw_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS pipeline_stages (
  id TEXT PRIMARY KEY,
  pipeline_id TEXT,
  name TEXT,
  raw_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS conversations (
  id TEXT PRIMARY KEY,
  contact_id TEXT,
  raw_json TEXT NOT NULL,
  updated_at TEXT
);
CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  conversation_id TEXT,
  contact_id TEXT,
  type TEXT,
  direction TEXT,
  created_at TEXT,
  raw_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  contact_id TEXT,
  title TEXT,
  due_at TEXT,
  completed INTEGER,
  raw_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS notes (
  id TEXT PRIMARY KEY,
  contact_id TEXT,
  body TEXT,
  created_at TEXT,
  raw_json TEXT NOT NULL
);
`

func SchemaContainsTable(table string) bool {
	needle := "CREATE TABLE IF NOT EXISTS " + table + " "
	return strings.Contains(Schema, needle) || strings.Contains(Schema, "CREATE TABLE IF NOT EXISTS "+table+"\n")
}

func InitDB(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(Schema); err != nil {
		return fmt.Errorf("init sqlite schema: %w", err)
	}
	return nil
}
