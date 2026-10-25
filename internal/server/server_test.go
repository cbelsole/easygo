package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"easygo/internal/auth"
	"easygo/internal/katago"
	"easygo/internal/session"
)

type testEnv struct {
	t   *testing.T
	srv *Server
	ts  *httptest.Server
}

func newEnv(t *testing.T, kg katago.Config) *testEnv {
	t.Helper()
	srv, err := New(Config{
		DataDir: t.TempDir(), KataGo: kg,
		Auth: auth.Config{Secret: auth.NewSecret(), BaseURL: "http://localhost", DevLogin: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Close() })
	return &testEnv{t, srv, ts}
}

// client is one signed-in browser.
type client struct {
	e *testEnv
	c *http.Client
}

func (e *testEnv) login(name string) *client {
	jar, _ := cookiejar.New(nil)
	cl := &client{e, &http.Client{Jar: jar}}
	if code, body := cl.do("POST", "/api/auth/dev-login", map[string]string{"name": name}, nil); code != 200 {
		e.t.Fatalf("login %s: %d %s", name, code, body)
	}
	return cl
}

func (c *client) do(method, path string, in, out any) (int, string) {
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.e.ts.URL+path, body)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.c.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 {
		if err := json.Unmarshal(raw, out); err != nil {
			c.e.t.Fatalf("decode %s: %v\n%s", path, err, raw)
		}
	}
	return resp.StatusCode, string(raw)
}

func (c *client) create(req map[string]any) session.GameView {
	var v session.GameView
	if code, body := c.do("POST", "/api/games", req, &v); code != 201 {
		c.e.t.Fatalf("create: %d %s", code, body)
	}
	return v
}

func (c *client) move(id string, x, y int) (int, session.GameView) {
	var v session.GameView
	code, _ := c.do("POST", "/api/games/"+id+"/move", map[string]int{"x": x, "y": y}, &v)
	return code, v
}

func TestRequiresSignIn(t *testing.T) {
	e := newEnv(t, katago.Config{})
	anon := &client{e, &http.Client{}}
	if code, _ := anon.do("POST", "/api/games", map[string]any{"size": 9}, nil); code != 401 {
		t.Errorf("anonymous create: %d", code)
	}
	var me struct{ User *struct{ Name string } }
	anon.do("GET", "/api/me", nil, &me)
	if me.User != nil {
		t.Error("anonymous /api/me should have no user")
	}
}

func TestLocalGameFlow(t *testing.T) {
	e := newEnv(t, katago.Config{})
	ann := e.login("Ann")
	v := ann.create(map[string]any{"size": 9, "rules": "japanese", "opponent": "local"})
	if v.Status != session.StatusPlaying || !v.You.CanMove || v.You.Seat != 3 {
		t.Fatalf("new local game: %+v", v.You)
	}
	if code, v := ann.move(v.ID, 4, 4); code != 200 || v.MoveNumber != 1 || v.Board[4*9+4] != 1 {
		t.Fatalf("first move: %d %+v", code, v.MoveNumber)
	}
	// Same point again is illegal (and reports why with a 4xx).
	if code, _ := ann.move(v.ID, 4, 4); code != 422 {
		t.Errorf("occupied point: %d", code)
	}
	var after session.GameView
	ann.do("POST", "/api/games/"+v.ID+"/undo", nil, &after)
	if after.MoveNumber != 0 {
		t.Errorf("undo: move %d", after.MoveNumber)
	}
}

