package shapes

import (
	"testing"

	"easygo/internal/board"
)

func TestKnightMove(t *testing.T) {
	b := board.New(9)
	b.Set(board.Point{X: 2, Y: 2}, board.Black)
	b.Set(board.Point{X: 3, Y: 4}, board.Black)
	fs := Detect(board.FromBoard(b, board.White), jp(t), "knight", "large-knight")
	if len(fs) != 1 || fs[0].Kind != "knight" || fs[0].Color != board.Black {
		t.Fatalf("findings: %+v", fs)
	}
}

func TestLargeKnightMove(t *testing.T) {
	b := board.New(9)
	b.Set(board.Point{X: 2, Y: 2}, board.White)
	b.Set(board.Point{X: 4, Y: 5}, board.White) // (2,3) offset
	b.Set(board.Point{X: 8, Y: 0}, board.White) // far, forms no pair
	fs := Detect(board.FromBoard(b, board.Black), jp(t), "large-knight")
	if len(fs) != 1 || fs[0].Kind != "large-knight" {
		t.Fatalf("findings: %+v", fs)
	}
	if got := Detect(board.FromBoard(b, board.Black), jp(t), "knight"); len(got) != 0 {
		t.Errorf("large knight misread as knight: %+v", got)
	}
}

func TestKnightSkipsConnectedOrCutPairs(t *testing.T) {
	// Connected through a chain: not a knight's-move shape.
	b := board.Parse(
		".........",
		".........",
		"..X......",
		"..XX.....",
		"...X.....",
		".........",
		".........",
		".........",
		".........",
	)
	if fs := Detect(board.FromBoard(b, board.White), jp(t), "knight"); len(fs) != 0 {
		t.Errorf("connected stones flagged: %+v", fs)
	}
	// An enemy stone inside the rectangle cuts the shape.
	c := board.New(9)
	c.Set(board.Point{X: 2, Y: 2}, board.Black)
	c.Set(board.Point{X: 3, Y: 4}, board.Black)
	c.Set(board.Point{X: 3, Y: 3}, board.White)
	if fs := Detect(board.FromBoard(c, board.White), jp(t), "knight"); len(fs) != 0 {
		t.Errorf("cut pair flagged: %+v", fs)
	}
}
