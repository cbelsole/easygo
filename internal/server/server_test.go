package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"easygo/internal/auth"
	"easygo/internal/katago"
	"easygo/internal/session"
	"easygo/internal/store"
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
	if hm, _ := filepath.Glob(filepath.Join(dir, "human", "*.bin.gz")); len(hm) > 0 {
		cfg.HumanModel = hm[0]
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
	e.t.Fatalf("katago never became ready: %+v", e.srv.katagoStatus(true))
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
	if got := s2.katagoStatus(true).Config.Binary; got != "C:/nope/katago.exe" {
		t.Errorf("saved KataGo path not restored: %q", got)
	}
}

// Changing which program the server launches is remote code execution on a
// public server, so it must be limited to administrators.
func TestOnlyAdminsChangeServerSettings(t *testing.T) {
	srv, err := New(Config{
		DataDir: t.TempDir(),
		Auth:    auth.Config{Secret: auth.NewSecret(), BaseURL: "http://localhost", GoogleID: "c", GoogleSecret: "s", Admins: []string{"boss@example.com"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer func() { ts.Close(); srv.Close() }()
	e := &testEnv{t, srv, ts}

	as := func(id, email string) *client {
		srv.store.UpsertUser(store.User{ID: id, Name: id, Email: email, Provider: "google"})
		jar, _ := cookiejar.New(nil)
		u, _ := url.Parse(ts.URL)
		jar.SetCookies(u, []*http.Cookie{srv.auth.SessionCookie(id)})
		return &client{e, &http.Client{Jar: jar}}
	}
	body := map[string]string{"binary": "C:/evil.exe", "model": "m", "config": "c"}

	user := as("google:1", "someone@example.com")
	if code, _ := user.do("PUT", "/api/katago", body, nil); code != 403 {
		t.Errorf("non-admin PUT /api/katago: %d", code)
	}
	if code, _ := user.do("POST", "/api/joseki/fetch", nil, nil); code != 403 {
		t.Errorf("non-admin joseki fetch: %d", code)
	}
	anon := &client{e, &http.Client{}}
	if code, _ := anon.do("PUT", "/api/katago", body, nil); code != 401 {
		t.Errorf("anonymous PUT: %d", code)
	}
	// File paths are not shown to ordinary users.
	var cfg struct {
		Katago struct{ Config struct{ Binary string } }
	}
	user.do("GET", "/api/config", nil, &cfg)
	if cfg.Katago.Config.Binary != "" {
		t.Errorf("paths leaked to a non-admin: %q", cfg.Katago.Config.Binary)
	}
	boss := as("google:2", "boss@example.com")
	if code, _ := boss.do("PUT", "/api/katago", body, nil); code != 202 {
		t.Errorf("admin PUT: %d", code)
	}
	boss.do("GET", "/api/config", nil, &cfg)
	if cfg.Katago.Config.Binary != "C:/evil.exe" {
		t.Errorf("admin should see the paths, got %q", cfg.Katago.Config.Binary)
	}
}

func TestJosekiExampleOpensAndReplacesItself(t *testing.T) {
	e := newEnv(t, katago.Config{})
	ann := e.login("Ann")
	// Put a tiny dictionary where the server looks for it.
	cache := filepath.Join(e.srv.cfg.DataDir, "cache")
	os.MkdirAll(cache, 0o755)
	os.WriteFile(filepath.Join(cache, "kogo-joseki-dictionary.sgf"),
		[]byte("(;SZ[19](;B[pd];W[qf])(;B[qd];W[pf]))"), 0o644)

	var first, second session.GameView
	if code, body := ann.do("POST", "/api/joseki/example", nil, &first); code != 201 {
		t.Fatalf("open: %d %s", code, body)
	}
	if first.Mode != session.ModeJoseki || len(first.Nodes) != 5 {
		t.Fatalf("example: mode %s, %d nodes", first.Mode, len(first.Nodes))
	}
	ann.do("POST", "/api/joseki/example", nil, &second)
	if code, _ := ann.do("GET", "/api/games/"+first.ID, nil, nil); code != 404 {
		t.Errorf("the previous example game should be dropped: %d", code)
	}
	if code, _ := ann.do("GET", "/api/games/"+second.ID, nil, nil); code != 200 {
		t.Errorf("the latest example game should be kept: %d", code)
	}
}

func TestServesFrontendWithClientRoutes(t *testing.T) {
	web := fstest.MapFS{
		"index.html":         {Data: []byte("<html>APP</html>")},
		"assets/app-1.js":    {Data: []byte("console.log(1)")},
		"assets/empty/.keep": {Data: []byte("")},
	}
	srv, err := New(Config{DataDir: t.TempDir(), Web: web, Auth: auth.Config{Secret: auth.NewSecret()}})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer func() { ts.Close(); srv.Close() }()

	get := func(path string) (int, string) {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	for _, p := range []string{"/", "/game/abc123", "/lobby", "/assets/empty"} {
		if code, body := get(p); code != 200 || !strings.Contains(body, "APP") {
			t.Errorf("%s -> %d %q (client routes get the app)", p, code, body)
		}
	}
	if code, body := get("/assets/app-1.js"); code != 200 || !strings.Contains(body, "console.log") {
		t.Errorf("asset -> %d %q", code, body)
	}
	if code, body := get("/api/nope"); code != 404 || strings.Contains(body, "APP") {
		t.Errorf("unknown API path must 404, not serve the app: %d %q", code, body)
	}
	if code, _ := get("/api/health"); code != 200 {
		t.Errorf("health: %d", code)
	}
}

// Computer help of any kind is off between two networked humans until both agree.
func TestShapesAndJosekiFollowTheFairPlayRule(t *testing.T) {
	e := newEnv(t, katago.Config{})
	ann, bo := e.login("Ann"), e.login("Bo")
	v := ann.create(map[string]any{"size": 19, "opponent": "open", "color": "black"})
	bo.do("POST", "/api/games/"+v.ID+"/join", map[string]string{}, nil)
	ann.move(v.ID, 3, 3)
	for _, ep := range []string{"shapes", "joseki", "analysis"} {
		if code, _ := ann.do("GET", "/api/games/"+v.ID+"/"+ep, nil, nil); code != 403 {
			t.Errorf("%s before agreement: %d", ep, code)
		}
	}
	ann.do("POST", "/api/games/"+v.ID+"/assist", map[string]bool{"on": true}, nil)
	bo.do("POST", "/api/games/"+v.ID+"/assist", map[string]bool{"on": true}, nil)
	for _, ep := range []string{"shapes", "joseki"} {
		if code, _ := ann.do("GET", "/api/games/"+v.ID+"/"+ep, nil, nil); code != 200 {
			t.Errorf("%s after both agreed: %d", ep, code)
		}
	}
	// A spectator still gets nothing while the game is in progress.
	cy := e.login("Cy")
	if code, _ := cy.do("GET", "/api/games/"+v.ID+"/shapes", nil, nil); code != 403 {
		t.Errorf("spectator shapes: %d", code)
	}
}

func TestViewAtNodeAndListSize(t *testing.T) {
	e := newEnv(t, katago.Config{})
	ann := e.login("Ann")
	v := ann.create(map[string]any{"size": 13, "opponent": "local", "handicap": 2})
	// With a handicap, white moves first.
	_, v1 := ann.move(v.ID, 6, 6)
	var root session.GameView
	ann.do("GET", "/api/games/"+v.ID+"?node="+itoa(v.Cursor), nil, &root)
	stones := 0
	for _, c := range root.Board {
		if c == 1 {
			stones++
		}
	}
	if stones != 2 || root.MoveNumber != 0 || root.You.CanMove {
		t.Errorf("root view: %d handicap stones, move %d, canMove %v", stones, root.MoveNumber, root.You.CanMove)
	}
	if v1.MoveNumber != 1 {
		t.Errorf("latest view move %d", v1.MoveNumber)
	}
	var lists struct{ Mine []struct{ Size int } }
	ann.do("GET", "/api/games", nil, &lists)
	if len(lists.Mine) != 1 || lists.Mine[0].Size != 13 {
		t.Errorf("list size: %+v", lists.Mine)
	}
}

func TestRankedOpponentNeedsAValidRankAndTheHumanModel(t *testing.T) {
	cfg := realKataGo(t)
	noHuman := cfg
	noHuman.HumanModel = ""
	e := newEnv(t, noHuman)
	e.waitKataGo()
	ann := e.login("Ann")
	for _, bad := range []string{"21k", "10d", "k16", "16", "rank_16k", "16k; rm -rf"} {
		if code, _ := ann.do("POST", "/api/games", map[string]any{"size": 9, "opponent": "ai", "aiRank": bad}, nil); code != 400 {
			t.Errorf("rank %q: %d, want 400", bad, code)
		}
	}
	// A valid rank, but KataGo was started without the human model.
	code, body := ann.do("POST", "/api/games", map[string]any{"size": 9, "opponent": "ai", "aiRank": "16k"}, nil)
	if code != 409 || !strings.Contains(body, "human-style model") {
		t.Errorf("without the model: %d %s", code, body)
	}
	var cfgOut struct{ Katago struct{ HumanAvailable bool } }
	ann.do("GET", "/api/config", nil, &cfgOut)
	if cfgOut.Katago.HumanAvailable {
		t.Error("config should report the human model as unavailable")
	}
}

func TestPlayAgainstA16kKataGo(t *testing.T) {
	cfg := realKataGo(t)
	if cfg.HumanModel == "" {
		t.Skip("no human-style model installed")
	}
	e := newEnv(t, cfg)
	e.waitKataGo()
	ann := e.login("Ann")
	var out struct {
		Ranks  []string
		Katago struct{ HumanAvailable bool }
	}
	ann.do("GET", "/api/config", nil, &out)
	if !out.Katago.HumanAvailable || len(out.Ranks) != 29 || out.Ranks[0] != "20k" || out.Ranks[28] != "9d" {
		t.Fatalf("config: %+v", out)
	}

	v := ann.create(map[string]any{"size": 19, "opponent": "ai", "aiRank": "16k", "color": "black"})
	if v.Players[1].Kind != "ai" || v.Players[1].Rank != "16k" {
		t.Fatalf("the opponent's card should read 16k: %+v", v.Players[1])
	}
	// Black plays star points; the 16k may have taken one itself, so always
	// pick a point that is still empty.
	stars := [][2]int{{15, 3}, {3, 15}, {15, 15}, {3, 3}}
	cur := v
	for i := 0; i < 2; i++ {
		want := 2 * (i + 1)
		var mv [2]int
		for _, s := range stars {
			if cur.Board[s[1]*19+s[0]] == 0 {
				mv = s
				break
			}
		}
		if code, _ := ann.move(v.ID, mv[0], mv[1]); code != 200 {
			t.Fatalf("move %d at %v: %d", i, mv, code)
		}
		cur = ann.waitView(v.ID, "the 16k reply", func(g session.GameView) bool { return g.MoveNumber >= want })
	}
	// The rank survives a save and reload.
	g, err := e.srv.store.LoadGame(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	var rank string
	g.Lock(func() { rank = g.Players[2].AIRank })
	if rank != "16k" {
		t.Errorf("stored rank %q", rank)
	}
}
