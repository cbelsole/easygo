package katago

import (
	"testing"

	"easygo/internal/board"
)

// fill builds an ownership map where every point is v, then applies edits.
func fill(size int, v float64) []float64 {
	o := make([]float64, size*size)
	for i := range o {
		o[i] = v
	}
	return o
}

func find(js []GroupJudgment, p board.Point) GroupJudgment {
	for _, j := range js {
		for _, s := range j.Stones {
			if s == p {
				return j
			}
		}
	}
	return GroupJudgment{}
}

func TestDeadGroupInsideEnemyArea(t *testing.T) {
	b := board.Parse(
		".....",
		".XXX.",
		".X.X.",
		".....",
		"..O..",
	)
	o := fill(5, 0.95) // Black owns everything...
	o[4*5+2] = 0.95    // ...including the lone white stone
	js := Judge(b, o)
	if j := find(js, board.Point{X: 2, Y: 4}); j.Status != Dead {
		t.Errorf("white stone in black area: %+v", j)
	}
	if j := find(js, board.Point{X: 1, Y: 1}); j.Status != Alive || j.Strength != Strong {
		t.Errorf("black group: %+v", j)
	}
}

func TestSekiNeedsSharedLiberty(t *testing.T) {
	// Two groups sharing two liberties, ownership near zero.
	b := board.Parse(
		"XXXX.",
		"X..O.",
		"XXXO.",
		"OOOO.",
		".....",
	)
	o := fill(5, 0.0)
	js := Judge(b, o)
	j := find(js, board.Point{X: 0, Y: 0})
	if j.Status != Seki {
		t.Errorf("shared liberties with zero ownership should be seki: %+v", j)
	}
	// Same ownership but no shared liberty: just unsettled.
	b2 := board.Parse(
		"X....",
		".....",
		".....",
		".....",
		"....O",
	)
	for _, g := range Judge(b2, fill(5, 0.0)) {
		if g.Status != Unsettled {
			t.Errorf("isolated stones with zero ownership: %+v", g)
		}
	}
}

func TestStrengthBands(t *testing.T) {
	b := board.Parse("X....", ".....", ".....", ".....", "....O")
	o := fill(5, 0)
	o[0] = 0.7 // just alive for black
	o[24] = -0.99
	js := Judge(b, o)
	if j := find(js, board.Point{X: 0, Y: 0}); j.Status != Alive || j.Strength != Weak {
		t.Errorf("barely-alive black: %+v", j)
	}
	if j := find(js, board.Point{X: 4, Y: 4}); j.Status != Alive || j.Strength != Strong {
		t.Errorf("white certainly alive: %+v", j)
	}
}

func TestDeadStonesWithOverride(t *testing.T) {
	b := board.Parse("X....", ".....", ".....", ".....", "....O")
	o := fill(5, 0)
	o[0], o[24] = 0.9, 0.9 // white stone sits in black's area: dead
	js := Judge(b, o)
	dead := DeadStones(js, nil)
	if !dead[board.Point{X: 4, Y: 4}] || dead[board.Point{X: 0, Y: 0}] {
		t.Fatalf("dead = %v", dead)
	}
	// The player disagrees and marks the white stone alive.
	dead = DeadStones(js, map[board.Point]bool{{X: 4, Y: 4}: false})
	if len(dead) != 0 {
		t.Errorf("override should clear it: %v", dead)
	}
	// ...and marks the black stone dead.
	dead = DeadStones(js, map[board.Point]bool{{X: 0, Y: 0}: true})
	if !dead[board.Point{X: 0, Y: 0}] {
		t.Errorf("override should force dead: %v", dead)
	}
}

func TestJudgeIgnoresMismatchedOwnership(t *testing.T) {
	if Judge(board.New(5), make([]float64, 3)) != nil {
		t.Error("wrong-size ownership must not be used")
	}
}

func TestTerritoryThreshold(t *testing.T) {
	got := Territory([]float64{0.95, 0.2, -0.9, -0.1}, 0.7)
	want := []board.Color{board.Black, board.Empty, board.White, board.Empty}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("point %d = %v want %v", i, got[i], want[i])
		}
	}
}
