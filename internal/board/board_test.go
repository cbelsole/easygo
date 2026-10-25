package board

import "testing"

func mustPlay(t *testing.T, p *Position, r Rules, c Color, x, y int) *Position {
	t.Helper()
	np, err := p.Play(r, c, Point{x, y})
	if err != nil {
		t.Fatalf("play %v (%d,%d): %v", c, x, y, err)
	}
	return np
}

func rules(t *testing.T, name string) Rules {
	t.Helper()
	r, err := Preset(name)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestGTPRoundTrip(t *testing.T) {
	for _, s := range []string{"A1", "D4", "J9", "T19", "K10"} {
		p, err := ParseGTP(s, 19)
		if s == "J9" || s == "T19" || s == "K10" || s == "D4" || s == "A1" {
			if err != nil {
				t.Fatalf("%s: %v", s, err)
			}
			if got := p.GTP(19); got != s {
				t.Errorf("%s round-tripped to %s", s, got)
			}
		}
	}
	if p, _ := ParseGTP("D4", 19); p != (Point{3, 15}) {
		t.Errorf("D4 on 19 = %v", p)
	}
	if _, err := ParseGTP("I5", 19); err == nil {
		// I is skipped in GTP, so "I5" must not parse as column 8.
		p, _ := ParseGTP("I5", 19)
		if p.X == 8 {
			t.Error("I should not be a valid column")
		}
	}
}

func TestCapture(t *testing.T) {
	r := rules(t, "japanese")
	p := NewPosition(5)
	p = mustPlay(t, p, r, Black, 1, 0)
	p = mustPlay(t, p, r, White, 0, 0)
	p = mustPlay(t, p, r, Black, 0, 1)
	if p.Board.At(Point{0, 0}) != Empty {
		t.Fatal("corner stone should be captured")
	}
	if p.Captures[Black] != 1 {
		t.Errorf("captures = %d", p.Captures[Black])
	}
}

func TestLibertiesAndGroup(t *testing.T) {
	b := Parse(
		".....",
		".XX..",
		".XO..",
		".....",
		".....",
	)
	// Black chain is (1,1),(2,1),(1,2); the white stone at (2,2) is walled in on two sides.
	if n := b.Liberties(Point{1, 1}); n != 6 {
		t.Errorf("black group libs = %d, want 6", n)
	}
	if n := b.Liberties(Point{2, 2}); n != 2 {
		t.Errorf("white stone libs = %d, want 2", n)
	}
	st, _ := b.Group(Point{1, 1})
	if len(st) != 3 {
		t.Errorf("group size = %d", len(st))
	}
}

func TestSuicide(t *testing.T) {
	b := Parse(
		".X...",
		"X....",
		".....",
		".....",
		".....",
	)
	p := FromBoard(b, White)
	if _, err := p.Play(rules(t, "japanese"), White, Point{0, 0}); err != ErrSuicide {
		t.Errorf("single-stone suicide: %v", err)
	}
	// Capturing makes it legal.
	b2 := Parse(
		".XO..",
		"XO...",
		".....",
		".....",
		".....",
	)
	// white at (0,0) would have no libs but... black (1,0) has libs (1,1)? (1,1)=O. Black (1,0): neighbors (0,0),(2,0)=O,(1,1)=O -> only lib (0,0).
	p2 := FromBoard(b2, White)
	np, err := p2.Play(rules(t, "japanese"), White, Point{0, 0})
	if err != nil {
		t.Fatalf("capturing move should be legal: %v", err)
	}
	if np.Board.At(Point{1, 0}) != Empty {
		t.Error("black stone at (1,0) should be captured")
	}
}

func TestMultiStoneSuicideRule(t *testing.T) {
	b := Parse(
		".OX..",
		"OX...",
		"X....",
		".....",
		".....",
	)
	// white O at (1,0) and (0,1) are separate; white plays (0,0) to form a 3-stone group with 0 libs? neighbors:
	// (0,0)-(1,0)-... (1,0) nbrs: (2,0)=X,(1,1)=X ; (0,1) nbrs: (0,2)=X,(1,1)=X. so group {(0,0),(1,0),(0,1)} no libs.
	p := FromBoard(b, White)
	if _, err := p.Play(rules(t, "japanese"), White, Point{0, 0}); err != ErrSuicide {
		t.Errorf("japanese forbids multi-stone suicide: %v", err)
	}
	np, err := p.Play(rules(t, "tromp-taylor"), White, Point{0, 0})
	if err != nil {
		t.Fatalf("tromp-taylor allows multi-stone suicide: %v", err)
	}
	if np.Board.At(Point{0, 0}) != Empty || np.Captures[Black] != 3 {
		t.Errorf("suicided stones should be removed and credited to black (caps=%d)", np.Captures[Black])
	}
}

// A standard single-stone ko in the corner.
func koPosition() *Position {
	b := Parse(
		".XO..",
		"XO.O.",
		".XO..",
		".....",
		".....",
	)
	return FromBoard(b, Black)
}

func TestSimpleKo(t *testing.T) {
	r := rules(t, "japanese")
	p := koPosition()
	p = mustPlay(t, p, r, Black, 2, 1) // black captures at (2,1), taking O(1,1)
	if p.Board.At(Point{1, 1}) != Empty {
		t.Fatal("expected capture of (1,1)")
	}
	if _, err := p.Play(r, White, Point{1, 1}); err != ErrKo {
		t.Errorf("immediate recapture should be ko, got %v", err)
	}
	// After a ko threat exchange, recapture is legal.
	p = mustPlay(t, p, r, White, 4, 4)
	p = mustPlay(t, p, r, Black, 4, 3)
	if _, err := p.Play(r, White, Point{1, 1}); err != nil {
		t.Errorf("recapture after threats: %v", err)
	}
}

func TestPassesEndGame(t *testing.T) {
	r := rules(t, "japanese")
	p := NewPosition(9)
	p, _ = p.Play(r, Black, Pass)
	if p.Over() {
		t.Fatal("one pass does not end the game")
	}
	p, _ = p.Play(r, White, Pass)
	if !p.Over() {
		t.Fatal("two passes end the game")
	}
	p2 := mustPlay(t, NewPosition(9), r, Black, 4, 4)
	if p2.Passes != 0 {
		t.Error("stone move resets passes")
	}
}

func TestWrongTurnAndOccupied(t *testing.T) {
	r := rules(t, "japanese")
	p := NewPosition(9)
	if _, err := p.Play(r, White, Point{1, 1}); err != ErrWrongTurn {
		t.Errorf("wrong turn: %v", err)
	}
	p = mustPlay(t, p, r, Black, 1, 1)
	if _, err := p.Play(r, White, Point{1, 1}); err != ErrOccupied {
		t.Errorf("occupied: %v", err)
	}
	if _, err := p.Play(r, White, Point{9, 1}); err != ErrOffBoard {
		t.Errorf("offboard: %v", err)
	}
}

func TestHandicap(t *testing.T) {
	for _, tc := range []struct{ size, n int }{{19, 2}, {19, 9}, {13, 5}, {9, 4}} {
		p, err := Handicap(tc.size, tc.n)
		if err != nil {
			t.Fatalf("%v: %v", tc, err)
		}
		count := 0
		for _, g := range p.Board.Groups() {
			count += len(g.Stones)
		}
		if count != tc.n || p.ToPlay != White {
			t.Errorf("%v: stones=%d toPlay=%v", tc, count, p.ToPlay)
		}
	}
	if _, err := Handicap(19, 1); err == nil {
		t.Error("handicap 1 should be rejected")
	}
}

func TestPresetsValidate(t *testing.T) {
	for _, n := range PresetNames() {
		r, err := Preset(n)
		if err != nil || r.Validate() != nil {
			t.Errorf("preset %s invalid: %v / %v", n, err, r.Validate())
		}
	}
}

// Every repetition rule must stop the immediate recapture in a plain ko and
// allow it once the board has changed.
func TestAllKoRulesStopImmediateRecapture(t *testing.T) {
	for _, ko := range []KoRule{KoSimple, KoPositional, KoSituational} {
		r := rules(t, "japanese")
		r.Ko = ko
		p := koPosition()
		p = mustPlay(t, p, r, Black, 2, 1)
		if _, err := p.Play(r, White, Point{1, 1}); err != ErrKo {
			t.Errorf("%s: immediate recapture: %v", ko, err)
		}
		p = mustPlay(t, p, r, White, 4, 4) // ko threat
		p = mustPlay(t, p, r, Black, 4, 3) // answered
		if _, err := p.Play(r, White, Point{1, 1}); err != nil {
			t.Errorf("%s: recapture after a threat exchange: %v", ko, err)
		}
	}
}
