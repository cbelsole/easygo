package sgf

import (
	"strings"
	"testing"

	"easygo/internal/board"
)

const sample = `(;GM[1]FF[4]SZ[19]CA[UTF-8]KM[6.5]PB[Black Player]PW[White]
AB[pd]AW[dp]
;B[dd]
;W[pp]
(;B[fq]C[Approach from the left. A \] bracket\\ and a
second line.]
 ;W[cn]
 ;B[fp])
(;B[jp]C[Or tenuki]TR[aa][bb]LB[cc:A]))`

func TestParseStructure(t *testing.T) {
	g, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	if g.Size != 19 || g.Rules.Komi != 6.5 || g.Info.BlackName != "Black Player" {
		t.Errorf("root info: size=%d komi=%v pb=%q", g.Size, g.Rules.Komi, g.Info.BlackName)
	}
	if got := len(g.Root.Setup); got != 2 {
		t.Errorf("setup stones = %d", got)
	}
	if len(g.Root.Children) != 1 {
		t.Fatalf("root children = %d", len(g.Root.Children))
	}
}

func TestBranchesAndComments(t *testing.T) {
	g, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	line := g.MainLine()
	// root, B dd, W pp, then the first variation: B fq, W cn, B fp
	if len(line) != 6 {
		t.Fatalf("main line length %d", len(line))
	}
	fq := line[3]
	if fq.Color != board.Black || fq.Move != (board.Point{X: 5, Y: 16}) {
		t.Errorf("fq = %v %v", fq.Color, fq.Move)
	}
	if !strings.Contains(fq.Comment, "A ] bracket\\") || !strings.Contains(fq.Comment, "second line") {
		t.Errorf("comment escapes lost: %q", fq.Comment)
	}
	parent := fq.Parent
	if len(parent.Children) != 2 {
		t.Fatalf("variations = %d", len(parent.Children))
	}
	alt := parent.Children[1]
	if alt.Comment != "Or tenuki" || len(alt.Marks) != 3 {
		t.Errorf("alt comment=%q marks=%d", alt.Comment, len(alt.Marks))
	}
}

func TestPositionFromParsedTree(t *testing.T) {
	g, _ := Parse(sample)
	end := g.MainLine()[len(g.MainLine())-1]
	pos, err := g.Position(end)
	if err != nil {
		t.Fatal(err)
	}
	if pos.Board.At(board.Point{X: 5, Y: 15}) == board.Empty && pos.Board.At(board.Point{X: 5, Y: 16}) == board.Empty {
		t.Error("expected stones on the board")
	}
	// Setup stones plus 5 moves on the board.
	count := 0
	for _, gr := range pos.Board.Groups() {
		count += len(gr.Stones)
	}
	if count != 7 {
		t.Errorf("stones on board = %d, want 7", count)
	}
}

func TestRoundTrip(t *testing.T) {
	g, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	out := Write(g)
	g2, err := Parse(out)
	if err != nil {
		t.Fatalf("reparse: %v\n%s", err, out)
	}
	if out2 := Write(g2); out != out2 {
		t.Errorf("write is not stable:\n%s\n%s", out, out2)
	}
	a, b := g.MainLine(), g2.MainLine()
	if len(a) != len(b) {
		t.Fatalf("main line %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Comment != b[i].Comment || a[i].Move != b[i].Move {
			t.Errorf("node %d differs", i)
		}
	}
}

func TestPassAndTT(t *testing.T) {
	g, err := Parse("(;SZ[9];B[ee];W[];B[tt])")
	if err != nil {
		t.Fatal(err)
	}
	line := g.MainLine()
	if !line[2].Move.IsPass() || !line[3].Move.IsPass() {
		t.Error("[] and [tt] should both be passes on small boards")
	}
}

func TestCompressedPoints(t *testing.T) {
	g, err := Parse("(;SZ[9]AB[aa:cc])")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Root.Setup) != 9 {
		t.Errorf("compressed rect gave %d points", len(g.Root.Setup))
	}
}

func TestUnknownPropsPreserved(t *testing.T) {
	g, err := Parse("(;SZ[9];B[ee]XX[hello][world])")
	if err != nil {
		t.Fatal(err)
	}
	out := Write(g)
	if !strings.Contains(out, "XX[hello][world]") {
		t.Errorf("unknown property dropped: %s", out)
	}
}

func TestBadInput(t *testing.T) {
	if _, err := Parse("not sgf at all"); err == nil {
		t.Error("expected error")
	}
	if _, err := Parse("(;SZ[9:13])"); err == nil {
		t.Error("non-square board should be rejected")
	}
}

func TestMultipleGames(t *testing.T) {
	gs, err := ParseAll("(;SZ[9];B[aa])(;SZ[13];B[bb])")
	if err != nil || len(gs) != 2 || gs[1].Size != 13 {
		t.Fatalf("games=%d err=%v", len(gs), err)
	}
}
