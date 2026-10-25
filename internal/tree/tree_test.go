package tree

import (
	"testing"

	"easygo/internal/board"
)

func newTree(t *testing.T) *Tree {
	t.Helper()
	r, _ := board.Preset("japanese")
	return New(9, r)
}

func TestAddMoveDedupesAndBranches(t *testing.T) {
	tr := newTree(t)
	a, err := tr.AddMove(tr.Root, board.Black, board.Point{X: 2, Y: 2})
	if err != nil {
		t.Fatal(err)
	}
	again, _ := tr.AddMove(tr.Root, board.Black, board.Point{X: 2, Y: 2})
	if again != a {
		t.Error("same move should reuse the existing child")
	}
	b, _ := tr.AddMove(tr.Root, board.Black, board.Point{X: 6, Y: 6})
	if len(tr.Root.Children) != 2 || b == a {
		t.Errorf("branching broken: %d children", len(tr.Root.Children))
	}
}

func TestAddMoveRejectsIllegal(t *testing.T) {
	tr := newTree(t)
	a, _ := tr.AddMove(tr.Root, board.Black, board.Point{X: 4, Y: 4})
	if _, err := tr.AddMove(a, board.White, board.Point{X: 4, Y: 4}); err == nil {
		t.Error("playing on an occupied point must fail")
	}
}

func TestPositionPerBranch(t *testing.T) {
	tr := newTree(t)
	a, _ := tr.AddMove(tr.Root, board.Black, board.Point{X: 4, Y: 4})
	w1, _ := tr.AddMove(a, board.White, board.Point{X: 3, Y: 4})
	w2, _ := tr.AddMove(a, board.White, board.Point{X: 5, Y: 4})
	p1, _ := tr.Position(w1)
	p2, _ := tr.Position(w2)
	if p1.Board.At(board.Point{X: 3, Y: 4}) != board.White || p1.Board.At(board.Point{X: 5, Y: 4}) != board.Empty {
		t.Error("branch 1 board wrong")
	}
	if p2.Board.At(board.Point{X: 5, Y: 4}) != board.White || p2.Board.At(board.Point{X: 3, Y: 4}) != board.Empty {
		t.Error("branch 2 board wrong")
	}
	if tr.MoveNumber(w1) != 2 {
		t.Errorf("move number = %d", tr.MoveNumber(w1))
	}
}

func TestDeleteAndPromote(t *testing.T) {
	tr := newTree(t)
	a, _ := tr.AddMove(tr.Root, board.Black, board.Point{X: 1, Y: 1})
	b, _ := tr.AddMove(tr.Root, board.Black, board.Point{X: 2, Y: 2})
	tr.Promote(b)
	if tr.Root.Children[0] != b {
		t.Error("promote should move b to the main line")
	}
	if err := tr.Delete(a); err != nil || len(tr.Root.Children) != 1 {
		t.Errorf("delete: %v children=%d", err, len(tr.Root.Children))
	}
	if tr.Delete(tr.Root) == nil {
		t.Error("deleting the root must fail")
	}
}

func TestHandicapWhiteMovesFirst(t *testing.T) {
	tr := newTree(t)
	tr.Info.Handicap = 2
	tr.Root.Setup = map[board.Point]board.Color{{X: 2, Y: 2}: board.Black, {X: 6, Y: 6}: board.Black}
	pos, err := tr.Position(tr.Root)
	if err != nil || pos.ToPlay != board.White {
		t.Fatalf("toPlay=%v err=%v", pos.ToPlay, err)
	}
}

func TestSameColorTwiceAllowed(t *testing.T) {
	// SGF permits two black moves in a row (setup-like play).
	tr := newTree(t)
	a := tr.AddChild(tr.Root)
	a.HasMove, a.Color, a.Move = true, board.Black, board.Point{X: 1, Y: 1}
	b := tr.AddChild(a)
	b.HasMove, b.Color, b.Move = true, board.Black, board.Point{X: 2, Y: 2}
	if _, err := tr.Position(b); err != nil {
		t.Errorf("same-color sequence rejected: %v", err)
	}
}
