//go:build preview

// Preview runs the window's page in an ordinary browser against a fake RomM
// and a scratch SD card, for checking the UI without Windows. It is not part
// of the app. Run: go run -tags preview . <a real card folder>
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"golang.org/x/image/draw"

	"slotlabelmaker/internal/label"
	"slotlabelmaker/internal/platforms"
)

var titles = map[string][]string{
	"GBA": {"Pokemon - Emerald Version (USA, Europe)", "Pokemon - Lazarus", "Pokemon Pinball - Ruby & Sapphire (Europe) (En,Fr,De,Es,It)", "Pokemon Radical Red", "Pokemon UnBound",
		"Advance Wars (USA)", "Castlevania - Aria of Sorrow (USA)", "Golden Sun (USA, Europe)", "Mario Kart - Super Circuit (USA)", "Metroid Fusion (USA)",
		"Pokemon - FireRed Version (USA, Europe) (Rev 1)", "The Legend of Zelda - The Minish Cap (USA)", "Fire Emblem (USA, Australia)", "Mother 3 (Japan) [T-En]", "Pokemon Gaia"},
	"GB":  {"Tetris (World) (Rev 1)", "Pokemon - Red Version (USA, Europe)", "Kirby's Dream Land (USA, Europe)", "Super Mario Land 2 - 6 Golden Coins (USA, Europe)"},
	"GBC": {"Pokemon - Crystal Version (USA, Europe) (Rev 1)", "The Legend of Zelda - Link's Awakening DX (USA, Europe) (Rev 2)", "Wario Land 3 (World) (En,Ja)"},
}

