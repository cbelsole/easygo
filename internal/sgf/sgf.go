// Package sgf reads and writes the SGF game record format (FF[4]) to and
// from tree.Tree. The parser is tolerant: unknown properties are kept, and
// a bad move does not abort the file.
package sgf

import (
	"fmt"
	"strconv"
	"strings"

	"easygo/internal/board"
	"easygo/internal/tree"
)

// rawNode is one ';' node before interpretation.
type rawNode struct {
	props []rawProp
}

type rawProp struct {
	ident  string
	values []string
}

// rawTree is a sequence of nodes followed by variations.
type rawTree struct {
	nodes    []rawNode
	children []*rawTree
}

type parser struct {
	s   string
	pos int
}

func (p *parser) skipWS() {
	for p.pos < len(p.s) && strings.ContainsRune(" \t\r\n", rune(p.s[p.pos])) {
		p.pos++
	}
}

// parseCollection parses every game tree in the input.
func parseCollection(s string) ([]*rawTree, error) {
	p := &parser{s: strings.TrimPrefix(s, "\xef\xbb\xbf")} // UTF-8 byte order mark
	var out []*rawTree
	for {
		p.skipWS()
		if p.pos >= len(p.s) {
			break
		}
		if p.s[p.pos] != '(' {
			p.pos++ // skip stray text between games
			continue
		}
		t, err := p.parseTree()
		if err != nil {
			return out, err
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("sgf: no game tree found")
	}
	return out, nil
}

func (p *parser) parseTree() (*rawTree, error) {
	p.pos++ // '('
	t := &rawTree{}
	for {
		p.skipWS()
		if p.pos >= len(p.s) {
			return t, nil // tolerate a missing closing paren
		}
		switch p.s[p.pos] {
		case ';':
			p.pos++
			n, err := p.parseNode()
			if err != nil {
				return nil, err
			}
			t.nodes = append(t.nodes, n)
		case '(':
			c, err := p.parseTree()
			if err != nil {
				return nil, err
			}
			t.children = append(t.children, c)
		case ')':
			p.pos++
			return t, nil
		default:
			p.pos++
		}
	}
}

func (p *parser) parseNode() (rawNode, error) {
	var n rawNode
	for {
		p.skipWS()
		if p.pos >= len(p.s) {
			return n, nil
		}
		c := p.s[p.pos]
		if c == ';' || c == '(' || c == ')' {
			return n, nil
		}
		start := p.pos
		for p.pos < len(p.s) && p.s[p.pos] >= 'A' && p.s[p.pos] <= 'Z' {
			p.pos++
		}
		ident := p.s[start:p.pos]
		if ident == "" {
			// Lowercase letters before the value are legal in old files; skip.
			p.pos++
			continue
		}
		prop := rawProp{ident: ident}
		for {
			p.skipWS()
			if p.pos >= len(p.s) || p.s[p.pos] != '[' {
				break
			}
			v, err := p.parseValue()
			if err != nil {
				return n, err
			}
			prop.values = append(prop.values, v)
		}
		n.props = append(n.props, prop)
	}
}

func (p *parser) parseValue() (string, error) {
	p.pos++ // '['
	var sb strings.Builder
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		switch c {
		case '\\':
			p.pos++
			if p.pos < len(p.s) {
				next := p.s[p.pos]
				// A backslash before a newline is a soft line break.
				if next == '\n' {
					p.pos++
					continue
				}
				if next == '\r' {
					p.pos++
					if p.pos < len(p.s) && p.s[p.pos] == '\n' {
						p.pos++
					}
					continue
				}
				sb.WriteByte(next)
				p.pos++
			}
		case ']':
			p.pos++
			return sb.String(), nil
		default:
			sb.WriteByte(c)
			p.pos++
		}
	}
	return "", fmt.Errorf("sgf: unterminated property value")
}

// Parse reads the first game in s.
func Parse(s string) (*tree.Tree, error) {
	games, err := ParseAll(s)
	if err != nil {
		return nil, err
	}
	return games[0], nil
}

