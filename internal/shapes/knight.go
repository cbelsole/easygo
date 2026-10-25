package shapes

import (
	"fmt"

	"easygo/internal/board"
)

// Knight's move (keima) is an offset of (1,2). The large knight's move
// (ogeima) is (2,3). Both are only reported for two stones of one color that
// are not already one group, with no enemy stone inside their bounding
// rectangle, so a pair that is solidly connected or already cut is not
// flagged.
//
// The spec's "(1,3)" offset is not a standard named shape and is not
// detected.
type knightDetector struct {
	name   string
	dx, dy int
	label  string
}

func (k knightDetector) Name() string { return k.name }

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (k knightDetector) Detect(pos *board.Position, r board.Rules) []Finding {
	b := pos.Board
	size := b.Size
	var out []Finding
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			a := board.Point{X: x, Y: y}
			ca := b.At(a)
			if ca == board.Empty {
				continue
			}
			// Each unordered pair once: look in the four orientations that
			// don't repeat by symmetry (offset in +x,+y and +x,-y, both axis orders).
			for _, o := range [][2]int{{k.dx, k.dy}, {k.dx, -k.dy}, {k.dy, k.dx}, {k.dy, -k.dx}} {
				c := board.Point{X: x + o[0], Y: y + o[1]}
				if !b.InBounds(c) || b.At(c) != ca {
					continue
				}
				if sameGroup(b, a, c) || enemyInside(b, a, c, ca.Opp()) {
					continue
				}
				out = append(out, Finding{
					Kind: k.name, Color: ca, Points: []board.Point{a, c},
					Confidence: Exact,
					Text:       fmt.Sprintf("%s %s: %s and %s.", ca, k.label, a.GTP(size), c.GTP(size)),
				})
			}
		}
	}
	return out
}

func sameGroup(b *board.Board, p, q board.Point) bool {
	stones, _ := b.Group(p)
	for _, s := range stones {
		if s == q {
			return true
		}
	}
	return false
}

func enemyInside(b *board.Board, p, q board.Point, enemy board.Color) bool {
	x0, x1 := min(p.X, q.X), max(p.X, q.X)
	y0, y1 := min(p.Y, q.Y), max(p.Y, q.Y)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if b.At(board.Point{X: x, Y: y}) == enemy {
				return true
			}
		}
	}
	return false
}

func init() {
	Register(knightDetector{name: "knight", dx: 1, dy: 2, label: "knight's move"})
	Register(knightDetector{name: "large-knight", dx: 2, dy: 3, label: "large knight's move"})
}
