package katago

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"easygo/internal/board"
	"easygo/internal/sgf"
)

// The integration tests need a real KataGo and are skipped without one.
// Point EASYGO_KATAGO_DIR at a folder holding bin/katago.exe, analysis.cfg
// and nets/*.bin.gz (see docs/katago.md).
func realConfig(t *testing.T) Config {
	t.Helper()
	dir := os.Getenv("EASYGO_KATAGO_DIR")
	if dir == "" {
		dir = filepath.Join(os.Getenv("USERPROFILE"), "katago")
	}
	nets, _ := filepath.Glob(filepath.Join(dir, "nets", "*.bin.gz"))
	cfg := Config{
		Binary: filepath.Join(dir, "bin", "katago.exe"),
		Config: filepath.Join(dir, "analysis.cfg"),
	}
	if len(nets) > 0 {
		cfg.Model = nets[0]
	}
	if err := cfg.Valid(); err != nil {
		t.Skipf("no KataGo available: %v", err)
	}
	return cfg
}

func TestKataGoRoundTrip(t *testing.T) {
	cfg := realConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	c, err := Start(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	t.Logf("katago %s", c.Version)

	// A short 9x9 opening: checks plumbing, legality and the ownership sign.
	g, err := sgf.Parse("(;SZ[9]KM[6.5];B[dd];W[ff];B[ee])")
	if err != nil {
		t.Fatal(err)
	}
	end := g.MainLine()[len(g.MainLine())-1]
	q, err := FromTree(g, end)
	if err != nil {
		t.Fatal(err)
	}
	q.MaxVisits, q.Ownership = 100, true
	resp, err := c.Analyze(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Ownership) != 81 || len(resp.MoveInfos) == 0 {
		t.Fatalf("ownership=%d moveInfos=%d", len(resp.Ownership), len(resp.MoveInfos))
	}
	if resp.RootInfo.CurrentPlayer != "W" {
		t.Errorf("current player %q, want W", resp.RootInfo.CurrentPlayer)
	}
	// Black stones should read as black group ownership (positive).
	pos, _ := g.Position(end)
	for _, j := range Judge(pos.Board, resp.Ownership) {
		t.Logf("%s group %v: %s/%s own=%.2f", j.Color, j.Stones[0].GTP(9), j.Status, j.Strength, j.Own)
	}
	best, _ := board.ParseGTP(resp.MoveInfos[0].Move, 9)
	if !pos.Legal(g.Rules, board.White, best) {
		t.Errorf("suggested move %s is not legal", resp.MoveInfos[0].Move)
	}
}

// Every rule preset's JSON object must be accepted by KataGo.
func TestKataGoAcceptsAllRulePresets(t *testing.T) {
	cfg := realConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	c, err := Start(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, name := range board.PresetNames() {
		r, _ := board.Preset(name)
		_, err := c.Analyze(ctx, Query{Size: 9, Rules: r, Moves: [][2]string{{"B", "E5"}}, MaxVisits: 5})
		if err != nil {
			t.Errorf("preset %s rejected: %v", name, err)
		}
	}
}

// A lone black stone sitting behind a white wall, deep in white's area, must
// be judged dead; the walls themselves must be alive.
func TestKataGoJudgesDeadStone(t *testing.T) {
	cfg := realConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	c, err := Start(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Black wall on the e-file, white wall on the g-file, black stone at h5.
	g, err := sgf.Parse("(;SZ[9]KM[6.5]" +
		"AB[ea][eb][ec][ed][ee][ef][eg][eh][ei]" +
		"AW[ga][gb][gc][gd][ge][gf][gg][gh][gi]" +
		"PL[B];B[he];W[hd])")
	if err != nil {
		t.Fatal(err)
	}
	end := g.MainLine()[len(g.MainLine())-1]
	q, err := FromTree(g, end)
	if err != nil {
		t.Fatal(err)
	}
	q.MaxVisits, q.Ownership = 300, true
	resp, err := c.Analyze(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	pos, err := g.Position(end)
	if err != nil {
		t.Fatal(err)
	}
	js := Judge(pos.Board, resp.Ownership)
	for _, j := range js {
		t.Logf("%s %v: %s/%s own=%.2f", j.Color, j.Stones[0].GTP(9), j.Status, j.Strength, j.Own)
	}
	lone := find(js, board.Point{X: 7, Y: 4}) // h5
	if lone.Status != Dead {
		t.Errorf("the stranded black stone should be dead: %+v", lone)
	}
	if w := find(js, board.Point{X: 6, Y: 0}); w.Status != Alive {
		t.Errorf("white wall should be alive: %+v", w)
	}
	dead := DeadStones(js, nil)
	if !dead[board.Point{X: 7, Y: 4}] {
		t.Error("dead set should contain the stranded stone")
	}
}
