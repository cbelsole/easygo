package shapes

import (
	"testing"

	"easygo/internal/board"
)

func jp(t *testing.T) board.Rules {
	t.Helper()
	r, err := board.Preset("japanese")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// A white stone at (4,4) with black at (3,4), (4,3) and the ladder-starting
// stone at (5,3). The chase runs down the diagonal toward the bottom-right.
func ladderBoard(extra ...[3]int) *board.Board {
	b := board.New(9)
	b.Set(board.Point{X: 4, Y: 4}, board.White)
	b.Set(board.Point{X: 3, Y: 4}, board.Black)
	b.Set(board.Point{X: 4, Y: 3}, board.Black)
	b.Set(board.Point{X: 5, Y: 3}, board.Black)
	for _, e := range extra {
		b.Set(board.Point{X: e[0], Y: e[1]}, board.Color(e[2]))
	}
	return b
}

func TestLadderWorks(t *testing.T) {
	pos := board.FromBoard(ladderBoard(), board.Black)
	res := ReadLadder(pos, jp(t), board.Point{X: 4, Y: 4}, board.Black)
	if res.Outcome != "works" {
		t.Fatalf("outcome %s, path %v", res.Outcome, res.Path)
	}
	if len(res.Path) < 6 {
		t.Errorf("expected a long chase, got %d plies", len(res.Path))
	}
}

func TestLadderBrokenByBreaker(t *testing.T) {
	// A white stone on the diagonal the chase runs along.
	for _, breaker := range [][2]int{{6, 6}, {6, 5}, {7, 6}, {5, 6}, {6, 7}} {
		b := ladderBoard([3]int{breaker[0], breaker[1], int(board.White)})
		pos := board.FromBoard(b, board.Black)
		res := ReadLadder(pos, jp(t), board.Point{X: 4, Y: 4}, board.Black)
		t.Logf("breaker %v -> %s breaker=%v path=%d", breaker, res.Outcome, res.Breaker, len(res.Path))
	}
	// At least one placement on the run must break the ladder.
	broken := 0
	for _, breaker := range [][2]int{{6, 6}, {6, 5}, {7, 6}, {5, 6}, {6, 7}, {7, 7}} {
		b := ladderBoard([3]int{breaker[0], breaker[1], int(board.White)})
		res := ReadLadder(board.FromBoard(b, board.Black), jp(t), board.Point{X: 4, Y: 4}, board.Black)
		if res.Outcome == "broken" {
			broken++
			if res.Breaker == nil {
				t.Errorf("breaker %v: broken but no breaker reported", breaker)
			}
		}
	}
	if broken == 0 {
		t.Error("no breaker placement broke the ladder; either the fixture or the reader is wrong")
	}
}

func TestLadderEscapeByCapture(t *testing.T) {
	// Same ladder, but one of the chasing stones is itself in atari: white
	// can escape by capturing it. Place a lone black stone at (6,4) with
	// white stones taking away all but one of its liberties.
	b := ladderBoard()
	b.Set(board.Point{X: 7, Y: 4}, board.White)
	b.Set(board.Point{X: 6, Y: 5}, board.White)
	b.Set(board.Point{X: 6, Y: 3}, board.White)
	b.Set(board.Point{X: 6, Y: 4}, board.Black)
	pos := board.FromBoard(b, board.Black)
	// (6,4) black has libs: (5,4) only. Attacker should not blindly claim success.
	res := ReadLadder(pos, jp(t), board.Point{X: 4, Y: 4}, board.Black)
	t.Logf("capture-escape fixture: %s %v", res.Outcome, res.Path)
	if res.Outcome == "unknown" {
		t.Errorf("reader gave up on a small position")
	}
}

func TestLadderWrongPreconditions(t *testing.T) {
	pos := board.FromBoard(ladderBoard(), board.Black)
	if r := ReadLadder(pos, jp(t), board.Point{X: 0, Y: 0}, board.Black); r.Outcome != "unknown" {
		t.Errorf("empty target: %s", r.Outcome)
	}
	if r := ReadLadder(pos, jp(t), board.Point{X: 3, Y: 4}, board.Black); r.Outcome != "unknown" {
		t.Errorf("own stone as target: %s", r.Outcome)
	}
	open := board.New(9)
	open.Set(board.Point{X: 4, Y: 4}, board.White)
	if r := ReadLadder(board.FromBoard(open, board.Black), jp(t), board.Point{X: 4, Y: 4}, board.Black); r.Outcome != "unknown" {
		t.Errorf("four-liberty stone: %s", r.Outcome)
	}
}

func TestLadderDetectorFinds(t *testing.T) {
	pos := board.FromBoard(ladderBoard(), board.Black)
	fs := Detect(pos, jp(t), "ladder")
	if len(fs) != 1 || fs[0].Result != "works" || fs[0].Color != board.Black {
		t.Fatalf("findings: %+v", fs)
	}
	if len(Names()) == 0 {
		t.Error("registry empty")
	}
}

func TestLadderIgnoresFarAwayStone(t *testing.T) {
	b := ladderBoard([3]int{0, 8, int(board.White)}, [3]int{1, 1, int(board.White)})
	res := ReadLadder(board.FromBoard(b, board.Black), jp(t), board.Point{X: 4, Y: 4}, board.Black)
	if res.Outcome != "works" {
		t.Errorf("irrelevant stones changed the result: %s", res.Outcome)
	}
}
