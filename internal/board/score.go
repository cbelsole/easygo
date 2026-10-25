package board

// ScoreResult is a finished count. Margin is positive when Black leads.
type ScoreResult struct {
	Black, White float64 // totals, White includes komi
	BlackTerr    int
	WhiteTerr    int
	Neutral      int
	Margin       float64
	// Territory[y*size+x] is Black, White or Empty (neutral / stone-held).
	Territory []Color
}

// ScoreOpts carries what the rules need beyond the board itself.
type ScoreOpts struct {
	Dead     map[Point]bool // stones to treat as dead (removed and counted as prisoners)
	Handicap int            // black handicap stones, for WhiteHandicapBonus
}

// Score counts a position under the rules. Dead stones are removed first;
// they count as prisoners under territory scoring and are simply gone
// under area scoring.
//
// Regions bordered by both colors are neutral, which also keeps seki
// liberties out of the count (the SEKI tax rule). HasButton is not scored.
func Score(pos *Position, r Rules, opts ScoreOpts) ScoreResult {
	b := pos.Board.Clone()
	deadBy := [3]int{} // dead stones of color c
	for pt := range opts.Dead {
		if c := b.At(pt); c != Empty {
			deadBy[c]++
			b.Set(pt, Empty)
		}
	}

	size := b.Size
	terr := make([]Color, size*size)
	seen := make([]bool, size*size)
	var bt, wt, neutral int
	stones := [3]int{}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			p := Point{x, y}
			switch c := b.At(p); c {
			case Black, White:
				stones[c]++
				continue
			}
			if seen[y*size+x] {
				continue
			}
			// Flood this empty region and note which colors touch it.
			var region []Point
			touches := [3]bool{}
			stack := []Point{p}
			seen[y*size+x] = true
			for len(stack) > 0 {
				cur := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				region = append(region, cur)
				for _, n := range b.Neighbors(cur) {
					switch nc := b.At(n); nc {
					case Empty:
						if !seen[n.Y*size+n.X] {
							seen[n.Y*size+n.X] = true
							stack = append(stack, n)
						}
					default:
						touches[nc] = true
					}
				}
			}
			owner := Empty
			switch {
			case touches[Black] && !touches[White]:
				owner = Black
				bt += len(region)
			case touches[White] && !touches[Black]:
				owner = White
				wt += len(region)
			default:
				neutral += len(region)
			}
			for _, rp := range region {
				terr[rp.Y*size+rp.X] = owner
			}
		}
	}

	komi := r.Komi
	if opts.Handicap > 0 {
		switch r.WhiteHandicapBonus {
		case "N":
			komi += float64(opts.Handicap)
		case "N-1":
			komi += float64(opts.Handicap - 1)
		}
	}

	var black, white float64
	if r.Scoring == ScoreArea {
		black = float64(stones[Black] + bt)
		white = float64(stones[White]+wt) + komi
	} else {
		// Prisoners: stones captured in play plus dead stones left on the board.
		black = float64(bt + pos.Captures[Black] + deadBy[White])
		white = float64(wt+pos.Captures[White]+deadBy[Black]) + komi
	}
	return ScoreResult{
		Black: black, White: white, BlackTerr: bt, WhiteTerr: wt,
		Neutral: neutral, Margin: black - white, Territory: terr,
	}
}
