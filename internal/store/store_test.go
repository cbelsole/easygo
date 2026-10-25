package store

import (
	"path/filepath"
	"testing"

	"easygo/internal/board"
	"easygo/internal/session"
)

func testGame(t *testing.T) *session.Game {
	t.Helper()
	r, _ := board.Preset("japanese")
	g, err := session.New(session.Options{
		Mode: session.ModeLive, Size: 9, Rules: r, Name: "test", OwnerID: "u1",
		Black: session.Player{Name: "Ann", Kind: "human", UserID: "u1"},
		White: session.Player{Name: "Bo", Kind: "human", UserID: "u2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestUsers(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.UpsertUser(User{ID: "g:1", Name: "Ann", Email: "a@x.org", Provider: "google"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateProfile("g:1", "Ann", "4k", "Lisbon"); err != nil {
		t.Fatal(err)
	}
	// A later sign-in refreshes the Google fields but keeps the player's rank.
	s.UpsertUser(User{ID: "g:1", Name: "Ann B.", Email: "a@x.org", Provider: "google"})
	u, err := s.User("g:1")
	if err != nil || u.Name != "Ann B." || u.Rank != "4k" || u.Location != "Lisbon" {
		t.Errorf("user %+v err %v", u, err)
	}
	if _, err := s.User("nobody"); err != ErrNotFound {
		t.Errorf("missing user: %v", err)
	}
}

func TestGamePersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "easygo.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	g := testGame(t)
	g.Play("u1", board.Point{X: 4, Y: 4}, 0)
	if err := s.SaveGame(g); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	r, err := s2.LoadGame(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v := r.View("u2"); v.MoveNumber != 1 || !v.You.CanMove {
		t.Errorf("restored view: move %d canMove %v", v.MoveNumber, v.You.CanMove)
	}
	active, err := s2.LoadActive()
	if err != nil || len(active) != 1 {
		t.Errorf("active games: %d %v", len(active), err)
	}
}

func TestListAndDelete(t *testing.T) {
	s, _ := Open(":memory:")
	defer s.Close()
	g := testGame(t)
	s.SaveGame(g)
	for _, uid := range []string{"u1", "u2"} {
		l, err := s.ListGames(uid, 10)
		if err != nil || len(l) != 1 {
			t.Errorf("%s sees %d games (%v)", uid, len(l), err)
		}
	}
	if l, _ := s.ListGames("stranger", 10); len(l) != 0 {
		t.Error("a stranger should see nothing")
	}
	if err := s.DeleteGame(g.ID, "u2"); err != ErrNotFound {
		t.Errorf("only the owner may delete: %v", err)
	}
	if err := s.DeleteGame(g.ID, "u1"); err != nil {
		t.Errorf("delete: %v", err)
	}
	if _, err := s.LoadGame(g.ID); err != ErrNotFound {
		t.Errorf("after delete: %v", err)
	}
}
