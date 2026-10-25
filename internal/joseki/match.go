package joseki

import (
	"sort"

	"easygo/internal/board"
	"easygo/internal/tree"
)

// Status of a corner against the book.
const (
	StatusJoseki   = "joseki"   // the corner is exactly a book position
	StatusDeviated = "deviated" // it matched earlier and then left the book
	StatusNone     = "none"
)

// MinStones is how many stones a corner needs before a match is reported.
// Below this, nearly every corner "matches" something.
const MinStones = 3

// Suggestion is a book continuation in board coordinates.
type Suggestion struct {
	Color   board.Color `json:"color"`
	Point   board.Point `json:"point"`
	Comment string      `json:"comment,omitempty"`
}

// CornerMatch is the verdict for one corner.
type CornerMatch struct {
	Corner  Corner       `json:"corner"`
	Status  string       `json:"status"`
	Stones  int          `json:"stones"`
	Comment string       `json:"comment,omitempty"` // book commentary on this position
	Next    []Suggestion `json:"next,omitempty"`
	// DeviatedAt is the move number after which the corner left the book.
	DeviatedAt int `json:"deviatedAt,omitempty"`
	// Swapped is true if the match needed Black and White exchanged.
	Swapped bool `json:"swapped,omitempty"`
}

// variants enumerates the (reflection, color swap) pairs: 4 corners x 2 x 2
// give the 8 board symmetries with either color first.
var variants = [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}}

// window returns the corner window for a board size, or 0 if the board is
// too small for joseki to mean anything.
func window(size int) int {
	switch {
	case size >= 19:
		return MaxWindow
	case size >= 13:
		return 8
	}
	return 0
}

type hit struct {
	refl, swap bool
	entry      *Entry
}

// lookup finds every variant of a corner that is a book position.
func (d *DB) lookup(b *board.Board, c Corner) (hits []hit, stones int) {
	w := window(b.Size)
	if w == 0 {
		return nil, 0
	}
	for _, v := range variants {
		h, n := windowHash(b, c, v[0], v[1], w)
		stones = n
		if n < MinStones {
			continue
		}
		if e, ok := d.Entries[h]; ok {
			hits = append(hits, hit{v[0], v[1], e})
		}
	}
	return hits, stones
}

// Analyze checks all four corners of the position after node n. For a
// corner that matched earlier in the game and no longer does, it reports
// where the game left the book and what the book offered there.
func (d *DB) Analyze(t *tree.Tree, n *tree.Node) []CornerMatch {
	path := t.Path(n)
	last := map[Corner]struct {
		hits []hit
		move int
	}{}
	var finalPos *board.Position
	for _, node := range path {
		pos, err := t.Position(node)
		if err != nil {
			break
		}
		finalPos = pos
		for _, c := range allCorners {
			hits, _ := d.lookup(pos.Board, c)
			if len(hits) > 0 {
				last[c] = struct {
					hits []hit
					move int
				}{hits, pos.MoveNum}
			}
		}
	}
	if finalPos == nil {
		return nil
	}
	size := finalPos.Board.Size
	var out []CornerMatch
	for _, c := range allCorners {
		hits, stones := d.lookup(finalPos.Board, c)
		cm := CornerMatch{Corner: c, Stones: stones, Status: StatusNone}
		switch {
		case len(hits) > 0:
			cm.Status = StatusJoseki
			cm.Comment = hits[0].entry.Comment
			cm.Swapped = hits[0].swap
			cm.Next = d.suggest(size, c, hits, finalPos.Board)
		case last[c].hits != nil && stones >= MinStones:
			cm.Status = StatusDeviated
			cm.DeviatedAt = last[c].move
			cm.Comment = last[c].hits[0].entry.Comment
			cm.Swapped = last[c].hits[0].swap
			cm.Next = d.suggest(size, c, last[c].hits, finalPos.Board)
		default:
			continue
		}
		out = append(out, cm)
	}
	return out
}

// suggest converts the book continuations of every hit back to board
// coordinates, dropping duplicates and occupied points.
func (d *DB) suggest(size int, c Corner, hits []hit, b *board.Board) []Suggestion {
	seen := map[Suggestion]bool{}
	var out []Suggestion
	for _, h := range hits {
		for _, e := range h.entry.Next {
			col := e.Color
			if h.swap {
				col = col.Opp()
			}
			p := fromCanon(size, c, h.refl, e.X, e.Y)
			if !b.InBounds(p) || b.At(p) != board.Empty {
				continue
			}
			s := Suggestion{Color: col, Point: p}
			if ce := d.Entries[e.Child]; ce != nil {
				s.Comment = shorten(ce.Comment, 240)
			}
			key := Suggestion{Color: col, Point: p}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Point.Y != out[j].Point.Y {
			return out[i].Point.Y < out[j].Point.Y
		}
		return out[i].Point.X < out[j].Point.X
	})
	return out
}

func shorten(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
