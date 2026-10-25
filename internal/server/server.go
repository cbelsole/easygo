// Package server is the HTTP API. It owns the rules and the clock; browsers
// only display state and send intentions (play here, pass, resign).
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"easygo/internal/auth"
	"easygo/internal/board"
	"easygo/internal/joseki"
	"easygo/internal/katago"
	"easygo/internal/session"
	"easygo/internal/store"
)

// Config holds everything the server needs from the outside.
type Config struct {
	DataDir string
	// Web is the built frontend (Vite output). Nil serves the API only.
	Web    fs.FS
	Auth   auth.Config
	KataGo katago.Config
}

type Server struct {
	cfg   Config
	store *store.Store
	auth  *auth.Auth
	mux   *http.ServeMux

	mu     sync.RWMutex
	games  map[string]*session.Game
	timers map[string]*time.Timer
	busy   map[string]bool
	// exampleOf remembers each user's open joseki-dictionary game; the tree is
	// large and not saved, so only the latest one is kept in memory.
	exampleOf map[string]string

	kgMu     sync.RWMutex
	kg       *katago.Client
	kgErr    string
	kgStart  bool // a start is in progress
	analysis *cache[*AnalysisResult]
	shapes   *cache[[]shapeView]
	// gpu bounds concurrent analyses so one user cannot starve the rest.
	gpu chan struct{}

	jMu    sync.RWMutex
	joseki *joseki.DB
	jInfo  josekiInfo

	ctx    context.Context
	cancel context.CancelFunc
}

type josekiInfo struct {
	Loaded    bool   `json:"loaded"`
	Positions int    `json:"positions"`
	Source    string `json:"source"`
	Error     string `json:"error,omitempty"`
}

// New builds a server, opens its database and restores unfinished games.
func New(cfg Config) (*Server, error) {
	if cfg.DataDir == "" {
		cfg.DataDir = "data"
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, err
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "easygo.db"))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		cfg: cfg, store: st, games: map[string]*session.Game{}, timers: map[string]*time.Timer{}, exampleOf: map[string]string{},
		analysis: newCache[*AnalysisResult](), shapes: newCache[[]shapeView](), gpu: make(chan struct{}, 6), ctx: ctx, cancel: cancel,
	}
	s.auth = auth.New(cfg.Auth, st)
	s.loadSettings()

	active, err := st.LoadActive()
	if err != nil {
		return nil, err
	}
	for _, g := range active {
		s.games[g.ID] = g
		s.armTimer(g)
	}

	s.mux = http.NewServeMux()
	s.routes()
	return s, nil
}

// Handler is the root HTTP handler.
func (s *Server) Handler() http.Handler { return s.logging(s.mux) }

// Close stops background work and closes the database.
func (s *Server) Close() {
	s.cancel()
	s.kgMu.Lock()
	if s.kg != nil {
		s.kg.Close()
		s.kg = nil
	}
	s.kgMu.Unlock()
	s.mu.Lock()
	for _, t := range s.timers {
		t.Stop()
	}
	s.mu.Unlock()
	s.store.Close()
}

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if r.URL.Path != "/api/health" && len(r.URL.Path) > 4 && r.URL.Path[:4] == "/api" {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}

// ---- helpers ---------------------------------------------------------

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	writeJSON(w, statusFor(err), apiError{err.Error()})
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, session.ErrNotYours):
		return http.StatusForbidden
	case errors.Is(err, session.ErrNotYourTurn), errors.Is(err, session.ErrNotPlaying),
		errors.Is(err, session.ErrNotAtTip), errors.Is(err, session.ErrBadMode), errors.Is(err, session.ErrSeatTaken):
		return http.StatusConflict
	case errors.Is(err, errBusy):
		return http.StatusTooManyRequests
	case errors.Is(err, session.ErrUnknownNode), errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, board.ErrOccupied), errors.Is(err, board.ErrSuicide), errors.Is(err, board.ErrKo),
		errors.Is(err, board.ErrOffBoard), errors.Is(err, board.ErrWrongTurn):
		return http.StatusUnprocessableEntity
	}
	return http.StatusBadRequest
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{"invalid request body"})
		return false
	}
	return true
}

// need returns the signed-in user, or writes 401.
func (s *Server) need(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	u, ok := s.auth.User(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, apiError{"sign in first"})
		return u, false
	}
	return u, true
}

func (s *Server) game(w http.ResponseWriter, r *http.Request) (*session.Game, bool) {
	id := r.PathValue("id")
	s.mu.RLock()
	g := s.games[id]
	s.mu.RUnlock()
	if g != nil {
		return g, true
	}
	// A finished game is not kept in memory; load it on demand.
	g, err := s.store.LoadGame(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, apiError{"no such game"})
		return nil, false
	}
	s.mu.Lock()
	s.games[id] = g
	s.mu.Unlock()
	return g, true
}

// ---- after every change ---------------------------------------------

