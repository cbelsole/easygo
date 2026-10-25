package sgf

import (
	"fmt"
	"sort"
	"strings"

	"easygo/internal/board"
	"easygo/internal/tree"
)

func escape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `]`, `\]`)
}

// Write serializes a tree as SGF (FF[4], UTF-8).
func Write(t *tree.Tree) string {
	var sb strings.Builder
	sb.WriteString("(")
	writeNode(&sb, t, t.Root, true)
	sb.WriteString(")\n")
	return sb.String()
}

func writeNode(sb *strings.Builder, t *tree.Tree, n *tree.Node, isRoot bool) {
	sb.WriteString(";")
	if isRoot {
		fmt.Fprintf(sb, "GM[1]FF[4]CA[UTF-8]AP[easygo]SZ[%d]", t.Size)
		fmt.Fprintf(sb, "KM[%g]", t.Rules.Komi)
		if t.Info.Handicap > 1 {
			fmt.Fprintf(sb, "HA[%d]", t.Info.Handicap)
		}
		for _, kv := range [][2]string{
			{"PB", t.Info.BlackName}, {"PW", t.Info.WhiteName},
			{"BR", t.Info.BlackRank}, {"WR", t.Info.WhiteRank},
			{"RE", t.Info.Result}, {"DT", t.Info.Date}, {"PC", t.Info.Place},
			{"GN", t.Info.GameName},
		} {
			if kv[1] != "" {
				fmt.Fprintf(sb, "%s[%s]", kv[0], escape(kv[1]))
			}
		}
	}
	if len(n.Setup) > 0 {
		for _, g := range []struct {
			id string
			c  board.Color
		}{{"AB", board.Black}, {"AW", board.White}, {"AE", board.Empty}} {
			var pts []board.Point
			for p, c := range n.Setup {
				if c == g.c {
					pts = append(pts, p)
				}
			}
			sort.Slice(pts, func(i, j int) bool {
				if pts[i].Y != pts[j].Y {
					return pts[i].Y < pts[j].Y
				}
				return pts[i].X < pts[j].X
			})
			if len(pts) > 0 {
				sb.WriteString(g.id)
				for _, p := range pts {
					fmt.Fprintf(sb, "[%s]", encodePoint(p))
				}
			}
		}
	}
	if n.ToPlay != board.Empty {
		fmt.Fprintf(sb, "PL[%s]", n.ToPlay)
	}
	if n.HasMove {
		fmt.Fprintf(sb, "%s[%s]", n.Color, encodePoint(n.Move))
	}
	if n.Name != "" {
		fmt.Fprintf(sb, "N[%s]", escape(n.Name))
	}
	if n.Comment != "" {
		fmt.Fprintf(sb, "C[%s]", escape(n.Comment))
	}
	switch n.Judgment {
	case "correct":
		sb.WriteString("TE[1]")
	case "mistake":
		sb.WriteString("BM[1]")
	case "playable":
		sb.WriteString("DO[]")
	}
	writeMarks(sb, n)
	keys := make([]string, 0, len(n.Props))
	for k := range n.Props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sb.WriteString(k)
		for _, v := range n.Props[k] {
			fmt.Fprintf(sb, "[%s]", escape(v))
		}
	}

	switch len(n.Children) {
	case 0:
	case 1:
		writeNode(sb, t, n.Children[0], false)
	default:
		for _, c := range n.Children {
			sb.WriteString("(")
			writeNode(sb, t, c, false)
			sb.WriteString(")")
		}
	}
}

func writeMarks(sb *strings.Builder, n *tree.Node) {
	byKind := map[string][]tree.Mark{}
	for _, m := range n.Marks {
		byKind[m.Kind] = append(byKind[m.Kind], m)
	}
	for kind, id := range map[string]string{"triangle": "TR", "square": "SQ", "circle": "CR", "cross": "MA"} {
		if ms := byKind[kind]; len(ms) > 0 {
			sb.WriteString(id)
			for _, m := range ms {
				fmt.Fprintf(sb, "[%s]", encodePoint(m.Pt))
			}
		}
	}
	if ls := byKind["label"]; len(ls) > 0 {
		sb.WriteString("LB")
		for _, m := range ls {
			fmt.Fprintf(sb, "[%s:%s]", encodePoint(m.Pt), escape(m.Text))
		}
	}
}
