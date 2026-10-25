package katago

import (
	"math"
	"math/rand"
	"testing"

	"easygo/internal/board"
)

func TestRanks(t *testing.T) {
	rs := Ranks()
	if len(rs) != 29 || rs[0] != "20k" || rs[19] != "1k" || rs[20] != "1d" || rs[28] != "9d" {
		t.Fatalf("ranks: %v", rs)
	}
	for _, r := range rs {
		if !ValidRank(r) {
			t.Errorf("%s should be valid", r)
		}
	}
	for _, bad := range []string{"", "21k", "0k", "10d", "0d", "16", "k16", "16K", "16kyu", "-3k", "rank_16k", " 16k"} {
		if ValidRank(bad) {
			t.Errorf("%q must be rejected (it ends up in a query sent to KataGo)", bad)
		}
	}
	if !IsDan("3d") || IsDan("3k") || IsDan("") {
		t.Error("IsDan")
	}
}

// policy builds a size*size+1 human policy with the given weights.
func policy(size int, w map[board.Point]float64) []float64 {
	p := make([]float64, size*size+1)
	for i := range p {
		p[i] = -1 // illegal until set
	}
	for pt, v := range w {
		p[pt.Y*size+pt.X] = v
	}
	return p
}

func pt(x, y int) board.Point { return board.Point{X: x, Y: y} }

func TestSamplingFollowsThePolicy(t *testing.T) {
	resp := &Response{
		MoveInfos:   []MoveInfo{{Move: "D4"}},
		HumanPolicy: policy(9, map[board.Point]float64{pt(2, 2): 0.75, pt(6, 6): 0.25}),
	}
	rng := rand.New(rand.NewSource(1))
	counts := map[board.Point]int{}
	const n = 4000
	for i := 0; i < n; i++ {
		counts[PickHumanMove(resp, 9, false, nil, rng)]++
	}
	if len(counts) != 2 {
		t.Fatalf("only the two policy moves are possible: %v", counts)
	}
	if got := float64(counts[pt(2, 2)]) / n; math.Abs(got-0.75) > 0.03 {
		t.Errorf("frequency %.3f, want about 0.75", got)
	}
}

func TestPassWhenEngineSaysPass(t *testing.T) {
	resp := &Response{
		MoveInfos:   []MoveInfo{{Move: "pass"}, {Move: "D4"}},
		HumanPolicy: policy(9, map[board.Point]float64{pt(2, 2): 1}),
	}
	if got := PickHumanMove(resp, 9, false, nil, nil); !got.IsPass() {
		t.Errorf("should pass, got %v", got)
	}
}

func TestIllegalMovesNeverChosen(t *testing.T) {
	resp := &Response{
		MoveInfos:   []MoveInfo{{Move: "D4"}},
		HumanPolicy: policy(9, map[board.Point]float64{pt(2, 2): 0.9, pt(6, 6): 0.1}),
	}
	// The game's own rules forbid (2,2), e.g. a ko KataGo does not know about.
	legal := func(p board.Point) bool { return p != pt(2, 2) }
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 200; i++ {
		if got := PickHumanMove(resp, 9, false, legal, rng); got != pt(6, 6) {
			t.Fatalf("chose %v", got)
		}
	}
}

func TestFallsBackToEngineMoveWhenPolicyIsEmpty(t *testing.T) {
	resp := &Response{
		MoveInfos:   []MoveInfo{{Move: "E5"}},
		HumanPolicy: policy(9, nil),
	}
	if got := PickHumanMove(resp, 9, false, nil, nil); got != pt(4, 4) {
		t.Errorf("got %v, want E5", got)
	}
	if got := PickHumanMove(&Response{HumanPolicy: policy(9, nil)}, 9, false, nil, nil); !got.IsPass() {
		t.Errorf("with nothing to play, pass: %v", got)
	}
}

// For dan ranks, a move the search says loses badly is suppressed even when
// the human model likes it.
func TestWeightingSuppressesBlunders(t *testing.T) {
	resp := &Response{
		MoveInfos: []MoveInfo{
			{Move: "C3", HumanPrior: 0.5, Utility: -3.0}, // popular but terrible
			{Move: "G7", HumanPrior: 0.5, Utility: 0.0},  // equally popular, good
		},
		HumanPolicy: policy(9, map[board.Point]float64{pt(2, 6): 0.5, pt(6, 2): 0.5}),
	}
	rng := rand.New(rand.NewSource(3))
	good := 0
	const n = 2000
	for i := 0; i < n; i++ {
		if PickHumanMove(resp, 9, true, nil, rng) == pt(6, 2) {
			good++
		}
	}
	if frac := float64(good) / n; frac < 0.99 {
		t.Errorf("good move chosen %.3f of the time; exp(-6) weighting should make it nearly always", frac)
	}
	// Without weighting (kyu), the two are equally likely.
	good = 0
	for i := 0; i < n; i++ {
		if PickHumanMove(resp, 9, false, nil, rng) == pt(6, 2) {
			good++
		}
	}
	if frac := float64(good) / n; frac < 0.45 || frac > 0.55 {
		t.Errorf("unweighted frequency %.3f, want about 0.5", frac)
	}
}

func TestQueryCarriesTheProfile(t *testing.T) {
	r, _ := board.Preset("japanese")
	m := Query{Size: 19, Rules: r, HumanProfile: "rank_16k", HumanExplore: 0.5}.toMap()
	os, ok := m["overrideSettings"].(map[string]any)
	if !ok || os["humanSLProfile"] != "rank_16k" || os["ignorePreRootHistory"] != false || os["humanSLRootExploreProbWeightless"] != 0.5 {
		t.Errorf("overrideSettings: %+v", m["overrideSettings"])
	}
	if _, has := (Query{Size: 19, Rules: r}).toMap()["overrideSettings"]; has {
		t.Error("a plain query must not carry override settings")
	}
}
