package katago

import (
	"context"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"easygo/internal/board"
	"easygo/internal/sgf"
)

// The integration tests need a real KataGo and are skipped without one.
// Point EASYGO_KATAGO_DIR at a folder holding bin/katago.exe, analysis.cfg
// and nets/*.bin.gz (scripts/setup-katago.ps1 creates that layout).
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
	if hm, _ := filepath.Glob(filepath.Join(dir, "human", "*.bin.gz")); len(hm) > 0 {
		cfg.HumanModel = hm[0]
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

// Positions from real opening play on 19x19, as GTP move lists.
var humanTestPositions = [][]string{
	{"Q16", "D4", "Q4", "D16"},
	{"Q16", "D4", "Q4", "D16", "R14", "C14"},
	{"R17", "Q16", "R16", "Q15", "P18", "N16"},
	{"Q16", "D4", "C16", "Q4", "R14", "K17", "R3", "O3"},
	{"D4", "Q16", "Q4", "D16", "K10"},
}

func startWithHuman(t *testing.T) (*Client, context.Context) {
	t.Helper()
	cfg := realConfig(t)
	if cfg.HumanModel == "" {
		t.Skip("no human-style model installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	t.Cleanup(cancel)
	c, err := Start(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c, ctx
}

func queryFor(moves []string) Query {
	r, _ := board.Preset("japanese")
	q := Query{Size: 19, Rules: r, Moves: [][2]string{}}
	for i, m := range moves {
		c := "B"
		if i%2 == 1 {
			c = "W"
		}
		q.Moves = append(q.Moves, [2]string{c, m})
	}
	return q
}

// Higher ranks should put more of their probability on the moves KataGo
// itself rates best, and lower ranks should spread out more.
func TestHumanPolicyGetsSharperWithRank(t *testing.T) {
	c, ctx := startWithHuman(t)
	mass := map[string]float64{}
	entropy := map[string]float64{}
	ranks := []string{"20k", "10k", "1d", "9d"}
	for _, moves := range humanTestPositions {
		base := queryFor(moves)
		base.MaxVisits = 200
		best, err := c.Analyze(ctx, base)
		if err != nil {
			t.Fatal(err)
		}
		top := map[int]bool{}
		for i, m := range best.MoveInfos {
			if i >= 3 {
				break
			}
			p, _ := board.ParseGTP(m.Move, 19)
			top[p.Y*19+p.X] = true
		}
		for _, rank := range ranks {
			q := queryFor(moves)
			q.HumanProfile, q.Policy, q.MaxVisits = "rank_"+rank, true, 20
			resp, err := c.Analyze(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			if len(resp.HumanPolicy) != 19*19+1 {
				t.Fatalf("humanPolicy length %d", len(resp.HumanPolicy))
			}
			for i, v := range resp.HumanPolicy {
				if v <= 0 {
					continue
				}
				if top[i] {
					mass[rank] += v
				}
				entropy[rank] -= v * math.Log(v)
			}
		}
	}
	n := float64(len(humanTestPositions))
	for _, r := range ranks {
		t.Logf("%-3s: %.2f of its probability on KataGo's top 3, entropy %.2f", r, mass[r]/n, entropy[r]/n)
	}
	if mass["9d"] <= mass["20k"] {
		t.Errorf("9d (%.2f) should agree with KataGo more than 20k (%.2f)", mass["9d"]/n, mass["20k"]/n)
	}
	if entropy["9d"] >= entropy["20k"] {
		t.Errorf("9d (%.2f) should be more decisive than 20k (%.2f)", entropy["9d"]/n, entropy["20k"]/n)
	}
}

func TestHumanMoveIsLegalAndRankDependent(t *testing.T) {
	c, ctx := startWithHuman(t)
	r, _ := board.Preset("japanese")
	q := queryFor(humanTestPositions[1])
	pos := board.NewPosition(19)
	for i, m := range humanTestPositions[1] {
		p, _ := board.ParseGTP(m, 19)
		col := board.Black
		if i%2 == 1 {
			col = board.White
		}
		var err error
		if pos, err = pos.Play(r, col, p); err != nil {
			t.Fatal(err)
		}
	}
	legal := func(p board.Point) bool { return pos.Legal(r, pos.ToPlay, p) }
	seen := map[board.Point]bool{}
	for i := 0; i < 12; i++ {
		p, err := c.HumanMove(ctx, q, "16k", legal, rand.New(rand.NewSource(int64(i))))
		if err != nil {
			t.Fatal(err)
		}
		if !p.IsPass() && !legal(p) {
			t.Fatalf("illegal move %v", p)
		}
		seen[p] = true
	}
	// A 16k plays varied moves, not the same one every time.
	if len(seen) < 2 {
		t.Errorf("12 samples gave %d distinct moves; a human-style player should vary", len(seen))
	}
	t.Logf("16k chose %d distinct moves in 12 tries", len(seen))
	if _, err := c.HumanMove(ctx, q, "nonsense", legal, nil); err == nil {
		t.Error("an invalid rank must be refused")
	}
}