// touch is called after any mutation: it saves the game, schedules the
// clock, and kicks off KataGo work (opponent move, scoring judgment).
func (s *Server) touch(g *session.Game) {
	if err := s.store.SaveGame(g); err != nil {
		log.Printf("save game %s: %v", g.ID, err)
	}
	s.armTimer(g)
	go s.followUp(g)
}

// armTimer schedules the flag for the side whose clock is running.
func (s *Server) armTimer(g *session.Game) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.timers[g.ID]; t != nil {
		t.Stop()
		delete(s.timers, g.ID)
	}
	var due time.Duration
	ok := false
	g.Lock(func() {
		if g.Status != session.StatusPlaying {
			return
		}
		if d, has := g.Clock.Deadline(); has {
			due, ok = time.Until(d)+20*time.Millisecond, true
		}
	})
	if !ok {
		return
	}
	if due < 0 {
		due = 0
	}
	s.timers[g.ID] = time.AfterFunc(due, func() {
		if g.CheckTimeout() {
			s.touch(g)
		}
	})
}

// ---- KataGo lifecycle ------------------------------------------------

// StartKataGo launches KataGo in the background. It is safe to call again
// after the paths change.
func (s *Server) StartKataGo() {
	s.kgMu.Lock()
	if s.kgStart {
		s.kgMu.Unlock()
		return
	}
	s.kgStart = true
	old := s.kg
	s.kg = nil
	cfg := s.cfg.KataGo
	s.kgMu.Unlock()
	if old != nil {
		old.Close()
	}
	go func() {
		defer func() { s.kgMu.Lock(); s.kgStart = false; s.kgMu.Unlock() }()
		if err := cfg.Valid(); err != nil {
			s.kgMu.Lock()
			s.kgErr = err.Error()
			s.kgMu.Unlock()
			return
		}
		c, err := katago.Start(s.ctx, cfg)
		s.kgMu.Lock()
		defer s.kgMu.Unlock()
		if err != nil {
			s.kgErr = err.Error()
			log.Printf("katago: %v", err)
			return
		}
		s.kg, s.kgErr = c, ""
		log.Printf("katago %s ready", c.Version)
	}()
}

// katago returns the running client, or nil.
func (s *Server) katago() *katago.Client {
	s.kgMu.RLock()
	defer s.kgMu.RUnlock()
	if s.kg != nil && s.kg.Running() {
		return s.kg
	}
	return nil
}

type katagoStatus struct {
	Available bool          `json:"available"`
	Starting  bool          `json:"starting"`
	Version   string        `json:"version,omitempty"`
	Error     string        `json:"error,omitempty"`
	Config    katago.Config `json:"config"`
}

func (s *Server) katagoStatus(withPaths bool) katagoStatus {
	s.kgMu.RLock()
	defer s.kgMu.RUnlock()
	st := katagoStatus{Starting: s.kgStart, Error: s.kgErr}
	if withPaths {
		st.Config = s.cfg.KataGo
	}
	if s.kg != nil && s.kg.Running() {
		st.Available, st.Version = true, s.kg.Version
	}
	return st
}

// ---- joseki ----------------------------------------------------------

// LoadJoseki builds the pattern database from the cached dictionary, if any.
func (s *Server) LoadJoseki() {
	path := filepath.Join(s.cfg.DataDir, "cache", joseki.KogoFile)
	s.jMu.Lock()
	defer s.jMu.Unlock()
	if _, err := os.Stat(path); err != nil {
		s.jInfo = josekiInfo{Error: "dictionary not downloaded yet"}
		return
	}
	db, err := joseki.LoadFile(path, "kogo")
	if err != nil {
		s.jInfo = josekiInfo{Error: err.Error()}
		return
	}
	s.joseki = db
	s.jInfo = josekiInfo{Loaded: true, Positions: db.Size(), Source: "Kogo's Joseki Dictionary"}
	log.Printf("joseki: %d positions", db.Size())
}

func (s *Server) josekiDB() *joseki.DB {
	s.jMu.RLock()
	defer s.jMu.RUnlock()
	return s.joseki
}

func (s *Server) fetchJoseki(ctx context.Context) error {
	if _, err := joseki.FetchKogo(ctx, filepath.Join(s.cfg.DataDir, "cache")); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	s.LoadJoseki()
	return nil
}

// ---- persisted settings ----------------------------------------------

type settings struct {
	KataGo *katago.Config `json:"katago,omitempty"`
}

func (s *Server) settingsPath() string { return filepath.Join(s.cfg.DataDir, "settings.json") }

// loadSettings applies saved in-app settings over the command-line defaults.
func (s *Server) loadSettings() {
	b, err := os.ReadFile(s.settingsPath())
	if err != nil {
		return
	}
	var st settings
	if json.Unmarshal(b, &st) == nil && st.KataGo != nil {
		s.cfg.KataGo = *st.KataGo
	}
}

func (s *Server) saveSettings() error {
	s.kgMu.RLock()
	kc := s.cfg.KataGo
	s.kgMu.RUnlock()
	b, err := json.MarshalIndent(settings{KataGo: &kc}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.settingsPath(), b, 0o644)
}

var errBusy = errors.New("the analysis engine is busy; try again in a moment")
