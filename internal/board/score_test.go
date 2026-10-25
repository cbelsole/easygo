package board

import "testing"

// 5x5: black owns the left two columns, white the right two, column 2 is a wall of mixed stones.
func scoreFixture() *Position {
	return FromBoard(Parse(
		".XXO.",
		".XXO.",
		".XXO.",
		".XXO.",
		".XXO.",
	), Black)
}

func TestAreaScoring(t *testing.T) {
	r := rules(t, "chinese")
	r.Komi = 0
	res := Score(scoreFixture(), r, ScoreOpts{})
	// Black: 10 stones + 5 territory; White: 5 stones + 5 territory.
	if res.Black != 15 || res.White != 10 {
		t.Errorf("area score B=%v W=%v", res.Black, res.White)
	}
	if res.BlackTerr != 5 || res.WhiteTerr != 5 {
		t.Errorf("territory %d/%d", res.BlackTerr, res.WhiteTerr)
	}
}

func TestTerritoryScoringUsesPrisoners(t *testing.T) {
	r := rules(t, "japanese")
	r.Komi = 0
	pos := scoreFixture()
	pos.Captures[Black] = 3
	res := Score(pos, r, ScoreOpts{})
	if res.Black != 5+3 || res.White != 5 {
		t.Errorf("territory score B=%v W=%v", res.Black, res.White)
	}
}

func TestDeadStonesAreRemoved(t *testing.T) {
	// A lone white stone deep in black territory, marked dead.
	b := Parse(
		"X.X..",
		"XOX..",
		"XXX..",
		".....",
		".....",
	)
	pos := FromBoard(b, Black)
	r := rules(t, "japanese")
	r.Komi = 0
	res := Score(pos, r, ScoreOpts{Dead: map[Point]bool{{1, 1}: true}})
	// Region at (1,0) joined with (1,1): the former O point plus (1,0) = 2 black territory.
	// Right side and bottom is all one big empty region touching only black... touches X only -> black's.
	if res.WhiteTerr != 0 {
		t.Errorf("white should own nothing, got %d", res.WhiteTerr)
	}
	if res.Black <= res.White {
		t.Errorf("black should lead: %v vs %v", res.Black, res.White)
	}
	// One dead white stone becomes a prisoner for black under territory scoring.
	noDead := Score(pos, r, ScoreOpts{})
	if res.Black-noDead.Black < 1 {
		t.Errorf("dead stone should add to black's count: %v -> %v", noDead.Black, res.Black)
	}
}

func TestHandicapBonus(t *testing.T) {
	r := rules(t, "aga")
	r.Komi = 0
	res := Score(scoreFixture(), r, ScoreOpts{Handicap: 4})
	// AGA gives white N-1 = 3 on top of area score 10.
	if res.White != 13 {
		t.Errorf("white = %v, want 13", res.White)
	}
}

func TestNeutralRegionsNotCounted(t *testing.T) {
	b := Parse(
		"X.O..",
		"X.O..",
		"X.O..",
		"X.O..",
		"X.O..",
	)
	r := rules(t, "japanese")
	r.Komi = 0
	res := Score(FromBoard(b, Black), r, ScoreOpts{})
	if res.Neutral != 5 {
		t.Errorf("dame column = %d, want 5", res.Neutral)
	}
}
