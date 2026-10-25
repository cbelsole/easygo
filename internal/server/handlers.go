package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math/rand"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"easygo/internal/board"
	"easygo/internal/clock"
	"easygo/internal/joseki"
	"easygo/internal/session"
	"easygo/internal/sgf"
	"easygo/internal/store"
)

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]bool{"ok": true}) })
	s.auth.Register(m)

	m.HandleFunc("GET /api/config", s.handleConfig)
	m.HandleFunc("GET /api/me", s.handleMe)
	m.HandleFunc("PUT /api/me", s.handleUpdateMe)

	m.HandleFunc("POST /api/games", s.handleCreate)
	m.HandleFunc("POST /api/games/import", s.handleImport)
	m.HandleFunc("GET /api/games", s.handleList)
	m.HandleFunc("GET /api/games/{id}", s.handleGet)
	m.HandleFunc("GET /api/games/{id}/events", s.handleEvents)
	m.HandleFunc("POST /api/games/{id}/join", s.handleJoin)
	m.HandleFunc("POST /api/games/{id}/move", s.handleMove)
	m.HandleFunc("POST /api/games/{id}/resign", s.handleResign)
	m.HandleFunc("POST /api/games/{id}/undo", s.handleUndo)
	m.HandleFunc("POST /api/games/{id}/goto", s.handleGoto)
	m.HandleFunc("POST /api/games/{id}/nodes/{node}/annotate", s.handleAnnotate)
	m.HandleFunc("DELETE /api/games/{id}/nodes/{node}", s.handleDeleteNode)
	m.HandleFunc("POST /api/games/{id}/nodes/{node}/promote", s.handlePromote)
	m.HandleFunc("POST /api/games/{id}/dead", s.handleDead)
	m.HandleFunc("POST /api/games/{id}/accept", s.handleAccept)
	m.HandleFunc("POST /api/games/{id}/assist", s.handleAssist)
	m.HandleFunc("GET /api/games/{id}/sgf", s.handleSGF)
	m.HandleFunc("DELETE /api/games/{id}", s.handleDelete)

	m.HandleFunc("GET /api/games/{id}/analysis", s.handleAnalysis)
	m.HandleFunc("GET /api/games/{id}/shapes", s.handleShapes)
	m.HandleFunc("GET /api/games/{id}/joseki", s.handleJosekiMatch)

	m.HandleFunc("GET /api/katago", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.katagoStatus(s.isAdmin(r))) })
	m.HandleFunc("PUT /api/katago", s.handleKataGoConfig)
	m.HandleFunc("GET /api/joseki", func(w http.ResponseWriter, r *http.Request) {
		s.jMu.RLock()
		defer s.jMu.RUnlock()
		writeJSON(w, 200, s.jInfo)
	})
	m.HandleFunc("POST /api/joseki/fetch", s.handleJosekiFetch)
	m.HandleFunc("POST /api/joseki/example", s.handleJosekiExample)

	if s.cfg.Web != nil {
		m.Handle("/", s.spa(s.cfg.Web))
	}
}

// spa serves the built frontend, falling back to index.html for client routes.
func (s *Server) spa(fsys fs.FS) http.Handler {
	files := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "."
		}
		if st, err := fs.Stat(fsys, name); err != nil || (st.IsDir() && name != ".") {
			if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/auth/") {
				http.NotFound(w, r)
				return
			}
			// A client-side route: hand the app its entry page.
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			files.ServeHTTP(w, r2)
			return
		}
		files.ServeHTTP(w, r)
	})
}

// ---- config & profile -------------------------------------------------

