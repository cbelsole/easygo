// Package board is the Go rules engine: stones, groups, liberties, captures.
// Origin is the top-left corner; X is the column, Y the row.
package board

import (
	"fmt"
	"math/rand"
)

// MaxSize bounds the Zobrist tables. Traditional sizes are 9, 13 and 19.
const MaxSize = 25

type Color uint8

const (
	Empty Color = iota
	Black
	White
)

func (c Color) Opp() Color {
	switch c {
	case Black:
		return White
	case White:
		return Black
	}
	return Empty
}

func (c Color) String() string {
	switch c {
	case Black:
		return "B"
	case White:
		return "W"
	}
	return "."
}

type Point struct{ X, Y int }

// Pass is the sentinel point for a pass move.
var Pass = Point{-1, -1}

func (p Point) IsPass() bool { return p.X < 0 || p.Y < 0 }

// GTP returns "D4"-style coordinates (column letters skip I).
func (p Point) GTP(size int) string {
	if p.IsPass() {
		return "pass"
	}
	const letters = "ABCDEFGHJKLMNOPQRSTUVWXYZ"
	return fmt.Sprintf("%c%d", letters[p.X], size-p.Y)
}

// ParseGTP is the inverse of GTP.
func ParseGTP(s string, size int) (Point, error) {
	if s == "pass" || s == "PASS" {
		return Pass, nil
	}
	if len(s) < 2 {
		return Point{}, fmt.Errorf("bad coordinate %q", s)
	}
	const letters = "ABCDEFGHJKLMNOPQRSTUVWXYZ"
	col := -1
	for i := 0; i < len(letters); i++ {
		if letters[i] == s[0] || letters[i]+32 == s[0] {
			col = i
			break
		}
	}
	var row int
	if _, err := fmt.Sscanf(s[1:], "%d", &row); err != nil || col < 0 {
		return Point{}, fmt.Errorf("bad coordinate %q", s)
	}
	p := Point{col, size - row}
	if p.X >= size || p.Y < 0 || p.Y >= size {
		return Point{}, fmt.Errorf("coordinate %q off the %d-board", s, size)
	}
	return p, nil
}

type Board struct {
	Size  int
	cells []Color
}

func New(size int) *Board {
	if size < 2 || size > MaxSize {
		panic(fmt.Sprintf("board: unsupported size %d", size))
	}
	return &Board{Size: size, cells: make([]Color, size*size)}
}

func (b *Board) Clone() *Board {
	c := &Board{Size: b.Size, cells: make([]Color, len(b.cells))}
	copy(c.cells, b.cells)
	return c
}

func (b *Board) InBounds(p Point) bool {
	return p.X >= 0 && p.Y >= 0 && p.X < b.Size && p.Y < b.Size
}

func (b *Board) At(p Point) Color { return b.cells[p.Y*b.Size+p.X] }

func (b *Board) Set(p Point, c Color) { b.cells[p.Y*b.Size+p.X] = c }

// Neighbors returns the orthogonally adjacent on-board points.
func (b *Board) Neighbors(p Point) []Point {
	out := make([]Point, 0, 4)
	for _, d := range [4]Point{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} {
		q := Point{p.X + d.X, p.Y + d.Y}
		if b.InBounds(q) {
			out = append(out, q)
		}
	}
	return out
}

// Group returns the chain containing p and its distinct liberties.
// p must hold a stone.
func (b *Board) Group(p Point) (stones, libs []Point) {
	color := b.At(p)
	if color == Empty {
		return nil, nil
	}
	seen := map[Point]bool{p: true}
	libSeen := map[Point]bool{}
	stack := []Point{p}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		stones = append(stones, cur)
		for _, n := range b.Neighbors(cur) {
			switch b.At(n) {
			case Empty:
				if !libSeen[n] {
					libSeen[n] = true
					libs = append(libs, n)
				}
			case color:
				if !seen[n] {
					seen[n] = true
					stack = append(stack, n)
				}
			}
		}
	}
	return stones, libs
}

// Liberties is the liberty count of the group at p (0 for an empty point).
func (b *Board) Liberties(p Point) int {
	_, libs := b.Group(p)
	return len(libs)
}

// Groups returns every chain on the board, keyed by a representative stone.
func (b *Board) Groups() []GroupInfo {
	var out []GroupInfo
	seen := make([]bool, len(b.cells))
	for y := 0; y < b.Size; y++ {
		for x := 0; x < b.Size; x++ {
			p := Point{x, y}
			if b.At(p) == Empty || seen[y*b.Size+x] {
				continue
			}
			stones, libs := b.Group(p)
			for _, s := range stones {
				seen[s.Y*b.Size+s.X] = true
			}
			out = append(out, GroupInfo{Color: b.At(p), Stones: stones, Liberties: libs})
		}
	}
	return out
}

type GroupInfo struct {
	Color     Color
	Stones    []Point
	Liberties []Point
}

var zobrist [MaxSize * MaxSize][3]uint64

func init() {
	r := rand.New(rand.NewSource(0x6f)) // fixed seed: hashes are stable across runs
	for i := range zobrist {
		zobrist[i][Black] = r.Uint64()
		zobrist[i][White] = r.Uint64()
	}
}

// Hash is a Zobrist hash of the stone layout.
func (b *Board) Hash() uint64 {
	var h uint64
	for y := 0; y < b.Size; y++ {
		for x := 0; x < b.Size; x++ {
			if c := b.cells[y*b.Size+x]; c != Empty {
				h ^= zobrist[y*MaxSize+x][c]
			}
		}
	}
	return h
}

// String renders the board for tests and debugging.
func (b *Board) String() string {
	s := ""
	for y := 0; y < b.Size; y++ {
		for x := 0; x < b.Size; x++ {
			switch b.At(Point{x, y}) {
			case Black:
				s += "X"
			case White:
				s += "O"
			default:
				s += "."
			}
		}
		s += "\n"
	}
	return s
}

// Parse builds a board from rows of X (black), O (white) and . (empty).
// Intended for tests and fixtures.
func Parse(rows ...string) *Board {
	b := New(len(rows))
	for y, row := range rows {
		for x, ch := range row {
			switch ch {
			case 'X':
				b.Set(Point{x, y}, Black)
			case 'O':
				b.Set(Point{x, y}, White)
			}
		}
	}
	return b
}
