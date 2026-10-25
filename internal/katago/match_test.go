package katago

import (
	"context"
	"math/rand"
	"os"
	"testing"
	"time"

	"easygo/internal/board"
)

// playGame plays one game between two ranks and returns KataGo's judgment of
// the final position: positive means Black is ahead.
func playGame(t *testing.T, ctx context.Context, c *Client, blackRank, whiteRank string, seed int64) (lead float64, moves int) {
	t.Helper()
	const size = 19
	r, _ := board.Preset("chinese")
	pos := board.NewPosition(size)
	rng := rand.New(rand.NewSource(seed))
	var hist [][2]string
	passes := 0
	for moves = 0; moves < 400 && passes < 2; moves++ {
		color, rank := board.Black, blackRank
		if moves%2 == 1 {
			color, rank = board.White, whiteRank
		}
		q := Query{Size: size, Rules: r, Moves: append([][2]string{}, hist...)}
		cur := pos
		p, err := c.HumanMove(ctx, q, rank, func(p board.Point) bool { return cur.Legal(r, color, p) }, rng)
		if err != nil {
			t.Fatal(err)
		}
		np, err := pos.Play(r, color, p)
		if err != nil {
			t.Fatalf("move %d %v: %v", moves, p, err)
		}
		pos = np
		letter := "B"
		if color == board.White {
			letter = "W"
		}
		hist = append(hist, [2]string{letter, p.GTP(size)})
		if p.IsPass() {
			passes++
		} else {
			passes = 0
		}
	}
	q := Query{Size: size, Rules: r, Moves: hist, MaxVisits: 300}
	resp, err := c.Analyze(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	return resp.RootInfo.ScoreLead, moves
}

// A slow test (minutes): a much stronger rank should beat a much weaker one.
// Run with EASYGO_SLOW=1.
func TestStrongerRankBeatsWeakerRank(t *testing.T) {
	if os.Getenv("EASYGO_SLOW") != "1" {
		t.Skip("set EASYGO_SLOW=1 to play full games")
	}
	c, _ := startWithHuman(t)
	// Full games take minutes; the shared helper's 8-minute limit is too short.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	const weak, strong = "20k", "5d"
	strongWins := 0
	const games = 4
	for g := 0; g < games; g++ {
		blackRank, whiteRank := weak, strong
		if g%2 == 1 {
			blackRank, whiteRank = strong, weak
		}
		lead, n := playGame(t, ctx, c, blackRank, whiteRank, int64(100+g))
		strongBlack := blackRank == strong
		won := (lead > 0) == strongBlack
		if won {
			strongWins++
		}
		t.Logf("game %d: black %s vs white %s, %d moves, Black leads by %.1f, %s wins", g, blackRank, whiteRank, n, lead, map[bool]string{true: strong, false: weak}[won])
	}
	t.Logf("%s won %d of %d against %s", strong, strongWins, games, weak)
	if strongWins < 3 {
		t.Errorf("%s won only %d of %d games against %s", strong, strongWins, games, weak)
	}
}