func pngBytes(img image.Image) []byte {
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

// cartPhoto draws a GBA cartridge like ScreenScraper's, with lbl in the label spot.
func cartPhoto(lbl image.Image, shell color.NRGBA) []byte {
	w, h := 700, 417
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := 34.0
			cx := max(0, max(r-float64(x), float64(x)-(float64(w)-1-r)))
			cy := max(0, max(r-float64(y), float64(y)-(float64(h)-1-r)))
			if cx*cx+cy*cy > r*r {
				continue
			}
			c := shell
			if y < 50 {
				c = color.NRGBA{shell.R + 12, shell.G + 12, shell.B + 12, 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	f := platforms.GBA.LabelBox
	box := image.Rect(int(f[0]*float64(w)), int(f[1]*float64(h)), int(f[2]*float64(w)), int(f[3]*float64(h)))
	draw.Draw(img, box.Inset(-8), image.NewUniform(color.NRGBA{shell.R - 15, shell.G - 15, shell.B - 15, 255}), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(img, box, lbl, lbl.Bounds(), draw.Src, nil)
	return pngBytes(img)
}

func cover(seed int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 300, 300))
	hues := []color.NRGBA{{220, 70, 60, 255}, {60, 110, 220, 255}, {60, 170, 100, 255}, {230, 170, 40, 255}, {150, 70, 200, 255}, {40, 160, 180, 255}}
	c := hues[seed%len(hues)]
	for y := 0; y < 300; y++ {
		for x := 20; x < 280; x++ {
			k := 1 - float64(y)/600
			img.SetNRGBA(x, y, color.NRGBA{uint8(float64(c.R) * k), uint8(float64(c.G) * k), uint8(float64(c.B) * k), 255})
		}
	}
	draw.Draw(img, image.Rect(45, 40, 255, 120), image.NewUniform(color.NRGBA{255, 255, 255, 200}), image.Point{}, draw.Over)
	return pngBytes(img)
}

func main() {
	realCard := os.Args[1]
	card, _ := os.MkdirTemp("", "slotcard")
	for _, d := range []string{"Games/GBA", "Games/GB", "Games/GBC", "System", "Labels/GBA", "BIOS"} {
		os.MkdirAll(filepath.Join(card, d), 0o755)
	}
	os.WriteFile(filepath.Join(card, "System/slot"), []byte("x"), 0o644)
	labels := map[string]image.Image{}
	for _, t := range titles["GBA"][:5] {
		os.WriteFile(filepath.Join(card, "Games/GBA", t+".gba"), bytes.Repeat([]byte{1}, 16<<20), 0o644)
		if data, err := os.ReadFile(filepath.Join(realCard, "Labels/GBA", t+".png")); err == nil {
			if t != "Pokemon Radical Red" { // leave one without a label
				os.WriteFile(filepath.Join(card, "Labels/GBA", t+".png"), data, 0o644)
			}
			labels[t], _ = label.Decode(data)
		}
	}
	ini, _ := os.ReadFile(filepath.Join(realCard, "Labels/cart_shell.ini"))
	if txt, err := os.ReadFile(filepath.Join(realCard, "Labels/cart_shell.txt")); err == nil {
		ini = txt
	}
	os.WriteFile(filepath.Join(card, "Labels/cart_shell.txt"), ini, 0o644)

	// Fake RomM.
	type rom map[string]any
	var roms []rom
	carts := map[int][]byte{}
	pid := map[string]int{"GBA": 1, "GB": 2, "GBC": 3}
	id := 0
	for _, p := range []string{"GBA", "GB", "GBC"} {
		for _, t := range titles[p] {
			id++
			ext := map[string]string{"GBA": ".gba", "GB": ".gb", "GBC": ".gbc"}[p]
			r := rom{"id": id, "platform_id": pid[p], "name": strings.Split(t, " (")[0], "fs_name": t + ext, "fs_name_no_ext": t,
				"fs_size_bytes": (4 + id%5*4) << 20, "created_at": fmt.Sprintf("2026-%02d-%02dT10:00:00Z", 1+id%9, 1+id*7%27), "path_cover_small": fmt.Sprintf("/cov/%d.png", id), "regions": []string{"USA"}}
			if lbl, ok := labels[t]; ok {
				carts[id] = cartPhoto(lbl, color.NRGBA{58, 58, 62, 255})
				r["ss_metadata"] = map[string]any{"physical_path": fmt.Sprintf("/cart/%d.png", id)}
			}
			roms = append(roms, r)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/platforms", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]rom{{"id": 1, "slug": "gba"}, {"id": 2, "slug": "gb"}, {"id": 3, "slug": "gbc"}})
	})
	mux.HandleFunc("/api/roms", func(w http.ResponseWriter, r *http.Request) {
		var items []rom
		for _, x := range roms {
			if fmt.Sprint(x["platform_id"]) == r.URL.Query().Get("platform_ids") {
				items = append(items, x)
			}
		}
		json.NewEncoder(w).Encode(rom{"items": items, "total": len(items)})
	})
	mux.HandleFunc("/cov/", func(w http.ResponseWriter, r *http.Request) {
		var n int
		fmt.Sscanf(r.URL.Path, "/cov/%d.png", &n)
		w.Write(cover(n))
	})
	mux.HandleFunc("/cart/", func(w http.ResponseWriter, r *http.Request) {
		var n int
		fmt.Sscanf(r.URL.Path, "/cart/%d.png", &n)
		w.Write(carts[n])
	})
	mux.HandleFunc("/api/roms/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/content/") {
			for i := 0; i < 20; i++ { // slow enough to see the progress bar
				w.Write(bytes.Repeat([]byte{7}, 200<<10))
				w.(http.Flusher).Flush()
				time.Sleep(60 * time.Millisecond)
			}
			return
		}
		http.NotFound(w, r)
	})
	romm := httptest.NewServer(mux)
	log.Println("fake RomM at", romm.URL, "card at", card)

	app := NewApp()
	app.core.OpenCard(card)
	front := http.FileServer(http.Dir("frontend"))
	http.Handle("/img/", app.imageHandler())
	http.HandleFunc("/romm-url", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(romm.URL)) })
	http.HandleFunc("/call/", func(w http.ResponseWriter, r *http.Request) {
		// Only this page may drive the app, not other websites open in the browser.
		if o := r.Header.Get("Origin"); o != "" && o != "http://127.0.0.1:34115" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/call/")
		var args []json.RawMessage
		json.NewDecoder(r.Body).Decode(&args)
		var out any
		var err error
		switch name {
		case "AddGame":
			var n int
			json.Unmarshal(args[0], &n)
			out, err = app.core.AddGame(n, func(string, float64) {})
		case "MakeMissingLabels":
			made, probs := app.core.MissingLabels(func(int, int) {})
			out = MissingResult{made, probs}
		case "ChooseLabel": // no file dialog in a browser: use a label from the real card
			var plat, stem string
			json.Unmarshal(args[0], &plat)
			json.Unmarshal(args[1], &stem)
			data, _ := os.ReadFile(filepath.Join(realCard, "Labels/GBA/Pokemon UnBound.png"))
			err = app.core.LabelFromImage(plat, stem, data)
			out = err == nil
		case "BrowseForCard":
			out = ""
		default:
			m := reflect.ValueOf(app).MethodByName(name)
			in := []reflect.Value{}
			for i := 0; i < m.Type().NumIn(); i++ {
				v := reflect.New(m.Type().In(i))
				json.Unmarshal(args[i], v.Interface())
				in = append(in, v.Elem())
			}
			res := m.Call(in)
			for _, v := range res {
				if e, ok := v.Interface().(error); ok && e != nil {
					err = e
				} else if v.Type().String() != "error" {
					out = v.Interface()
				}
			}
		}
		if err != nil {
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(err.Error())
			return
		}
		json.NewEncoder(w).Encode(out)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			data, _ := os.ReadFile("frontend/index.html")
			html := strings.Replace(string(data), `<script src="icons.js">`, `<script src="/preview.js"></script><script src="icons.js">`, 1)
			html = strings.Replace(html, "script-src 'self'", "script-src 'self' 'unsafe-inline'", 1)
			w.Write([]byte(html))
			return
		}
		if r.URL.Path == "/preview.js" {
			w.Header().Set("Content-Type", "text/javascript")
			w.Write([]byte(previewJS))
			return
		}
		front.ServeHTTP(w, r)
	})
	log.Fatal(http.ListenAndServe("127.0.0.1:34115", nil))
}

const previewJS = `
window.runtime = { EventsOn(){}, WindowMinimise(){}, WindowToggleMaximise(){}, Quit(){} };
window.go = { main: { App: new Proxy({}, { get: (_, name) => async (...args) => {
  const r = await fetch('/call/' + name, { method: 'POST', body: JSON.stringify(args) });
  const j = await r.json();
  if (!r.ok) throw j;
  return j;
} }) } };
`
