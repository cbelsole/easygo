package joseki

import (
	"os"
	"path/filepath"
	"testing"

	"easygo/internal/board"
	"easygo/internal/sgf"
	"easygo/internal/tree"
)

// A made-up line in the top-right corner of a 19x19 board with a fork at
// the fourth move. It exercises the machinery; it is not claimed to be
// real joseki.
const testBook = `(;SZ[19]
;B[pd]
;W[qf]
;B[pj]
(;W[nc]C[Branch one.])
(;W[pb]C[Branch two.]))`

func testDB(t *testing.T) *DB {
	t.Helper()
	g, err := sgf.Parse(testBook)
	if err != nil {
		t.Fatal(err)
	}
	db := NewDB()
	db.AddTree(g, TopRight, "test")
	return db
}

// play alternates colors starting with Black.
func play(t *testing.T, size int, moves ...string) (*tree.Tree, *tree.Node) {
	t.Helper()
	r, _ := board.Preset("japanese")
	g := tree.New(size, r)
	n := g.Root
	for i, m := range moves {
		col := board.Black
		if i%2 == 1 {
			col = board.White
		}
		p, err := board.ParseGTP(m, size)
		if err != nil {
			t.Fatal(err)
		}
		n, err = g.AddMove(n, col, p)
		if err != nil {
			t.Fatalf("move %s: %v", m, err)
		}
	}
	return g, n
}

func cornerOf(ms []CornerMatch, c Corner) (CornerMatch, bool) {
	for _, m := range ms {
		if m.Corner == c {
			return m, true
		}
	}
	return CornerMatch{}, false
}

func TestBuildCountsPositions(t *testing.T) {
	// empty, B, B+W, B+W+B, and two branches
	if got := testDB(t).Size(); got != 6 {
		t.Errorf("entries = %d, want 6", got)
	}
}

func TestMatchesInOriginalCorner(t *testing.T) {
	db := testDB(t)
	g, n := play(t, 19, "Q16", "R14", "Q10", "O17")
	tr, ok := cornerOf(db.Analyze(g, n), TopRight)
	if !ok || tr.Status != StatusJoseki || tr.Comment != "Branch one." {
		t.Fatalf("top-right: %+v", tr)
	}
}

func TestMatchesReflectedCornerWithColorsSwapped(t *testing.T) {
	db := testDB(t)
	// The book line in the bottom-left corner, reflected across the diagonal,
	// with White playing the book's Black role. Black first plays far away so
	// the colors alternate naturally.
	g, n := play(t, 19, "T19", "D4", "F3", "K4")
	bl, ok := cornerOf(db.Analyze(g, n), BottomLeft)
	if !ok || bl.Status != StatusJoseki || !bl.Swapped {
		t.Fatalf("bottom-left: %+v", bl)
	}
	if len(bl.Next) != 2 {
		t.Fatalf("continuations: %+v", bl.Next)
	}
	// Book continuations are White moves in the book; with colors swapped they are Black's.
	for _, s := range bl.Next {
		if s.Color != board.Black {
			t.Errorf("swapped continuation should be black: %+v", s)
		}
	}
}

func TestSuggestsBookContinuations(t *testing.T) {
	db := testDB(t)
	g, n := play(t, 19, "Q16", "R14", "Q10")
	tr, _ := cornerOf(db.Analyze(g, n), TopRight)
	if tr.Status != StatusJoseki || len(tr.Next) != 2 {
		t.Fatalf("expected two continuations: %+v", tr)
	}
	want := map[board.Point]bool{}
	for _, s := range []string{"O17", "Q18"} {
		p, _ := board.ParseGTP(s, 19)
		want[p] = true
	}
	for _, s := range tr.Next {
		if !want[s.Point] || s.Color != board.White {
			t.Errorf("unexpected suggestion %+v", s)
		}
	}
}

func TestDeviationReported(t *testing.T) {
	db := testDB(t)
	// White's fourth move is in the corner window but not in the book.
	g, n := play(t, 19, "Q16", "R14", "Q10", "T17")
	tr, ok := cornerOf(db.Analyze(g, n), TopRight)
	if !ok || tr.Status != StatusDeviated || tr.DeviatedAt != 3 || len(tr.Next) == 0 {
		t.Errorf("deviation: %+v", tr)
	}
}

func TestTooFewStonesNoMatch(t *testing.T) {
	db := testDB(t)
	g, n := play(t, 19, "Q16", "R14")
	if ms := db.Analyze(g, n); len(ms) != 0 {
		t.Errorf("two stones should not be reported: %+v", ms)
	}
}

func TestSmallBoardsDisabled(t *testing.T) {
	db := testDB(t)
	g, n := play(t, 9, "E5", "D4", "C3")
	if ms := db.Analyze(g, n); len(ms) != 0 {
		t.Errorf("9x9 should not report joseki: %+v", ms)
	}
}

// Against the real dictionary: a line copied from the book (3-3 point,
// then the hane) is recognized in its own corner and, mirrored,
// in another one. Skipped when the dictionary has not been fetched.
func TestRealDictionary(t *testing.T) {
	path := filepath.Join("..", "..", "data", "cache", KogoFile)
	if _, err := os.Stat(path); err != nil {
		t.Skip("dictionary not fetched")
	}
	db, err := LoadFile(path, "kogo")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("positions: %d", db.Size())
	if db.Size() < 5000 {
		t.Fatalf("only %d positions; the dictionary has far more", db.Size())
	}
	// SGF qc, pd, qd, pe, ob, md in the top-right corner (the 3-3 point section).
	g, n := play(t, 19, "R17", "Q16", "R16", "Q15", "P18", "N16")
	tr, ok := cornerOf(db.Analyze(g, n), TopRight)
	if !ok || tr.Status != StatusJoseki {
		t.Fatalf("top-right: %+v", tr)
	}
	t.Logf("book offers %d continuations", len(tr.Next))
	// The same line mirrored left-right lands in the top-left corner.
	g2, n2 := play(t, 19, "C17", "D16", "C16", "D15", "E18", "G16")
	tl, ok := cornerOf(db.Analyze(g2, n2), TopLeft)
	if !ok || tl.Status != StatusJoseki || len(tl.Next) != len(tr.Next) {
		t.Fatalf("top-left mirror: %+v (top-right had %d next)", tl, len(tr.Next))
	}
}