type presetInfo struct {
	Name  string      `json:"name"`
	Rules board.Rules `json:"rules"`
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	var presets []presetInfo
	for _, n := range board.PresetNames() {
		rr, _ := board.Preset(n)
		presets = append(presets, presetInfo{n, rr})
	}
	s.jMu.RLock()
	ji := s.jInfo
	s.jMu.RUnlock()
	writeJSON(w, 200, map[string]any{
		"presets":    presets,
		"sizes":      []int{9, 13, 19},
		"katago":     s.katagoStatus(s.isAdmin(r)),
		"joseki":     ji,
		"googleAuth": s.auth.GoogleEnabled(),
		"devLogin":   s.auth.DevEnabled(),
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, ok := s.auth.User(r)
	if !ok {
		writeJSON(w, 200, map[string]any{"user": nil})
		return
	}
	writeJSON(w, 200, map[string]any{"user": u})
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	u, ok := s.need(w, r)
	if !ok {
		return
	}
	var b struct{ Name, Rank, Location string }
	if !readJSON(w, r, &b) {
		return
	}
	name := strings.TrimSpace(b.Name)
	if name == "" || len(name) > 40 || len(b.Rank) > 12 || len(b.Location) > 60 {
		writeJSON(w, 400, apiError{"name must be 1-40 characters, rank up to 12, location up to 60"})
		return
	}
	if err := s.store.UpdateProfile(u.ID, name, strings.TrimSpace(b.Rank), strings.TrimSpace(b.Location)); err != nil {
		writeErr(w, err)
		return
	}
	u, _ = s.store.User(u.ID)
	writeJSON(w, 200, map[string]any{"user": u})
}

// ---- creating games ---------------------------------------------------

type createReq struct {
	Name     string          `json:"name"`
	Mode     session.Mode    `json:"mode"`
	Size     int             `json:"size"`
	Rules    json.RawMessage `json:"rules"` // preset name or a rules object
	Komi     *float64        `json:"komi"`
	Handicap int             `json:"handicap"`
	Opponent string          `json:"opponent"` // local | ai | open
	Color    string          `json:"color"`    // black | white | random
	AIVisits int             `json:"aiVisits"`
	Clock    clock.Config    `json:"clock"`
}

func parseRules(raw json.RawMessage, komi *float64) (board.Rules, error) {
	var r board.Rules
	if len(raw) == 0 || string(raw) == "null" {
		r, _ = board.Preset("japanese")
	} else if raw[0] == '"' {
		var name string
		json.Unmarshal(raw, &name)
		var err error
		if r, err = board.Preset(name); err != nil {
			return r, err
		}
	} else if err := json.Unmarshal(raw, &r); err != nil {
		return r, fmt.Errorf("bad rules object: %w", err)
	}
	if komi != nil {
		r.Komi = *komi
	}
	return r, r.Validate()
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	u, ok := s.need(w, r)
	if !ok {
		return
	}
	var req createReq
	if !readJSON(w, r, &req) {
		return
	}
	rules, err := parseRules(req.Rules, req.Komi)
	if err != nil {
		writeErr(w, err)
		return
	}
	if req.Mode == "" {
		req.Mode = session.ModeLive
	}
	me := session.Player{Name: u.Name, Rank: u.Rank, Location: u.Location, Kind: "human", UserID: u.ID}
	opts := session.Options{
		Name: req.Name, Mode: req.Mode, Size: req.Size, Rules: rules, Handicap: req.Handicap,
		Clock: req.Clock, OwnerID: u.ID,
	}
	if opts.Name == "" {
		opts.Name = fmt.Sprintf("%s's game", u.Name)
	}
	if req.Mode != session.ModeLive {
		opts.Black, opts.White, opts.Local = me, me, true
		opts.Black.UserID, opts.White.UserID = "", ""
		opts.White.Name = "White"
		opts.Black.Name = "Black"
		if req.Mode == session.ModeTeaching {
			opts.Black.Name, opts.White.Name = u.Name, "Student"
		}
	} else {
		other := session.Player{Kind: "open"}
		switch req.Opponent {
		case "local":
			opts.Local = true
			other = session.Player{Name: "Player 2", Kind: "human"}
			me.UserID = ""
		case "ai":
			if s.katago() == nil {
				writeJSON(w, 409, apiError{"KataGo is not running, so there is no computer opponent right now"})
				return
			}
			visits := req.AIVisits
			if visits <= 0 || visits > 5000 {
				visits = 200
			}
			other = session.Player{Name: "KataGo", Rank: fmt.Sprintf("%d visits", visits), Kind: "ai", UserID: "ai:katago", AIVisits: visits}
		case "open", "":
		default:
			writeJSON(w, 400, apiError{"opponent must be local, ai or open"})
			return
		}
		color := req.Color
		if color == "random" || color == "" {
			color = []string{"black", "white"}[rand.Intn(2)]
		}
		if color == "white" {
			opts.Black, opts.White = other, me
		} else {
			opts.Black, opts.White = me, other
		}
	}
	g, err := session.New(opts)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.mu.Lock()
	s.games[g.ID] = g
	s.mu.Unlock()
	s.touch(g)
	writeJSON(w, 201, g.View(u.ID))
}

type importReq struct {
	SGF  string       `json:"sgf"`
	Mode session.Mode `json:"mode"`
	Name string       `json:"name"`
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	u, ok := s.need(w, r)
	if !ok {
		return
	}
	var req importReq
	if !readJSON(w, r, &req) {
		return
	}
	t, err := sgf.Parse(req.SGF)
	if err != nil {
		writeJSON(w, 400, apiError{"that is not a readable SGF file: " + err.Error()})
		return
	}
	switch t.Size {
	case 9, 13, 19:
	default:
		writeJSON(w, 400, apiError{fmt.Sprintf("this app plays 9x9, 13x13 and 19x19; the file is %dx%d", t.Size, t.Size)})
		return
	}
	if req.Mode == "" || req.Mode == session.ModeLive {
		req.Mode = session.ModeTeaching
	}
	if req.Name != "" {
		t.Info.GameName = req.Name
	}
	g := session.FromTree(t, req.Mode, u.ID)
	s.mu.Lock()
	s.games[g.ID] = g
	s.mu.Unlock()
	s.touch(g)
	writeJSON(w, 201, g.View(u.ID))
}

// handleJosekiExample opens the downloaded joseki dictionary as a Joseki
// Example Set the user can browse.
func (s *Server) handleJosekiExample(w http.ResponseWriter, r *http.Request) {
	u, ok := s.need(w, r)
	if !ok {
		return
	}
	data, err := os.ReadFile(filepath.Join(s.cfg.DataDir, "cache", joseki.KogoFile))
	if err != nil {
		writeJSON(w, 409, apiError{"the joseki dictionary has not been downloaded yet"})
		return
	}
	t, err := sgf.Parse(string(data))
	if err != nil {
		writeErr(w, err)
		return
	}
	g := session.FromTree(t, session.ModeJoseki, u.ID)
	g.Name = "Kogo's Joseki Dictionary"
	s.mu.Lock()
	if old := s.exampleOf[u.ID]; old != "" {
		delete(s.games, old)
	}
	s.exampleOf[u.ID] = g.ID
	s.games[g.ID] = g
	s.mu.Unlock()
	// Not saved: a 1 MB tree per click would be wasteful, and the set is rebuilt on demand.
	writeJSON(w, 201, g.View(u.ID))
}

// ---- reading ----------------------------------------------------------

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	u, ok := s.need(w, r)
	if !ok {
		return
	}
	list, err := s.store.ListGames(u.ID, 100)
	if err != nil {
		writeErr(w, err)
		return
	}
	if list == nil {
		list = []store.GameSummary{}
	}
	// Open seats anyone could take.
	var open []store.GameSummary
	s.mu.RLock()
	for _, g := range s.games {
		g.Lock(func() {
			if g.Status == session.StatusWaiting && g.OwnerID != u.ID {
				open = append(open, store.GameSummary{
					ID: g.ID, Name: g.Name, Mode: string(g.Mode), Status: string(g.Status),
					Black: g.Players[board.Black].Name, White: g.Players[board.White].Name, Size: g.Tree.Size,
				})
			}
		})
	}
	s.mu.RUnlock()
	if open == nil {
		open = []store.GameSummary{}
	}
	writeJSON(w, 200, map[string]any{"mine": list, "open": open})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	g, ok := s.game(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, g.View(s.auth.UserID(r)))
}

func (s *Server) handleSGF(w http.ResponseWriter, r *http.Request) {
	g, ok := s.game(w, r)
	if !ok {
		return
	}
	var out string
	g.Lock(func() { out = sgf.Write(g.Tree) })
	w.Header().Set("Content-Type", "application/x-go-sgf; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.sgf"`, g.ID))
	w.Write([]byte(out))
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	u, ok := s.need(w, r)
	if !ok {
		return
	}
	g, ok := s.game(w, r)
	if !ok {
		return
	}
	if !g.IsOwner(u.ID) {
		writeErr(w, session.ErrNotYours)
		return
	}
	s.mu.Lock()
	delete(s.games, g.ID)
	if t := s.timers[g.ID]; t != nil {
		t.Stop()
		delete(s.timers, g.ID)
	}
	s.mu.Unlock()
	s.store.DeleteGame(g.ID, u.ID)
	w.WriteHeader(http.StatusNoContent)
}

// handleEvents streams the game's state as server-sent events.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	g, ok := s.game(w, r)
	if !ok {
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, 500, apiError{"streaming is not supported"})
		return
	}
	uid := s.auth.UserID(r)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")

	ch, cancel := g.Subscribe()
	defer cancel()
	send := func() bool {
		b, err := json.Marshal(g.View(uid))
		if err != nil {
			return false
		}
		_, err = fmt.Fprintf(w, "event: state\ndata: %s\n\n", b)
		fl.Flush()
		return err == nil
	}
	if !send() {
		return
	}
	// A periodic resend resyncs clocks even when nothing else changed.
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.ctx.Done():
			return
		case <-ch:
			if !send() {
				return
			}
		case <-tick.C:
			if !send() {
				return
			}
		}
	}
}

// ---- playing ----------------------------------------------------------

func (s *Server) mutate(w http.ResponseWriter, r *http.Request, fn func(g *session.Game, userID string) error) {
	u, ok := s.need(w, r)
	if !ok {
		return
	}
	g, ok := s.game(w, r)
	if !ok {
		return
	}
	if err := fn(g, u.ID); err != nil {
		writeErr(w, err)
		return
	}
	s.touch(g)
	writeJSON(w, 200, g.View(u.ID))
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	s.mutate(w, r, func(g *session.Game, uid string) error {
		u, err := s.store.User(uid)
		if err != nil {
			return err
		}
		var b struct{ Color string }
		json.NewDecoder(r.Body).Decode(&b)
		color := board.White
		if b.Color == "black" {
			color = board.Black
		}
		if b.Color == "" { // take whichever seat is open
			g.Lock(func() {
				if g.Players[board.Black].Kind == "open" {
					color = board.Black
				}
			})
		}
		return g.Join(uid, u.Name, u.Rank, u.Location, color)
	})
}

func (s *Server) handleMove(w http.ResponseWriter, r *http.Request) {
	var b struct {
		X, Y  int
		Pass  bool
		Color int // teaching and joseki games only
	}
	if !readJSON(w, r, &b) {
		return
	}
	s.mutate(w, r, func(g *session.Game, uid string) error {
		pt := board.Point{X: b.X, Y: b.Y}
		if b.Pass {
			pt = board.Pass
		}
		_, err := g.Play(uid, pt, board.Color(b.Color))
		return err
	})
}

func (s *Server) handleResign(w http.ResponseWriter, r *http.Request) {
	s.mutate(w, r, func(g *session.Game, uid string) error { return g.Resign(uid) })
}

func (s *Server) handleUndo(w http.ResponseWriter, r *http.Request) {
	s.mutate(w, r, func(g *session.Game, uid string) error {
		if err := g.Undo(uid); err != nil {
			return err
		}
		// Against the computer, take back the reply too so it is the human's turn again.
		var vsAI, humanToMove bool
		g.Lock(func() {
			vsAI = g.Mode == session.ModeLive &&
				(g.Players[board.Black].Kind == "ai" || g.Players[board.White].Kind == "ai")
			if pos, err := g.Tree.Position(g.Cursor); err == nil {
				humanToMove = g.Players[pos.ToPlay].Kind != "ai"
			}
		})
		if vsAI && !humanToMove {
			return g.Undo(uid)
		}
		return nil
	})
}

func (s *Server) handleGoto(w http.ResponseWriter, r *http.Request) {
	var b struct{ Node int }
	if !readJSON(w, r, &b) {
		return
	}
	s.mutate(w, r, func(g *session.Game, uid string) error { return g.Goto(uid, b.Node) })
}

func nodeID(r *http.Request) (int, error) {
	var n int
	if _, err := fmt.Sscanf(r.PathValue("node"), "%d", &n); err != nil {
		return 0, session.ErrUnknownNode
	}
	return n, nil
}

func (s *Server) handleAnnotate(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Comment  *string `json:"comment"`
		Name     *string `json:"name"`
		Judgment *string `json:"judgment"`
		Marks    *[]struct {
			X    int    `json:"x"`
			Y    int    `json:"y"`
			Kind string `json:"kind"`
			Text string `json:"text"`
		} `json:"marks"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	id, err := nodeID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	a := session.Annotation{Comment: b.Comment, Name: b.Name, Judgment: b.Judgment}
	if b.Marks != nil {
		ms := make([]markT, 0, len(*b.Marks))
		for _, m := range *b.Marks {
			switch m.Kind {
			case "label", "triangle", "square", "circle", "cross":
			default:
				writeJSON(w, 400, apiError{"unknown mark kind " + m.Kind})
				return
			}
			ms = append(ms, markT{Pt: board.Point{X: m.X, Y: m.Y}, Kind: m.Kind, Text: m.Text})
		}
		a.Marks = &ms
	}
	s.mutate(w, r, func(g *session.Game, uid string) error { return g.Annotate(uid, id, a) })
}

func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	id, err := nodeID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.mutate(w, r, func(g *session.Game, uid string) error { return g.DeleteNode(uid, id) })
}

func (s *Server) handlePromote(w http.ResponseWriter, r *http.Request) {
	id, err := nodeID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.mutate(w, r, func(g *session.Game, uid string) error { return g.Promote(uid, id) })
}

func (s *Server) handleDead(w http.ResponseWriter, r *http.Request) {
	var b struct {
		X, Y int
		Dead bool
	}
	if !readJSON(w, r, &b) {
		return
	}
	s.mutate(w, r, func(g *session.Game, uid string) error {
		return g.ToggleDead(uid, board.Point{X: b.X, Y: b.Y}, b.Dead)
	})
}

func (s *Server) handleAccept(w http.ResponseWriter, r *http.Request) {
	s.mutate(w, r, func(g *session.Game, uid string) error { return g.AcceptScore(uid) })
}

func (s *Server) handleAssist(w http.ResponseWriter, r *http.Request) {
	var b struct{ On bool }
	if !readJSON(w, r, &b) {
		return
	}
	s.mutate(w, r, func(g *session.Game, uid string) error { return g.SetAssist(uid, b.On) })
}

// ---- admin-ish --------------------------------------------------------

// admin returns the signed-in user if they may change server settings.
func (s *Server) admin(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	u, ok := s.need(w, r)
	if !ok {
		return u, false
	}
	if !s.auth.IsAdmin(u) {
		writeJSON(w, http.StatusForbidden, apiError{"only the server's administrator can change this"})
		return u, false
	}
	return u, true
}

func (s *Server) handleKataGoConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.admin(w, r); !ok {
		return
	}
	var c struct{ Binary, Model, Config string }
	if !readJSON(w, r, &c) {
		return
	}
	s.kgMu.Lock()
	s.cfg.KataGo.Binary, s.cfg.KataGo.Model, s.cfg.KataGo.Config = c.Binary, c.Model, c.Config
	s.kgErr = ""
	s.kgMu.Unlock()
	if err := s.saveSettings(); err != nil {
		writeJSON(w, 500, apiError{"could not save settings: " + err.Error()})
		return
	}
	s.StartKataGo()
	writeJSON(w, 202, s.katagoStatus(true))
}

func (s *Server) handleJosekiFetch(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.admin(w, r); !ok {
		return
	}
	if err := s.fetchJoseki(r.Context()); err != nil {
		writeJSON(w, 502, apiError{err.Error()})
		return
	}
	s.jMu.RLock()
	defer s.jMu.RUnlock()
	writeJSON(w, 200, s.jInfo)
}

// isAdmin reports whether the request is from a signed-in administrator.
func (s *Server) isAdmin(r *http.Request) bool {
	u, ok := s.auth.User(r)
	return ok && s.auth.IsAdmin(u)
}
