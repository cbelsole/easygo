package session

import (
	"testing"

	"easygo/internal/board"
	"easygo/internal/clock"
)

func TestSnapshotRestore(t *testing.T) {
	g, _ := New(Options{
		Mode: ModeLive, Size: 9, Rules: rules(t), Name: "Friday game", OwnerID: "u1",
		Black: Player{Name: "Ann", Rank: "3k", Kind: "human", UserID: "u1"},
		White: Player{Name: "Bo", Rank: "2k", Kind: "human", UserID: "u2"},
		Clock: clock.Config{Enabled: true, MainSeconds: 600, Periods: 5, PeriodSeconds: 30},
	})
	g.Play("u1", pt(4, 4), 0)
	g.Play("u2", pt(2, 2), 0)
	g.Play("u1", board.Pass, 0)
	g.Play("u2", board.Pass, 0)
	g.ToggleDead("u1", pt(2, 2), true)

	m, record := g.Snapshot()
	r, err := Restore(m, record)
	if err != nil {
		t.Fatal(err)
	}
	a, b := g.View("u1"), r.View("u1")
	if a.MoveNumber != b.MoveNumber || a.Status != b.Status || a.Name != b.Name {
		t.Errorf("view mismatch: %+v vs %+v", a.Status, b.Status)
	}
	for i := range a.Board {
		if a.Board[i] != b.Board[i] {
			t.Fatalf("board differs at %d", i)
		}
	}
	if !r.Dead()[pt(2, 2)] {
		t.Error("dead-stone override lost")
	}
	if b.Players[0].Rank != "3k" || !b.Players[0].You {
		t.Errorf("players %+v", b.Players)
	}
	if r.Seat("u2") != board.White {
		t.Error("seat ownership lost")
	}
	if b.Rules.Komi != 6.5 || b.Rules.Scoring != board.ScoreTerritory {
		t.Errorf("rules lost: %+v", b.Rules)
	}
}

func TestRestoreKeepsCursorInBranch(t *testing.T) {
	g, _ := New(Options{Mode: ModeTeaching, Size: 9, Rules: rules(t), Black: Player{Kind: "human"}, White: Player{Kind: "human"}})
	g.Play("", pt(4, 4), 0)
	g.Goto("", g.Tree.Root.ID)
	g.Play("", pt(2, 2), 0) // second branch from the root
	g.Play("", pt(3, 3), 0)
	want := g.View("").Cursor

	m, record := g.Snapshot()
	r, err := Restore(m, record)
	if err != nil {
		t.Fatal(err)
	}
	cur := r.Cursor
	if cur.Parent == nil || cur.Parent.Parent != r.Tree.Root || r.Tree.Root.Children[1] != cur.Parent {
		t.Errorf("cursor not in the second branch (was node %d)", want)
	}
}
