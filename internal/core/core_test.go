package core

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"slotlabelmaker/internal/label"
	"slotlabelmaker/internal/platforms"
	"slotlabelmaker/internal/romm"
	"slotlabelmaker/internal/sdcard"
)

// fakeCart is a grey GBA cartridge 570 wide with a red label in the template spot.
func fakeCart() ([]byte, label.Box) {
	h := 350
	img := image.NewNRGBA(image.Rect(0, 0, 640, 60+h))
	grey := color.NRGBA{150, 150, 155, 255}
	for y := 30; y < 30+h; y++ {
		for x := 40; x < 610; x++ {
			img.Set(x, y, grey)
		}
	}
	lb := label.Box{121, 30 + int(float64(h)*0.228), 535, 30 + int(float64(h)*0.863)}
	for y := lb[1]; y < lb[3]; y++ {
		for x := lb[0]; x < lb[2]; x++ {
			img.Set(x, y, color.NRGBA{200, 40, 40, 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes(), lb
}

func makeCard(t *testing.T) string {
	root := t.TempDir()
	for _, d := range []string{"Games/GBA", "System", "Labels/GBA"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	os.WriteFile(filepath.Join(root, "Games/GBA/Pokemon Radical Red.gba"), []byte("radical"), 0o644)
	os.WriteFile(filepath.Join(root, "Labels/cart_shell.ini"), []byte("Pokemon Radical Red = auto c0282c solid\n"), 0o644)
	return root
}

func fakeRomM(t *testing.T) *httptest.Server {
	cart, _ := fakeCart()
	var zipped bytes.Buffer
	zw := zip.NewWriter(&zipped)
	w, _ := zw.Create("Pokemon UnBound.gba")
	w.Write([]byte("unbound rom"))
	zw.Close()
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer good-token" {
				w.WriteHeader(401)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/api/platforms", auth(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{{"id": 7, "slug": "gba", "name": "Game Boy Advance"}, {"id": 9, "slug": "snes"}})
	}))
	mux.HandleFunc("/api/roms", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("platform_ids") != "7" {
			json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "total": 0})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"total": 2, "items": []map[string]any{
			{"id": 1, "name": "Pokemon Radical Red", "fs_name": "Pokemon Radical Red.gba", "fs_name_no_ext": "Pokemon Radical Red",
				"ss_metadata": map[string]any{"physical_path": "roms/1/physical.png"}},
			{"id": 2, "name": "Pokemon Unbound", "fs_name": "Pokemon UnBound.zip", "fs_name_no_ext": "Pokemon UnBound", "fs_size_bytes": 11,
				"ss_metadata": map[string]any{"physical_path": "roms/2/physical.png"}},
		}})
	}))
	mux.HandleFunc("/assets/romm/resources/", auth(func(w http.ResponseWriter, r *http.Request) { w.Write(cart) }))
	mux.HandleFunc("/api/roms/2/content/", auth(func(w http.ResponseWriter, r *http.Request) { w.Write(zipped.Bytes()) }))
	return httptest.NewServer(mux)
}

func TestWrongLogin(t *testing.T) {
	srv := fakeRomM(t)
	defer srv.Close()
	err := New().Connect(srv.URL, "", "", "bad")
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("want refused login, got %v", err)
	}
}

func TestAddGameAndLabels(t *testing.T) {
	srv := fakeRomM(t)
	defer srv.Close()
	root := makeCard(t)
	c := New()
	if _, err := c.OpenCard(root); err != nil {
		t.Fatal(err)
	}
	if err := c.Connect(srv.URL, "", "", "good-token"); err != nil {
		t.Fatal(err)
	}
	lib := c.Library()
	if len(lib) != 2 || !lib[0].OnCard || lib[0].RomID != 1 || lib[0].HasLabel || lib[1].OnCard {
		t.Fatalf("library: %+v", lib)
	}
	added, err := c.AddGame(2, func(string, float64) {})
	if err != nil || !added.Label {
		t.Fatalf("add: %+v %v", added, err)
	}
	got, _ := os.ReadFile(filepath.Join(root, "Games/GBA/Pokemon UnBound.gba"))
	if string(got) != "unbound rom" {
		t.Fatalf("rom = %q", got)
	}
	if _, err := c.AddGame(2, func(string, float64) {}); err == nil {
		t.Fatal("adding twice should fail")
	}
	made, problems := c.MissingLabels(func(int, int) {})
	if made != 1 || len(problems) != 0 {
		t.Fatalf("made %d %v", made, problems)
	}
	f, _ := os.Open(filepath.Join(root, "Labels/GBA/Pokemon UnBound.png"))
	cfg, err := png.DecodeConfig(f)
	f.Close()
	if err != nil || cfg.Width != 1280 || cfg.Height != 661 {
		t.Fatalf("label %v %v", cfg, err)
	}
	shells := sdcard.ReadShells(root)
	if shells["Pokemon Radical Red"].Colour != "c0282c" { // existing line kept
		t.Fatalf("shells %v", shells)
	}
	if s := shells["Pokemon UnBound"]; s.Colour != DefaultShell.Colour || s.Finish != "solid" { // no colour chosen: GBA grey, not Slot's plain black
		t.Fatalf("new shell %v", s)
	}
	info, _ := c.Card()
	if info.Games != 2 || info.MissingLabels != 0 {
		t.Fatalf("card %+v", info)
	}
}

