// Package tree is the branching move tree: one Node per move or setup, with
// comments and annotations. Positions are computed lazily and cached.
package tree

import (
	"fmt"

	"easygo/internal/board"
)

// Mark is an annotation drawn on a point.
type Mark struct {
	Pt   board.Point `json:"pt"`
	Kind string      `json:"kind"` // label, triangle, square, circle, cross
	Text string      `json:"text,omitempty"`
}

// Node is one step in the tree. The root carries setup and game info.
type Node struct {
	ID       int
	Parent   *Node
	Children []*Node

	HasMove bool
	Color   board.Color
	Move    board.Point // board.Pass for a pass

	Setup    map[board.Point]board.Color // AB/AW/AE; Empty clears a point
	ToPlay   board.Color                 // SGF PL, Empty if unset
	Comment  string
	Name     string
	Marks    []Mark
	Judgment string // joseki trainer: correct | playable | mistake | ""

	// Props keeps SGF properties the app does not model, so import/export
	// round-trips them.
	Props map[string][]string

	pos    *board.Position
	posErr error
}

// Info is the game-level metadata from the SGF root.
type Info struct {
	BlackName, WhiteName string
	BlackRank, WhiteRank string
	Result               string
	Date                 string
	Place                string
	GameName             string
	Handicap             int
}

type Tree struct {
	Root  *Node
	Size  int
	Rules board.Rules
	Info  Info

	nextID int
}

// New returns a tree with an empty root.
func New(size int, rules board.Rules) *Tree {
	t := &Tree{Size: size, Rules: rules}
	t.Root = t.newNode(nil)
	return t
}

func (t *Tree) newNode(parent *Node) *Node {
	t.nextID++
	return &Node{ID: t.nextID, Parent: parent}
}

// Find returns the node with the given ID, or nil.
func (t *Tree) Find(id int) *Node {
	var found *Node
	t.Walk(func(n *Node) bool {
		if n.ID == id {
			found = n
			return false
		}
		return true
	})
	return found
}

// Walk visits nodes depth-first, main line first. Return false to stop.
func (t *Tree) Walk(fn func(*Node) bool) {
	var rec func(*Node) bool
	rec = func(n *Node) bool {
		if !fn(n) {
			return false
		}
		for _, c := range n.Children {
			if !rec(c) {
				return false
			}
		}
		return true
	}
	rec(t.Root)
}

// AddMove appends a move under parent. If an identical move already exists
// as a child, that child is returned instead of creating a duplicate branch.
func (t *Tree) AddMove(parent *Node, color board.Color, pt board.Point) (*Node, error) {
	for _, c := range parent.Children {
		if c.HasMove && c.Color == color && c.Move == pt {
			return c, nil
		}
	}
	pp, err := t.Position(parent)
	if err != nil {
		return nil, err
	}
	if _, err := pp.WithSetup(nil, color).Play(t.Rules, color, pt); err != nil {
		return nil, err
	}
	n := t.newNode(parent)
	n.HasMove, n.Color, n.Move = true, color, pt
	parent.Children = append(parent.Children, n)
	return n, nil
}

// AddChild appends a node without validating it. The SGF importer uses
// this so one bad move does not discard the whole file.
func (t *Tree) AddChild(parent *Node) *Node {
	n := t.newNode(parent)
	parent.Children = append(parent.Children, n)
	return n
}

// Delete removes a node and its subtree. The root cannot be deleted.
func (t *Tree) Delete(n *Node) error {
	if n.Parent == nil {
		return fmt.Errorf("cannot delete the root")
	}
	sib := n.Parent.Children
	for i, c := range sib {
		if c == n {
			n.Parent.Children = append(sib[:i:i], sib[i+1:]...)
			break
		}
	}
	n.Parent = nil
	return nil
}

// Promote moves a branch earlier among its siblings (toward the main line).
func (t *Tree) Promote(n *Node) {
	if n.Parent == nil {
		return
	}
	sib := n.Parent.Children
	for i, c := range sib {
		if c == n && i > 0 {
			sib[i-1], sib[i] = sib[i], sib[i-1]
			return
		}
	}
}

// Path returns the nodes from the root down to n.
func (t *Tree) Path(n *Node) []*Node {
	var rev []*Node
	for cur := n; cur != nil; cur = cur.Parent {
		rev = append(rev, cur)
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

// MainLine follows first children from the root.
func (t *Tree) MainLine() []*Node {
	line := []*Node{t.Root}
	for cur := t.Root; len(cur.Children) > 0; {
		cur = cur.Children[0]
		line = append(line, cur)
	}
	return line
}

// MoveNumber counts moves from the root to n.
func (t *Tree) MoveNumber(n *Node) int {
	c := 0
	for _, p := range t.Path(n) {
		if p.HasMove {
			c++
		}
	}
	return c
}

// Position returns the game state after n. Same-color consecutive moves,
// which SGF permits, are accepted.
func (t *Tree) Position(n *Node) (*board.Position, error) {
	if n.pos != nil || n.posErr != nil {
		return n.pos, n.posErr
	}
	var pos *board.Position
	if n.Parent == nil {
		pos = board.NewPosition(t.Size)
		if t.Info.Handicap > 1 {
			pos = pos.WithSetup(nil, board.White)
		}
	} else {
		pp, err := t.Position(n.Parent)
		if err != nil {
			n.posErr = err
			return nil, err
		}
		pos = pp
	}
	if len(n.Setup) > 0 {
		pos = pos.WithSetup(n.Setup, n.ToPlay)
	} else if n.ToPlay != board.Empty {
		pos = pos.WithSetup(nil, n.ToPlay)
	}
	if n.HasMove {
		np, err := pos.WithSetup(nil, n.Color).Play(t.Rules, n.Color, n.Move)
		if err != nil {
			n.posErr = fmt.Errorf("move %d (%s %s): %w", t.MoveNumber(n), n.Color, n.Move.GTP(t.Size), err)
			return nil, n.posErr
		}
		pos = np
	}
	n.pos = pos
	return pos, nil
}
