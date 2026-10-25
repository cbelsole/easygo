package shapes

import (
	"testing"

	"easygo/internal/board"
)

func TestSnapback(t *testing.T) {
	// Black throws in at (0,0). White must take at (1,0) and is then
	// recaptured: four white stones for one black.
	b := board.Parse(
		"..OX.",
		"OOX..",
		"XX...",
		".....",
		".....",
	)
	pos := board.FromBoard(b, board.Black)
	fs := Detect(pos, jp(t), "snapback")
	found := false
	for _, f := range fs {
		if f.Move != nil && *f.Move == (board.Point{X: 0, Y: 0}) {
			found = true
		}
	}
	if !found {
		t.Fatalf("snapback at A5 not found: %+v", fs)
	}
}

func TestNoSnapbackOnOrdinaryPosition(t *testing.T) {
	b := board.New(5)
	b.Set(board.Point{X: 2, Y: 2}, board.White)
	b.Set(board.Point{X: 1, Y: 2}, board.Black)
	if fs := Detect(board.FromBoard(b, board.Black), jp(t), "snapback"); len(fs) != 0 {
		t.Errorf("false snapback: %+v", fs)
	}
}

func TestNetSearchTerminates(t *testing.T) {
	// A white stone on the second line with a couple of black neighbors.
	b := board.New(9)
	b.Set(board.Point{X: 4, Y: 1}, board.White)
	b.Set(board.Point{X: 3, Y: 2}, board.Black)
	b.Set(board.Point{X: 5, Y: 2}, board.Black)
	pos := board.FromBoard(b, board.Black)
	fs := Detect(pos, jp(t), "net")
	for _, f := range fs {
		if f.Confidence != Bounded {
			t.Errorf("net findings must be flagged bounded: %+v", f)
		}
		t.Logf("net: %s", f.Text)
	}
}

func TestNetNeverReportsAnAtari(t *testing.T) {
	b := board.New(9)
	b.Set(board.Point{X: 0, Y: 0}, board.White)
	b.Set(board.Point{X: 1, Y: 0}, board.Black)
	pos := board.FromBoard(b, board.Black)
	for _, f := range Detect(pos, jp(t), "net") {
		t.Errorf("a two-liberty-or-less group is not a net target: %+v", f)
	}
}

// A white stone on the second line. Black at A6 (the net) leaves its only
// extensions, (1,3) and (1,5), as suicide points.
func TestNetFindsGeta(t *testing.T) {
	b := board.Parse(
		".........",
		".........",
		".X.......",
		"..X......",
		".O.......",
		"..X......",
		".X.......",
		".........",
		".........",
	)
	fs := Detect(board.FromBoard(b, board.Black), jp(t), "net")
	found := false
	for _, f := range fs {
		if f.Move != nil && *f.Move == (board.Point{X: 0, Y: 3}) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the net at A6, got %+v", fs)
	}
}