func TestRemoveGame(t *testing.T) {
	root := makeCard(t)
	os.WriteFile(filepath.Join(root, "Labels/GBA/Pokemon Radical Red.png"), []byte("png"), 0o644)
	os.MkdirAll(filepath.Join(root, "Saves/GBA"), 0o755)
	save := filepath.Join(root, "Saves/GBA/Pokemon Radical Red.sav")
	os.WriteFile(save, []byte("save"), 0o644)
	c := New()
	c.OpenCard(root)
	if err := c.RemoveGame("GBA", "Pokemon Radical Red", "Pokemon Radical Red.gba"); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"Games/GBA/Pokemon Radical Red.gba", "Labels/GBA/Pokemon Radical Red.png"} {
		if _, err := os.Stat(filepath.Join(root, gone)); !os.IsNotExist(err) {
			t.Errorf("%s still there", gone)
		}
	}
	if _, err := os.Stat(save); err != nil {
		t.Error("the save should be kept")
	}
	if _, ok := sdcard.ReadShells(root)["Pokemon Radical Red"]; ok {
		t.Error("shell line should be gone")
	}
	if err := c.RemoveGame("GBA", "../../etc", "x.gba"); err == nil {
		t.Error("removing something not on the card should fail")
	}
}

func TestRemoveKeepsTwinLabel(t *testing.T) {
	root := makeCard(t)
	os.MkdirAll(filepath.Join(root, "Games/GBC"), 0o755)
	os.MkdirAll(filepath.Join(root, "Labels/GBC"), 0o755)
	os.WriteFile(filepath.Join(root, "Games/GBC/Tetris.gb"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(root, "Games/GBC/Tetris.gbc"), []byte("b"), 0o644)
	lbl := filepath.Join(root, "Labels/GBC/Tetris.png")
	os.WriteFile(lbl, []byte("png"), 0o644)
	c := New()
	c.OpenCard(root)
	if err := c.RemoveGame("GBC", "Tetris", "Tetris.gb"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lbl); err != nil {
		t.Fatal("the label is still used by Tetris.gbc")
	}
	if _, err := os.Stat(filepath.Join(root, "Games/GBC/Tetris.gbc")); err != nil {
		t.Fatal("the twin should be untouched")
	}
	if err := c.SetShell("../x", sdcard.Shell{Colour: "ffffff", Finish: "solid"}); err == nil {
		t.Fatal("bad names should be refused")
	}
}

func TestTemplateBox(t *testing.T) {
	data, want := fakeCart()
	img, _ := label.Decode(data)
	box, how := label.FindBox(img, platforms.GBA)
	if how != "template" {
		t.Fatal(how)
	}
	for i := range box {
		if d := box[i] - want[i]; d < -20 || d > 20 {
			t.Fatalf("box %v want ~%v", box, want)
		}
	}
	if box[0] < want[0] || box[1] < want[1] || box[2] > want[2] || box[3] > want[3] {
		t.Fatalf("box %v should be inside %v", box, want)
	}
	if c := label.ShellColour(label.Crop(img, box, platforms.GBA)); !strings.HasPrefix(c, "c") {
		t.Fatalf("shell colour %s", c)
	}
}

func TestSafeFileName(t *testing.T) {
	for in, want := range map[string]string{`..\..\evil`: "evil", "con": "_con", `a:b?`: "ab", "...": "game", "Fine Name": "Fine Name"} {
		if got := sdcard.SafeFileName(in); got != want {
			t.Errorf("%q -> %q want %q", in, got, want)
		}
	}
}

func TestUnpack(t *testing.T) {
	r := &romm.Rom{Name: "Tetris", FsName: "Tetris.gb"}
	data, ext, err := Unpack([]byte("plain rom"), r, platforms.GBC)
	if err != nil || ext != ".gb" || string(data) != "plain rom" {
		t.Fatalf("plain: %q %q %v", data, ext, err)
	}
	if _, _, err := Unpack([]byte("7z\xbc\xaf\x27\x1c damaged"), r, platforms.GBC); err == nil {
		t.Fatal("a damaged 7z should be an error, not saved as a game")
	}
	seven, _ := os.ReadFile("testdata/game.7z") // readme.txt + "Tetris DX.gbc"
	data, ext, err = Unpack(seven, r, platforms.GBC)
	if err != nil || ext != ".gbc" || !strings.HasPrefix(string(data), "seven rom data") {
		t.Fatalf("7z: %q %v", ext, err)
	}
	rar, _ := os.ReadFile("testdata/small.rar") // holds asd.go, 187 bytes
	goFiles := &platforms.Platform{Key: "X", Extensions: []string{".go"}}
	data, ext, err = Unpack(rar, r, goFiles)
	if err != nil || ext != ".go" || len(data) != 187 {
		t.Fatalf("rar: %q %d %v", ext, len(data), err)
	}
	if _, _, err := Unpack(rar, r, platforms.GBA); err == nil {
		t.Fatal("a rar with no game in it should be an error")
	}
}

func TestOwnLabel(t *testing.T) {
	root := makeCard(t)
	c := New()
	c.OpenCard(root)
	sq := image.NewNRGBA(image.Rect(0, 0, 400, 400)) // a black square picture: trimmed, not stretched
	for i := 3; i < len(sq.Pix); i += 4 {
		sq.Pix[i] = 255
	}
	var buf bytes.Buffer
	png.Encode(&buf, sq)
	plan, err := c.PlanOwnLabel("GBA", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if b := plan.Box; plan.How != "fitted" || b[0] != 0 || b[2] != 400 || b[1] <= 0 || b[3] >= 400 {
		t.Fatalf("fit box %v %s", b, plan.How)
	}
	if plan.Token == "" || plan.MarginX <= 0 || plan.MarginY < 0 {
		t.Fatalf("plan %+v", plan)
	}
	if _, err := c.OwnImage(plan.Token); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveOwnLabel("GBA", "../evil", plan.Token, plan.Box, nil); err == nil {
		t.Fatal("bad names should be refused")
	}
	// Zoomed right out: the whole picture, with white either side.
	wide := label.Box{-plan.MarginX, -plan.MarginY, 400 + plan.MarginX, 400 + plan.MarginY}
	if _, err := c.OwnCropPNG(plan.Token, wide); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveOwnLabel("GBA", "Pokemon Radical Red", plan.Token, wide, nil); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(filepath.Join(root, "Labels/GBA/Pokemon Radical Red.png"))
	lbl, err := png.Decode(f)
	f.Close()
	if err != nil || lbl.Bounds().Dx() != 1280 || lbl.Bounds().Dy() != 661 {
		t.Fatalf("label %v %v", lbl.Bounds(), err)
	}
	if r, g, b, _ := lbl.At(5, 330).RGBA(); r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Fatalf("past the picture should be white, got %d %d %d", r>>8, g>>8, b>>8)
	}
	if r, _, _, _ := lbl.At(640, 330).RGBA(); r>>8 > 10 {
		t.Fatalf("the picture itself should be black, got %d", r>>8)
	}
	if err := c.SaveOwnLabel("GBA", "Pokemon Radical Red", plan.Token, plan.Box, nil); err == nil {
		t.Fatal("a saved picture should be forgotten")
	}

	cart, _ := fakeCart() // a whole cartridge photo: starts on its label
	plan, err = c.PlanOwnLabel("GBA", cart)
	if err != nil || (plan.How != "template" && plan.How != "detected") {
		t.Fatalf("cart photo: %+v %v", plan, err)
	}
	c.DropOwnLabel(plan.Token)
	if _, err := c.OwnImage(plan.Token); err == nil {
		t.Fatal("a dropped picture should be gone")
	}
}
