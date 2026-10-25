package board

import (
	"errors"
	"fmt"
)

var (
	ErrOccupied  = errors.New("point is occupied")
	ErrOffBoard  = errors.New("point is off the board")
	ErrSuicide   = errors.New("suicide is not allowed")
	ErrKo        = errors.New("move repeats an earlier position (ko)")
	ErrWrongTurn = errors.New("not this color's turn")
)

// Position is the full game state after some move. Positions form a chain
// through prev, which the ko rules walk. A Position is never mutated once
// returned, so tree nodes can cache and share them.
type Position struct {
	Board    *Board
	ToPlay   Color
	Captures [3]int // Captures[Black] = white stones black has captured
	MoveNum  int
	Passes   int // consecutive passes ending at this position
	LastMove Point
	LastBy   Color
	Hash     uint64

	prev *Position
}

// NewPosition starts a game with black to play on an empty board.
func NewPosition(size int) *Position {
	b := New(size)
	return &Position{Board: b, ToPlay: Black, LastMove: Pass, Hash: b.Hash()}
}

// FromBoard starts from an arbitrary layout (setup stones).
func FromBoard(b *Board, toPlay Color) *Position {
	return &Position{Board: b, ToPlay: toPlay, LastMove: Pass, Hash: b.Hash()}
}

// Over reports whether two consecutive passes have ended the game.
func (p *Position) Over() bool { return p.Passes >= 2 }

// WithSetup returns a copy with stones forced onto points (SGF AB/AW/AE).
// Setup does not capture and does not extend the ko history.
func (p *Position) WithSetup(stones map[Point]Color, toPlay Color) *Position {
	b := p.Board.Clone()
	for pt, c := range stones {
		b.Set(pt, c)
	}
	np := *p
	np.Board = b
	np.Hash = b.Hash()
	if toPlay != Empty {
		np.ToPlay = toPlay
	}
	return &np
}

// Play applies color's move at pt (use Pass for a pass) under the rules.
func (p *Position) Play(r Rules, color Color, pt Point) (*Position, error) {
	if color != p.ToPlay {
		return nil, ErrWrongTurn
	}
	if pt.IsPass() {
		np := &Position{
			Board: p.Board, ToPlay: color.Opp(), Captures: p.Captures,
			MoveNum: p.MoveNum + 1, Passes: p.Passes + 1,
			LastMove: Pass, LastBy: color, Hash: p.Hash, prev: p,
		}
		return np, nil
	}
	if !p.Board.InBounds(pt) {
		return nil, ErrOffBoard
	}
	if p.Board.At(pt) != Empty {
		return nil, ErrOccupied
	}

	b := p.Board.Clone()
	b.Set(pt, color)
	caps := p.Captures
	opp := color.Opp()

	for _, n := range b.Neighbors(pt) {
		if b.At(n) != opp {
			continue
		}
		stones, libs := b.Group(n)
		if len(libs) == 0 {
			for _, s := range stones {
				b.Set(s, Empty)
			}
			caps[color] += len(stones)
		}
	}

	own, ownLibs := b.Group(pt)
	if len(ownLibs) == 0 {
		// Nothing was captured, so this is suicide.
		if len(own) == 1 || !r.MultiStoneSuicide {
			return nil, ErrSuicide
		}
		for _, s := range own {
			b.Set(s, Empty)
		}
		caps[opp] += len(own)
	}

	np := &Position{
		Board: b, ToPlay: opp, Captures: caps, MoveNum: p.MoveNum + 1,
		LastMove: pt, LastBy: color, Hash: b.Hash(), prev: p,
	}
	if err := np.checkKo(r); err != nil {
		return nil, err
	}
	return np, nil
}

// checkKo applies the configured repetition rule to a freshly built position.
func (np *Position) checkKo(r Rules) error {
	switch r.Ko {
	case KoSimple:
		// Forbid returning to the position before the opponent's last move.
		if np.prev != nil && np.prev.prev != nil && np.prev.prev.Hash == np.Hash && np.prev.prev.ToPlay == np.ToPlay {
			return ErrKo
		}
	case KoPositional:
		for q := np.prev; q != nil; q = q.prev {
			if q.Hash == np.Hash {
				return ErrKo
			}
		}
	case KoSituational:
		for q := np.prev; q != nil; q = q.prev {
			if q.Hash == np.Hash && q.ToPlay == np.ToPlay {
				return ErrKo
			}
		}
	}
	return nil
}

// Legal reports whether color may play pt now.
func (p *Position) Legal(r Rules, color Color, pt Point) bool {
	_, err := p.Play(r, color, pt)
	return err == nil
}

// Handicap places n handicap stones for black on a standard board and
// leaves white to play. Supports 2-9 stones on 9, 13 and 19 lines.
func Handicap(size, n int) (*Position, error) {
	pts, err := handicapPoints(size, n)
	if err != nil {
		return nil, err
	}
	b := New(size)
	for _, pt := range pts {
		b.Set(pt, Black)
	}
	return FromBoard(b, White), nil
}

func handicapPoints(size, n int) ([]Point, error) {
	if n < 2 || n > 9 {
		return nil, fmt.Errorf("handicap must be 2-9, got %d", n)
	}
	var edge int
	switch size {
	case 9:
		edge = 2
	case 13, 19:
		edge = 3
	default:
		return nil, fmt.Errorf("no handicap layout for %dx%d", size, size)
	}
	lo, hi, mid := edge, size-1-edge, size/2
	corners := []Point{{hi, lo}, {lo, hi}, {hi, hi}, {lo, lo}} // 1:top-right 2:bottom-left 3:bottom-right 4:top-left
	sides := []Point{{lo, mid}, {hi, mid}, {mid, lo}, {mid, hi}}
	center := Point{mid, mid}

	switch n {
	case 2, 3, 4:
		return corners[:n], nil
	case 5:
		return append(append([]Point{}, corners...), center), nil
	case 6:
		return append(append([]Point{}, corners...), sides[0], sides[1]), nil
	case 7:
		return append(append(append([]Point{}, corners...), sides[0], sides[1]), center), nil
	case 8:
		return append(append([]Point{}, corners...), sides...), nil
	default:
		return append(append(append([]Point{}, corners...), sides...), center), nil
	}
}