func TestNetworkedGameAndSpectator(t *testing.T) {
	e := newEnv(t, katago.Config{})
	ann, bo, cy := e.login("Ann"), e.login("Bo"), e.login("Cy")
	v := ann.create(map[string]any{"size": 9, "opponent": "open", "color": "black"})
	if v.Status != session.StatusWaiting {
		t.Fatalf("status %s", v.Status)
	}
	// The open game shows up in Bo's list.
	var lists struct{ Open []struct{ ID string } }
	bo.do("GET", "/api/games", nil, &lists)
	if len(lists.Open) != 1 || lists.Open[0].ID != v.ID {
		t.Fatalf("open games: %+v", lists.Open)
	}
	if code, _ := ann.move(v.ID, 4, 4); code != 409 {
		t.Errorf("moving before the opponent joins: %d", code)
	}
	var joined session.GameView
	if code, body := bo.do("POST", "/api/games/"+v.ID+"/join", map[string]string{}, &joined); code != 200 {
		t.Fatalf("join: %d %s", code, body)
	}
	if joined.Status != session.StatusPlaying || joined.You.Seat != 2 {
		t.Fatalf("after join: %+v", joined.You)
	}
	if code, _ := bo.move(v.ID, 2, 2); code != 409 {
		t.Errorf("white moving first: %d", code)
	}
	if code, _ := cy.move(v.ID, 3, 3); code != 409 {
		t.Errorf("spectator moving: %d", code)
	}
	if code, _ := ann.move(v.ID, 4, 4); code != 200 {
		t.Fatalf("black move: %d", code)
	}
	// The spectator can watch but cannot ask the AI for help.
	var cv session.GameView
	cy.do("GET", "/api/games/"+v.ID, nil, &cv)
	if cv.MoveNumber != 1 || cv.You.CanMove || cv.You.Seat != 0 {
		t.Errorf("spectator view: %+v moves=%d", cv.You, cv.MoveNumber)
	}
	if code, _ := cy.do("GET", "/api/games/"+v.ID+"/analysis", nil, nil); code != 403 {
		t.Errorf("spectator analysis: %d", code)
	}
	// Players need to agree before the AI helps in a human-vs-human game.
	if code, _ := ann.do("GET", "/api/games/"+v.ID+"/analysis", nil, nil); code != 403 {
		t.Errorf("analysis without agreement: %d", code)
	}
}

func TestEventStreamPushesMoves(t *testing.T) {
	e := newEnv(t, katago.Config{})
	ann := e.login("Ann")
	v := ann.create(map[string]any{"size": 9, "opponent": "local"})

	req, _ := http.NewRequest("GET", e.ts.URL+"/api/games/"+v.ID+"/events", nil)
	resp, err := ann.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content type %q", ct)
	}
	events := make(chan session.GameView, 4)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 8<<20)
		for sc.Scan() {
			if line := sc.Text(); strings.HasPrefix(line, "data: ") {
				var gv session.GameView
				if json.Unmarshal([]byte(line[6:]), &gv) == nil {
					events <- gv
				}
			}
		}
	}()
	wait := func() session.GameView {
		select {
		case gv := <-events:
			return gv
		case <-time.After(3 * time.Second):
			t.Fatal("no event")
			return session.GameView{}
		}
	}
	if first := wait(); first.MoveNumber != 0 {
		t.Errorf("initial state has move %d", first.MoveNumber)
	}
	ann.move(v.ID, 3, 3)
	if next := wait(); next.MoveNumber != 1 {
		t.Errorf("pushed state has move %d", next.MoveNumber)
	}
}

func TestClockCanBeDisabledPerGame(t *testing.T) {
	e := newEnv(t, katago.Config{})
	ann := e.login("Ann")
	timed := ann.create(map[string]any{"size": 9, "opponent": "local",
		"clock": map[string]any{"enabled": true, "mainSeconds": 300, "periods": 3, "periodSeconds": 30}})
	untimed := ann.create(map[string]any{"size": 9, "opponent": "local",
		"clock": map[string]any{"enabled": false, "mainSeconds": 300}})
	if !timed.Clock[0].Enabled || timed.Clock[0].MainMs < 299_000 || timed.Clock[0].MainMs > 300_000 || timed.Clock[0].Periods != 3 {
		t.Errorf("timed clock %+v", timed.Clock[0])
	}
	if untimed.Clock[0].Enabled {
		t.Errorf("untimed game reports a running clock: %+v", untimed.Clock[0])
	}
}

