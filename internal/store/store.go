// Package store keeps users and saved games in SQLite. Each game is stored
// as its SGF record plus a JSON metadata blob.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"easygo/internal/session"
)

type Store struct{ db *sql.DB }

// User is a signed-in player.
type User struct {
	ID       string `json:"id"`
	Email    string `json:"email,omitempty"`
	Name     string `json:"name"`
	Picture  string `json:"picture,omitempty"`
	Rank     string `json:"rank"`
	Location string `json:"location"`
	Provider string `json:"provider"`
}

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  picture TEXT NOT NULL DEFAULT '',
  rank TEXT NOT NULL DEFAULT '',
  location TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL,
  created INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS games (
  id TEXT PRIMARY KEY,
  owner TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '',
  mode TEXT NOT NULL,
  status TEXT NOT NULL,
  updated INTEGER NOT NULL,
  meta TEXT NOT NULL,
  sgf TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS games_owner ON games(owner);
CREATE INDEX IF NOT EXISTS games_status ON games(status);
`

// Open opens (creating if needed) the database at path. Use ":memory:" in tests.
func Open(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		dsn = "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection keeps ":memory:" databases coherent and avoids writer contention.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

var ErrNotFound = errors.New("not found")

// UpsertUser inserts a user or refreshes the profile fields Google owns
// (name, picture, email) while keeping the rank and location the player set.
func (s *Store) UpsertUser(u User) error {
	_, err := s.db.Exec(`
INSERT INTO users (id, email, name, picture, rank, location, provider, created)
VALUES (?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET email=excluded.email, name=excluded.name, picture=excluded.picture`,
		u.ID, u.Email, u.Name, u.Picture, u.Rank, u.Location, u.Provider, time.Now().Unix())
	return err
}

func (s *Store) User(id string) (User, error) {
	var u User
	err := s.db.QueryRow(`SELECT id,email,name,picture,rank,location,provider FROM users WHERE id=?`, id).
		Scan(&u.ID, &u.Email, &u.Name, &u.Picture, &u.Rank, &u.Location, &u.Provider)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// UpdateProfile sets the rank and location a player shows on their card.
func (s *Store) UpdateProfile(id, name, rank, location string) error {
	res, err := s.db.Exec(`UPDATE users SET name=?, rank=?, location=? WHERE id=?`, name, rank, location, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SaveGame writes the current state of a game.
func (s *Store) SaveGame(g *session.Game) error {
	meta, record := g.Snapshot()
	blob, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO games (id, owner, name, mode, status, updated, meta, sgf)
VALUES (?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET owner=excluded.owner, name=excluded.name, mode=excluded.mode,
  status=excluded.status, updated=excluded.updated, meta=excluded.meta, sgf=excluded.sgf`,
		meta.ID, meta.OwnerID, meta.Name, string(meta.Mode), string(meta.Status), meta.Updated.Unix(), string(blob), record)
	return err
}

// LoadGame restores one game.
func (s *Store) LoadGame(id string) (*session.Game, error) {
	var blob, record string
	err := s.db.QueryRow(`SELECT meta, sgf FROM games WHERE id=?`, id).Scan(&blob, &record)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return restore(blob, record)
}

func restore(blob, record string) (*session.Game, error) {
	var m session.Meta
	if err := json.Unmarshal([]byte(blob), &m); err != nil {
		return nil, err
	}
	return session.Restore(m, record)
}

// LoadActive returns every game that is not finished, for server startup.
func (s *Store) LoadActive() ([]*session.Game, error) {
	rows, err := s.db.Query(`SELECT meta, sgf FROM games WHERE status != 'finished'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*session.Game
	for rows.Next() {
		var blob, record string
		if err := rows.Scan(&blob, &record); err != nil {
			return nil, err
		}
		g, err := restore(blob, record)
		if err != nil {
			continue // one corrupt record must not stop the server starting
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GameSummary is a row in the game lists.
type GameSummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Mode    string `json:"mode"`
	Status  string `json:"status"`
	Result  string `json:"result"`
	Updated int64  `json:"updated"`
	Black   string `json:"black"`
	White   string `json:"white"`
	Size    int    `json:"size"`
	Owner   string `json:"owner"`
}

// ListGames returns games a user owns or plays in, most recent first.
func (s *Store) ListGames(userID string, limit int) ([]GameSummary, error) {
	rows, err := s.db.Query(`SELECT meta FROM games ORDER BY updated DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GameSummary
	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err != nil {
			return nil, err
		}
		var m session.Meta
		if json.Unmarshal([]byte(blob), &m) != nil {
			continue
		}
		mine := m.OwnerID == userID || m.Players[0].UserID == userID || m.Players[1].UserID == userID
		if !mine {
			continue
		}
		out = append(out, GameSummary{
			ID: m.ID, Name: m.Name, Mode: string(m.Mode), Status: string(m.Status), Result: m.Result,
			Updated: m.Updated.Unix(), Black: m.Players[0].Name, White: m.Players[1].Name, Owner: m.OwnerID,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

// DeleteGame removes a saved game.
func (s *Store) DeleteGame(id, owner string) error {
	res, err := s.db.Exec(`DELETE FROM games WHERE id=? AND owner=?`, id, owner)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
