// Package joseki recognizes corner patterns in a live game against a
// database of known sequences. It is a lookup, not an evaluation: no KataGo.
//
// Matching is by position, not by move order. Each database entry is the set
// of stones in a corner window, hashed in a canonical frame; a live corner is
// hashed under all 8 board symmetries and both color assignments and looked
// up. That makes transpositions, mid-tree setup stones and captures behave
// without special cases.
package joseki

import (
	"math/rand"

	"easygo/internal/board"
	"easygo/internal/tree"
)

// MaxWindow is the canonical corner window the database is built in.
const MaxWindow = 10

// Corner names which corner of the board a point is measured from.
type Corner string

const (
	TopLeft     Corner = "tl"
	TopRight    Corner = "tr"
	BottomLeft  Corner = "bl"
	BottomRight Corner = "br"
)

var allCorners = []Corner{TopLeft, TopRight, BottomLeft, BottomRight}

var zob [MaxWindow][MaxWindow][3]uint64

func init() {
	r := rand.New(rand.NewSource(0x305e1))
	for x := range zob {
		for y := range zob[x] {
			zob[x][y][board.Black] = r.Uint64()
			zob[x][y][board.White] = r.Uint64()
		}
	}
}

// toCanon maps a board point to canonical corner-relative coordinates.
// refl swaps the two axes, which together with the four corners gives all
// eight symmetries of the board.
func toCanon(size int, c Corner, refl bool, p board.Point) (cx, cy int) {
	switch c {
	case TopLeft:
		cx, cy = p.X, p.Y
	case TopRight:
		cx, cy = size-1-p.X, p.Y
	case BottomLeft:
		cx, cy = p.X, size-1-p.Y
	case BottomRight:
		cx, cy = size-1-p.X, size-1-p.Y
	}
	if refl {
		cx, cy = cy, cx
	}
	return
}

// fromCanon is the inverse of toCanon.
func fromCanon(size int, c Corner, refl bool, cx, cy int) board.Point {
	if refl {
		cx, cy = cy, cx
	}
	switch c {
	case TopLeft:
		return board.Point{X: cx, Y: cy}
	case TopRight:
		return board.Point{X: size - 1 - cx, Y: cy}
	case BottomLeft:
		return board.Point{X: cx, Y: size - 1 - cy}
	}
	return board.Point{X: size - 1 - cx, Y: size - 1 - cy}
}

// windowHash hashes the stones of b inside the w x w window of a corner,
// under one symmetry and optional color swap. It also returns the stone count.
func windowHash(b *board.Board, c Corner, refl, swap bool, w int) (h uint64, n int) {
	for y := 0; y < b.Size; y++ {
		for x := 0; x < b.Size; x++ {
			col := b.At(board.Point{X: x, Y: y})
			if col == board.Empty {
				continue
			}
			cx, cy := toCanon(b.Size, c, refl, board.Point{X: x, Y: y})
			if cx >= w || cy >= w || cx < 0 || cy < 0 {
				continue
			}
			if swap {
				col = col.Opp()
			}
			h ^= zob[cx][cy][col]
			n++
		}
	}
	return
}

// Edge is a book move from one position to the next.
type Edge struct {
	Color board.Color
	X, Y  int // canonical corner-relative coordinates
	Child uint64
}

// Entry is one book position.
type Entry struct {
	Hash    uint64
	Stones  int
	Comment string
	Source  string
	Next    []Edge
}

// DB is the pattern database.
type DB struct {
	Entries map[uint64]*Entry
}

func NewDB() *DB { return &DB{Entries: map[uint64]*Entry{0: {Hash: 0, Source: "root"}}} }

// Size is the number of distinct book positions.
func (d *DB) Size() int { return len(d.Entries) }

const maxComment = 3000

// AddTree walks a joseki tree (as SGF import produced it), plays it on a real
// board, and records every position. frame says which corner the tree's
// moves are written in. Illegal moves cut off that subtree only.
func (d *DB) AddTree(t *tree.Tree, frame Corner, source string) {
	rules, _ := board.Preset("japanese")
	rules.Ko = board.KoSimple
	start, err := t.Position(t.Root)
	if err != nil {
		return
	}
	var walk func(n *tree.Node, pos *board.Position, parent uint64)
	walk = func(n *tree.Node, pos *board.Position, parent uint64) {
		cur := pos
		if n.Parent != nil {
			// Apply setup and the move for this node.
			if len(n.Setup) > 0 {
				cur = cur.WithSetup(n.Setup, n.ToPlay)
			}
			if n.HasMove && !n.Move.IsPass() {
				np, err := cur.WithSetup(nil, n.Color).Play(rules, n.Color, n.Move)
				if err != nil {
					return
				}
				cur = np
			}
		}
		h, stones := windowHash(cur.Board, frame, false, false, MaxWindow)
		hash := parent
		if n.Parent != nil && h != parent {
			e := d.Entries[h]
			if e == nil {
				e = &Entry{Hash: h, Stones: stones, Source: source}
				d.Entries[h] = e
			}
			if len(e.Comment) == 0 && n.Comment != "" {
				c := n.Comment
				if len(c) > maxComment {
					c = c[:maxComment]
				}
				e.Comment = c
			}
			if h != parent && n.HasMove && !n.Move.IsPass() {
				cx, cy := toCanon(t.Size, frame, false, n.Move)
				if cx < MaxWindow && cy < MaxWindow {
					pe := d.Entries[parent]
					if pe != nil && !hasEdge(pe, n.Color, cx, cy) {
						pe.Next = append(pe.Next, Edge{Color: n.Color, X: cx, Y: cy, Child: h})
					}
				}
			}
			hash = h
		}
		for _, c := range n.Children {
			walk(c, cur, hash)
		}
	}
	h0, n0 := windowHash(start.Board, frame, false, false, MaxWindow)
	if d.Entries[h0] == nil {
		d.Entries[h0] = &Entry{Hash: h0, Stones: n0, Source: source}
	}
	walk(t.Root, start, h0)
}

func hasEdge(e *Entry, c board.Color, x, y int) bool {
	for _, ed := range e.Next {
		if ed.Color == c && ed.X == x && ed.Y == y {
			return true
		}
	}
	return false
}