func TestImportAndTeachingBranches(t *testing.T) {
	e := newEnv(t, katago.Config{})
	ann := e.login("Ann")
	var v session.GameView
	code, body := ann.do("POST", "/api/games/import", map[string]any{
		"sgf": "(;SZ[9]KM[6.5];B[ee](;W[ec];B[cc])(;W[ce]))", "mode": "teaching",
	}, &v)
	if code != 201 {
		t.Fatalf("import: %d %s", code, body)
	}
	if v.Mode != session.ModeTeaching || len(v.Nodes) != 5 {
		t.Fatalf("imported %s with %d nodes", v.Mode, len(v.Nodes))
	}
	// Navigate to the second variation and annotate it.
	var branch int
	for _, n := range v.Nodes {
		if n.Color == 2 && n.X == 2 && n.Y == 4 {
			branch = n.ID
		}
	}
	var g session.GameView
	if code, _ := ann.do("POST", "/api/games/"+v.ID+"/goto", map[string]int{"node": branch}, &g); code != 200 || g.Cursor != branch {
		t.Fatalf("goto: %d cursor %d want %d", code, g.Cursor, branch)
	}
	if code, _ := ann.do("POST", "/api/games/"+v.ID+"/nodes/"+itoa(branch)+"/annotate",
		map[string]any{"comment": "the other way", "judgment": "mistake",
			"marks": []map[string]any{{"x": 2, "y": 4, "kind": "label", "text": "A"}}}, &g); code != 200 {
		t.Fatalf("annotate: %d", code)
	}
	var found bool
	for _, n := range g.Nodes {
		if n.ID == branch && n.Comment == "the other way" && n.Judgment == "mistake" && len(n.Marks) == 1 {
			found = true
		}
	}
	if !found {
		t.Error("annotation not in view")
	}
	// A bad mark kind is refused.
	if code, _ := ann.do("POST", "/api/games/"+v.ID+"/nodes/"+itoa(branch)+"/annotate",
		map[string]any{"marks": []map[string]any{{"x": 0, "y": 0, "kind": "sparkle"}}}, nil); code != 400 {
		t.Errorf("bad mark: %d", code)
	}
	// Someone else cannot edit it.
	bo := e.login("Bo")
	if code, _ := bo.do("POST", "/api/games/"+v.ID+"/goto", map[string]int{"node": 1}, nil); code != 403 {
		t.Errorf("stranger editing: %d", code)
	}
	// SGF export contains the variation.
	_, sgfBody := ann.do("GET", "/api/games/"+v.ID+"/sgf", nil, nil)
	if !strings.Contains(sgfBody, "the other way") || !strings.Contains(sgfBody, "BM[1]") {
		t.Errorf("export: %s", sgfBody)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestImportRejectsOddBoardsAndGarbage(t *testing.T) {
	e := newEnv(t, katago.Config{})
	ann := e.login("Ann")
	for _, sgf := range []string{"(;SZ[15];B[aa])", "hello", "(;SZ[9:13])"} {
		if code, _ := ann.do("POST", "/api/games/import", map[string]any{"sgf": sgf}, nil); code != 400 {
			t.Errorf("%q: %d", sgf, code)
		}
	}
}

func TestShapesEndpoint(t *testing.T) {
	e := newEnv(t, katago.Config{})
	ann := e.login("Ann")
	// A black knight's move on 9x9.
	var v session.GameView
	ann.do("POST", "/api/games/import", map[string]any{"sgf": "(;SZ[9];B[cc];W[gg];B[de])", "mode": "teaching"}, &v)
	var g session.GameView
	ann.do("POST", "/api/games/"+v.ID+"/goto", map[string]int{"node": v.MainLine[len(v.MainLine)-1]}, &g)
	var out struct{ Findings []shapeView }
	if code, body := ann.do("GET", "/api/games/"+v.ID+"/shapes", nil, &out); code != 200 {
		t.Fatalf("shapes: %d %s", code, body)
	}
	found := false
	for _, f := range out.Findings {
		if f.Kind == "knight" && f.Color == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("knight's move not reported: %+v", out.Findings)
	}
}

func TestPersistenceAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{DataDir: dir, Auth: auth.Config{Secret: []byte("0123456789abcdef0123456789abcdef"), BaseURL: "http://localhost", DevLogin: true}}
	s1, _ := New(cfg)
	ts1 := httptest.NewServer(s1.Handler())
	e1 := &testEnv{t, s1, ts1}
	jar, _ := cookiejar.New(nil)
	cl := &client{e1, &http.Client{Jar: jar}}
	cl.do("POST", "/api/auth/dev-login", map[string]string{"name": "Ann"}, nil)
	v := cl.create(map[string]any{"size": 9, "opponent": "local"})
	cl.move(v.ID, 4, 4)
	cl.move(v.ID, 2, 2)
	ts1.Close()
	s1.Close()

	s2, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ts2 := httptest.NewServer(s2.Handler())
	defer func() { ts2.Close(); s2.Close() }()
	cl.e = &testEnv{t, s2, ts2} // same cookie secret, so the old session still works
	var got session.GameView
	if code, body := cl.do("GET", "/api/games/"+v.ID, nil, &got); code != 200 || got.MoveNumber != 2 {
		t.Fatalf("after restart: %d moves=%d %s", code, got.MoveNumber, body)
	}
	if code, _ := cl.move(v.ID, 6, 6); code != 200 {
		t.Errorf("playing on after restart: %d", code)
	}
}

// ---- with a real KataGo -------------------------------------------------

func realKataGo(t *testing.T) katago.Config {
	t.Helper()
	dir := os.Getenv("EASYGO_KATAGO_DIR")
	if dir == "" {
		dir = filepath.Join(os.Getenv("USERPROFILE"), "katago")
	}
	cfg := katago.Config{Binary: filepath.Join(dir, "bin", "katago.exe"), Config: filepath.Join(dir, "analysis.cfg")}
	if nets, _ := filepath.Glob(filepath.Join(dir, "nets", "*.bin.gz")); len(nets) > 0 {
		cfg.Model = nets[0]
	}
	if cfg.Valid() != nil {
		t.Skip("no KataGo available")
	}
	return cfg
}

func (e *testEnv) waitKataGo() {
	e.srv.StartKataGo()
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		if e.srv.katago() != nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	e.t.Fatalf("katago never became ready: %+v", e.srv.katagoStatus())
}

func TestAnalysisEndpointWithKataGo(t *testing.T) {
	e := newEnv(t, realKataGo(t))
	e.waitKataGo()
	ann := e.login("Ann")
	v := ann.create(map[string]any{"size": 9, "opponent": "local"})
	ann.move(v.ID, 4, 4)
	var res AnalysisResult
	if code, body := ann.do("GET", "/api/games/"+v.ID+"/analysis?visits=50", nil, &res); code != 200 {
		t.Fatalf("analysis: %d %s", code, body)
	}
	if !res.Available || len(res.Suggestions) == 0 || len(res.Ownership) != 81 || len(res.Groups) != 1 {
		t.Fatalf("analysis: %+v", res)
	}
	if res.ToPlay != 2 {
		t.Errorf("toPlay %d", res.ToPlay)
	}
}

func (c *client) waitView(id, what string, cond func(session.GameView) bool) session.GameView {
	c.e.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		var g session.GameView
		c.do("GET", "/api/games/"+id, nil, &g)
		if cond(g) {
			return g
		}
		time.Sleep(100 * time.Millisecond)
	}
	c.e.t.Fatalf("timed out waiting for %s", what)
	return session.GameView{}
}

