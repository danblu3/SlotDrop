package sdcard

import (
	"os"
	"path/filepath"
	"testing"

	"slotlabelmaker/internal/platforms"
)

func TestFavouritesAndRecents(t *testing.T) {
	root := t.TempDir()
	fav := filepath.Join(root, "Config", "favorites.txt")
	rec := filepath.Join(root, "Config", "recents.txt")
	os.MkdirAll(filepath.Dir(fav), 0o755)
	os.WriteFile(fav, []byte("gb/Tetris\nGBA/Pokemon - Lazarus\n"), 0o644)
	os.WriteFile(rec, []byte("gba/Pokemon - Lazarus\ngbc/Wario Land 3\n"), 0o644)

	if f := Favourites(root); !f["GBA/Pokemon - Lazarus"] || !f["GB/Tetris"] || len(f) != 2 {
		t.Fatalf("read: %v", f)
	}
	// Adding twice adds one line, at the end; others stay as written.
	SetFavourite(root, platforms.GBA, "Golden Sun", true)
	SetFavourite(root, platforms.GBA, "Golden Sun", true)
	if b, _ := os.ReadFile(fav); string(b) != "gb/Tetris\nGBA/Pokemon - Lazarus\ngba/Golden Sun\n" {
		t.Fatalf("add: %q", b)
	}
	// Same stem on another platform is a different game.
	SetFavourite(root, platforms.GBC, "Tetris", false)
	if b, _ := os.ReadFile(fav); string(b) != "gb/Tetris\nGBA/Pokemon - Lazarus\ngba/Golden Sun\n" {
		t.Fatalf("other platform touched: %q", b)
	}
	SetFavourite(root, platforms.GB, "Tetris", false)
	if b, _ := os.ReadFile(fav); string(b) != "GBA/Pokemon - Lazarus\ngba/Golden Sun\n" {
		t.Fatalf("remove: %q", b)
	}
	// Removing a game clears it from both lists.
	if err := ForgetGame(root, platforms.GBA, "Pokemon - Lazarus"); err != nil {
		t.Fatal(err)
	}
	b1, _ := os.ReadFile(fav)
	b2, _ := os.ReadFile(rec)
	if string(b1) != "gba/Golden Sun\n" || string(b2) != "gbc/Wario Land 3\n" {
		t.Fatalf("forget: %q %q", b1, b2)
	}
	// No files: nothing is created by forgetting.
	empty := t.TempDir()
	ForgetGame(empty, platforms.GBA, "x")
	if _, err := os.Stat(filepath.Join(empty, "Config")); err == nil {
		t.Fatal("forgetting shouldn't create Config")
	}
}
