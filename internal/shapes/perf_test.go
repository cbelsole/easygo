package shapes

import (
	"math/rand"
	"testing"
	"time"

	"easygo/internal/board"
)

// randomGame plays legal random moves, which gives a messy, dense board
// with many groups: a harsher input than any real game.
func randomGame(t *testing.T, size, moves int, seed int64) *board.Position {
	t.Helper()
	r, _ := board.Preset("japanese")
	rng := rand.New(rand.NewSource(seed))
	pos := board.NewPosition(size)
	for i := 0; i < moves; i++ {
		for try := 0; try < 50; try++ {
			p := board.Point{X: rng.Intn(size), Y: rng.Intn(size)}
			if np, err := pos.Play(r, pos.ToPlay, p); err == nil {
				pos = np
				break
			}
		}
	}
	return pos
}

func TestDetectIsFastOnDenseBoards(t *testing.T) {
	r, _ := board.Preset("japanese")
	for _, tc := range []struct{ moves int }{{60}, {150}, {250}} {
		pos := randomGame(t, 19, tc.moves, int64(tc.moves))
		start := time.Now()
		fs := Detect(pos, r)
		d := time.Since(start)
		t.Logf("%3d moves: %d findings in %v", tc.moves, len(fs), d.Round(time.Millisecond))
		if d > 3*time.Second {
			t.Errorf("%d moves took %v; the shapes endpoint would feel hung", tc.moves, d)
		}
	}
}