func TestKataGoPlaysTheOpponent(t *testing.T) {
	e := newEnv(t, realKataGo(t))
	e.waitKataGo()
	ann := e.login("Ann")
	v := ann.create(map[string]any{"size": 9, "opponent": "ai", "color": "black", "aiVisits": 30, "rules": "chinese"})
	if v.Players[1].Kind != "ai" {
		t.Fatalf("white should be the AI: %+v", v.Players)
	}
	if code, _ := ann.move(v.ID, 4, 4); code != 200 {
		t.Fatalf("first move: %d", code)
	}
	g := ann.waitView(v.ID, "the AI reply", func(g session.GameView) bool { return g.MoveNumber >= 2 })
	whites := 0
	for _, c := range g.Board {
		if c == 2 {
			whites++
		}
	}
	if whites != 1 || g.ToPlay != 1 {
		t.Errorf("expected one white stone and black to move: whites=%d toPlay=%d", whites, g.ToPlay)
	}
	// Taking back a move against the computer undoes its reply too.
	var back session.GameView
	ann.do("POST", "/api/games/"+v.ID+"/undo", nil, &back)
	if back.MoveNumber != 0 {
		t.Errorf("undo against the AI left move %d", back.MoveNumber)
	}
}

// Reaches scoring with a stranded black stone and checks that KataGo marks
// it dead on its own, and that the final result reflects that.
func TestScoringJudgesDeadStonesWithKataGo(t *testing.T) {
	e := newEnv(t, realKataGo(t))
	e.waitKataGo()
	ann := e.login("Ann")
	v := ann.create(map[string]any{"size": 9, "opponent": "local", "rules": "chinese"})
	// Black wall on column 4, white wall on column 6, interleaved.
	for y := 0; y < 9; y++ {
		if code, _ := ann.move(v.ID, 4, y); code != 200 {
			t.Fatalf("black wall %d: %d", y, code)
		}
		if code, _ := ann.move(v.ID, 6, y); code != 200 {
			t.Fatalf("white wall %d: %d", y, code)
		}
	}
	ann.move(v.ID, 7, 4) // black strands a stone behind white's wall
	pass := func() { ann.do("POST", "/api/games/"+v.ID+"/move", map[string]any{"pass": true}, nil) }
	pass() // white
	pass() // black
	g := ann.waitView(v.ID, "KataGo's dead-stone verdict", func(g session.GameView) bool {
		return g.Status == session.StatusScoring && len(g.Dead) > 0
	})
	if len(g.Dead) != 1 || g.Dead[0].X != 7 || g.Dead[0].Y != 4 {
		t.Fatalf("dead stones: %+v", g.Dead)
	}
	// The player disagrees, then agrees again.
	var v2 session.GameView
	ann.do("POST", "/api/games/"+v.ID+"/dead", map[string]any{"x": 7, "y": 4, "dead": false}, &v2)
	if len(v2.Dead) != 0 {
		t.Errorf("override should revive the stone: %+v", v2.Dead)
	}
	ann.do("POST", "/api/games/"+v.ID+"/dead", map[string]any{"x": 7, "y": 4, "dead": true}, &v2)
	var done session.GameView
	if code, body := ann.do("POST", "/api/games/"+v.ID+"/accept", nil, &done); code != 200 {
		t.Fatalf("accept: %d %s", code, body)
	}
	if done.Status != session.StatusFinished || done.Result != "B+11" {
		t.Errorf("result %q, want B+11: black 36 territory + 9 stones = 45 against white 18 + 9 + 7 komi = 34 (B+29 would mean the dead stone was not removed)", done.Result)
	}
	t.Logf("result %s", done.Result)
}

func TestKataGoSettingsPersist(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{DataDir: dir, Auth: auth.Config{Secret: []byte("0123456789abcdef0123456789abcdef"), DevLogin: true}}
	s1, _ := New(cfg)
	ts := httptest.NewServer(s1.Handler())
	jar, _ := cookiejar.New(nil)
	cl := &client{&testEnv{t, s1, ts}, &http.Client{Jar: jar}}
	cl.do("POST", "/api/auth/dev-login", map[string]string{"name": "Ann"}, nil)
	// Nonexistent paths: the change is saved, KataGo reports why it cannot start.
	if code, _ := cl.do("PUT", "/api/katago", map[string]string{"binary": "C:/nope/katago.exe", "model": "C:/nope/m.bin.gz", "config": "C:/nope/a.cfg"}, nil); code != 202 {
		t.Fatalf("put katago: %d", code)
	}
	ts.Close()
	s1.Close()

	s2, _ := New(Config{DataDir: dir, Auth: cfg.Auth})
	defer s2.Close()
	if got := s2.katagoStatus().Config.Binary; got != "C:/nope/katago.exe" {
		t.Errorf("saved KataGo path not restored: %q", got)
	}
}
