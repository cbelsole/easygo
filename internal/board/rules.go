package board

import "fmt"

// KoRule selects how repetition is forbidden.
type KoRule string

const (
	KoSimple      KoRule = "SIMPLE"      // only the immediate single-stone recapture
	KoPositional  KoRule = "POSITIONAL"  // no earlier whole-board position, either player
	KoSituational KoRule = "SITUATIONAL" // no earlier position with the same player to move
)

// ScoringRule is area (stones + territory) or territory (territory + prisoners).
type ScoringRule string

const (
	ScoreArea      ScoringRule = "AREA"
	ScoreTerritory ScoringRule = "TERRITORY"
)

// TaxRule mirrors KataGo's names. Only NONE and SEKI are scored here: under
// territory scoring a seki's shared liberties touch both colors, so the
// flood fill leaves them uncounted, which is what SEKI means. ALL is not
// implemented; scoring with it falls back to SEKI behaviour.
type TaxRule string

const (
	TaxNone TaxRule = "NONE"
	TaxSeki TaxRule = "SEKI"
	TaxAll  TaxRule = "ALL"
)

// Rules is deliberately the same shape as KataGo's rules object so the board
// enforces exactly what KataGo analyzes.
type Rules struct {
	Ko                 KoRule      `json:"ko"`
	Scoring            ScoringRule `json:"scoring"`
	Tax                TaxRule     `json:"tax"`
	MultiStoneSuicide  bool        `json:"suicide"`
	HasButton          bool        `json:"hasButton"`
	WhiteHandicapBonus string      `json:"whiteHandicapBonus"` // "0", "N-1" or "N"
	Komi               float64     `json:"komi"`
	Preset             string      `json:"preset,omitempty"` // KataGo preset name, informational
}

// Preset names accepted by KataGo's `rules` string.
var presets = map[string]Rules{
	"tromp-taylor":  {KoPositional, ScoreArea, TaxNone, true, false, "0", 7.5, "tromp-taylor"},
	"chinese":       {KoSimple, ScoreArea, TaxNone, false, false, "N", 7, "chinese"},
	"japanese":      {KoSimple, ScoreTerritory, TaxSeki, false, false, "0", 6.5, "japanese"},
	"korean":        {KoSimple, ScoreTerritory, TaxSeki, false, false, "0", 6.5, "korean"},
	"aga":           {KoSituational, ScoreArea, TaxNone, false, false, "N-1", 7.5, "aga"},
	"new-zealand":   {KoSituational, ScoreArea, TaxNone, true, false, "0", 7, "new-zealand"},
	"stone-scoring": {KoSimple, ScoreArea, TaxAll, false, false, "0", 7.5, "stone-scoring"},
}

// PresetNames lists presets in display order.
func PresetNames() []string {
	return []string{"japanese", "chinese", "korean", "aga", "new-zealand", "tromp-taylor", "stone-scoring"}
}

// Preset returns the named rule set.
func Preset(name string) (Rules, error) {
	r, ok := presets[name]
	if !ok {
		return Rules{}, fmt.Errorf("unknown rules preset %q", name)
	}
	return r, nil
}

// Validate reports a rules combination the engine cannot honor.
func (r Rules) Validate() error {
	switch r.Ko {
	case KoSimple, KoPositional, KoSituational:
	default:
		return fmt.Errorf("bad ko rule %q", r.Ko)
	}
	switch r.Scoring {
	case ScoreArea, ScoreTerritory:
	default:
		return fmt.Errorf("bad scoring rule %q", r.Scoring)
	}
	switch r.Tax {
	case TaxNone, TaxSeki, TaxAll:
	default:
		return fmt.Errorf("bad tax rule %q", r.Tax)
	}
	switch r.WhiteHandicapBonus {
	case "0", "N-1", "N":
	default:
		return fmt.Errorf("bad whiteHandicapBonus %q", r.WhiteHandicapBonus)
	}
	return nil
}
