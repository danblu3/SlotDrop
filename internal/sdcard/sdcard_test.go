package sdcard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellFiles(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "Labels"), 0o755)
	os.MkdirAll(filepath.Join(root, "Config"), 0o755)
	// An old card: the Labels file still has its .ini name.
	os.WriteFile(filepath.Join(root, "Labels", "cart_shell.ini"), []byte("Tetris = auto c0282c solid\r\n"), 0o644)
	os.WriteFile(filepath.Join(root, "Config", "cart_shell.txt"), []byte("Tetris = auto 111111 solid\nZelda = notched e8b72e clear\n"), 0o644)

	s := ReadShells(root)
	if s["Tetris"].Colour != "c0282c" || s["Zelda"].Colour != "e8b72e" {
		t.Fatalf("Labels should override Config: %v", s)
	}
	if err := SetShells(root, map[string]Shell{"Golden Sun": {Colour: "3a7bd5", Finish: "glitter"}}); err != nil {
		t.Fatal(err)
	}
	txt, err := os.ReadFile(filepath.Join(root, "Labels", "cart_shell.txt"))
	if err != nil {
		t.Fatal("should write cart_shell.txt:", err)
	}
	if !strings.Contains(string(txt), "Tetris = auto c0282c solid") || !strings.Contains(string(txt), "Golden Sun = auto 3a7bd5 glitter") {
		t.Fatalf("old lines should carry over: %q", txt)
	}
	if _, err := os.Stat(filepath.Join(root, "Labels", "cart_shell.ini")); err == nil {
		t.Fatal("the old .ini should be moved aside, so Slot can't migrate it over the .txt")
	}
	if bak, err := os.ReadFile(filepath.Join(root, "Labels", "cart_shell.ini.bak")); err != nil || !strings.Contains(string(bak), "Tetris") {
		t.Fatalf("the old .ini should be kept as .bak: %q %v", bak, err)
	}
	if err := RemoveShell(root, "Tetris"); err != nil {
		t.Fatal(err)
	}
	if s := ReadShells(root); s["Tetris"].Colour != "111111" {
		t.Fatalf("after removing, Config's colour should show: %v", s)
	}
}