// ParseAll reads every game in s.
func ParseAll(s string) ([]*tree.Tree, error) {
	raws, err := parseCollection(s)
	if err != nil && len(raws) == 0 {
		return nil, err
	}
	var out []*tree.Tree
	for _, r := range raws {
		t, err := build(r)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func first(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func build(r *rawTree) (*tree.Tree, error) {
	size := 19
	var rootNode rawNode
	if len(r.nodes) > 0 {
		rootNode = r.nodes[0]
	}
	for _, pr := range rootNode.props {
		if pr.ident == "SZ" {
			// SZ[n] or SZ[cols:rows]; only square boards are supported.
			v := first(pr.values)
			if i := strings.IndexByte(v, ':'); i >= 0 {
				if v[:i] != v[i+1:] {
					return nil, fmt.Errorf("sgf: non-square board %q is not supported", v)
				}
				v = v[:i]
			}
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || n < 2 || n > board.MaxSize {
				return nil, fmt.Errorf("sgf: unsupported board size %q", v)
			}
			size = n
		}
	}
	rules, _ := board.Preset("japanese")
	t := tree.New(size, rules)

	// The ruleset goes first so an explicit KM below overrides its komi.
	for _, pr := range rootNode.props {
		if pr.ident == "RU" {
			if r, ok := ruleByName(first(pr.values)); ok {
				t.Rules = r
			}
		}
	}

	// Handicap and komi are read first because they affect position setup.
	for _, pr := range rootNode.props {
		switch pr.ident {
		case "HA":
			t.Info.Handicap, _ = strconv.Atoi(first(pr.values))
		case "KM":
			if k, err := strconv.ParseFloat(first(pr.values), 64); err == nil {
				t.Rules.Komi = k
			}
		}
	}

	var attach func(parent *tree.Node, rt *rawTree, isRoot bool) error
	attach = func(parent *tree.Node, rt *rawTree, isRoot bool) error {
		cur := parent
		for i, rn := range rt.nodes {
			var n *tree.Node
			if isRoot && i == 0 {
				n = t.Root
			} else {
				n = t.AddChild(cur)
			}
			applyProps(t, n, rn, isRoot && i == 0)
			cur = n
		}
		for _, c := range rt.children {
			if err := attach(cur, c, false); err != nil {
				return err
			}
		}
		return nil
	}
	if err := attach(nil, r, true); err != nil {
		return nil, err
	}
	return t, nil
}

func applyProps(t *tree.Tree, n *tree.Node, rn rawNode, isRoot bool) {
	size := t.Size
	for _, pr := range rn.props {
		switch pr.ident {
		case "B", "W":
			n.HasMove = true
			n.Color = board.Black
			if pr.ident == "W" {
				n.Color = board.White
			}
			n.Move = decodePoint(first(pr.values), size)
		case "AB", "AW", "AE":
			if n.Setup == nil {
				n.Setup = map[board.Point]board.Color{}
			}
			c := map[string]board.Color{"AB": board.Black, "AW": board.White, "AE": board.Empty}[pr.ident]
			for _, v := range pr.values {
				for _, pt := range decodePointList(v, size) {
					n.Setup[pt] = c
				}
			}
		case "PL":
			if first(pr.values) == "W" {
				n.ToPlay = board.White
			} else {
				n.ToPlay = board.Black
			}
		case "C":
			n.Comment = first(pr.values)
		case "N":
			n.Name = first(pr.values)
		case "TR", "SQ", "CR", "MA":
			kind := map[string]string{"TR": "triangle", "SQ": "square", "CR": "circle", "MA": "cross"}[pr.ident]
			for _, v := range pr.values {
				for _, pt := range decodePointList(v, size) {
					n.Marks = append(n.Marks, tree.Mark{Pt: pt, Kind: kind})
				}
			}
		case "LB":
			for _, v := range pr.values {
				if i := strings.IndexByte(v, ':'); i > 0 {
					n.Marks = append(n.Marks, tree.Mark{Pt: decodePoint(v[:i], size), Kind: "label", Text: v[i+1:]})
				}
			}
		case "TE":
			n.Judgment = "correct"
		case "BM":
			n.Judgment = "mistake"
		case "DO":
			n.Judgment = "playable"
		default:
			if isRoot {
				switch pr.ident {
				case "PB":
					t.Info.BlackName = first(pr.values)
				case "PW":
					t.Info.WhiteName = first(pr.values)
				case "BR":
					t.Info.BlackRank = first(pr.values)
				case "WR":
					t.Info.WhiteRank = first(pr.values)
				case "RE":
					t.Info.Result = first(pr.values)
				case "DT":
					t.Info.Date = first(pr.values)
				case "PC":
					t.Info.Place = first(pr.values)
				case "GN":
					t.Info.GameName = first(pr.values)
				case "SZ", "KM", "HA", "GM", "FF", "CA", "AP", "ST", "RU":
					// consumed or irrelevant
				default:
					keepProp(n, pr)
				}
				continue
			}
			keepProp(n, pr)
		}
	}
}

func keepProp(n *tree.Node, pr rawProp) {
	if n.Props == nil {
		n.Props = map[string][]string{}
	}
	n.Props[pr.ident] = append(n.Props[pr.ident], pr.values...)
}

// decodePoint reads "pd"-style coordinates. Empty (or "tt" on boards of
// 19 or less) is a pass.
func decodePoint(s string, size int) board.Point {
	if len(s) < 2 {
		return board.Pass
	}
	x, y := int(s[0]-'a'), int(s[1]-'a')
	if size <= 19 && s == "tt" {
		return board.Pass
	}
	if x < 0 || y < 0 || x >= size || y >= size {
		return board.Pass
	}
	return board.Point{X: x, Y: y}
}

// decodePointList expands a point or a "aa:cc" compressed rectangle.
func decodePointList(s string, size int) []board.Point {
	if i := strings.IndexByte(s, ':'); i >= 0 {
		a, b := decodePoint(s[:i], size), decodePoint(s[i+1:], size)
		if a.IsPass() || b.IsPass() {
			return nil
		}
		var out []board.Point
		for y := min(a.Y, b.Y); y <= max(a.Y, b.Y); y++ {
			for x := min(a.X, b.X); x <= max(a.X, b.X); x++ {
				out = append(out, board.Point{X: x, Y: y})
			}
		}
		return out
	}
	p := decodePoint(s, size)
	if p.IsPass() {
		return nil
	}
	return []board.Point{p}
}

func encodePoint(p board.Point) string {
	if p.IsPass() {
		return ""
	}
	return string([]byte{byte('a' + p.X), byte('a' + p.Y)})
}

// ruleByName maps an SGF RU value to a preset. Unknown names keep the default.
func ruleByName(s string) (board.Rules, bool) {
	names := map[string]string{
		"japanese": "japanese", "jp": "japanese", "korean": "korean",
		"chinese": "chinese", "cn": "chinese", "aga": "aga", "nz": "new-zealand",
		"new zealand": "new-zealand", "new-zealand": "new-zealand",
		"tromp-taylor": "tromp-taylor", "tt": "tromp-taylor", "stone-scoring": "stone-scoring",
	}
	key := strings.ToLower(strings.TrimSpace(s))
	if n, ok := names[key]; ok {
		r, err := board.Preset(n)
		return r, err == nil
	}
	return board.Rules{}, false
}
