// Fakeromm is a pretend RomM server for the machine tests in
// .github/workflows/machine-test.yml. It serves a handful of made-up games
// (no real game names or artwork) with drawn cartridge photos, and only
// answers requests that carry the test token. It is not part of the app.
//
// Run: go run ./ci/fakeromm -addr 127.0.0.1:8737 -token ci-token
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"strings"

	"golang.org/x/image/draw"

	"slotlabelmaker/internal/platforms"
)

type game struct {
	id       int
	platform int // 1 GBA, 2 GB, 3 GBC
	name     string
	file     string // what RomM calls the file
	cart     bool   // has a cartridge photo
}

var games = []game{
	{1, 1, "Test Quest", "Test Quest (World).gba", true},
	{2, 1, "Pixel Rally", "Pixel Rally (World).gba", true},
	{3, 3, "Pocket Garden", "Pocket Garden (World).zip", false}, // a zip, to check unpacking
	{4, 1, "Sky Courier", "Sky Courier (World).gba", false},
	{5, 2, "Block Drop", "Block Drop (World).gb", false},
	{6, 1, "Lantern Knight", "Lantern Knight (World).gba", true},
}

func pngBytes(img image.Image) []byte {
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

// labelArt draws a made-up label: bands of colour with a pale title block.
func labelArt(seed int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 640, 330))
	hues := []color.NRGBA{{220, 70, 60, 255}, {60, 110, 220, 255}, {60, 170, 100, 255}, {230, 170, 40, 255}, {150, 70, 200, 255}, {40, 160, 180, 255}}
	a, b := hues[seed%len(hues)], hues[(seed+2)%len(hues)]
	for y := 0; y < 330; y++ {
		for x := 0; x < 640; x++ {
			c := a
			if (x+y)/40%2 == 1 {
				c = b
			}
			img.SetNRGBA(x, y, c)
		}
	}
	draw.Draw(img, image.Rect(60, 40, 580, 120), image.NewUniform(color.NRGBA{250, 248, 240, 255}), image.Point{}, draw.Src)
	return img
}

// cartPhoto draws a GBA cartridge shaped like ScreenScraper's photos, with
// the label where SlotDrop expects to find it.
func cartPhoto(seed int) []byte {
	w, h := 700, 417
	shell := color.NRGBA{58, 58, 62, 255}
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
	lbl := labelArt(seed)
	draw.CatmullRom.Scale(img, box, lbl, lbl.Bounds(), draw.Src, nil)
	return pngBytes(img)
}

func cover(seed int) []byte {
	return pngBytes(labelArt(seed + 1))
}

// romFile is the download: a few bytes of "game", zipped if RomM stores a zip.
func romFile(g game) []byte {
	data := bytes.Repeat([]byte{byte(g.id)}, 1<<20)
	if !strings.HasSuffix(g.file, ".zip") {
		return data
	}
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, _ := zw.Create(strings.TrimSuffix(g.file, ".zip") + ".gbc")
	w.Write(data)
	zw.Close()
	return b.Bytes()
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8737", "address to listen on")
	token := flag.String("token", "ci-token", "the API token SlotDrop must send")
	flag.Parse()

	var roms []map[string]any
	for _, g := range games {
		stem := g.file[:strings.LastIndex(g.file, ".")]
		r := map[string]any{
			"id": g.id, "platform_id": g.platform, "name": g.name,
			"fs_name": g.file, "fs_name_no_ext": stem, "fs_size_bytes": len(romFile(g)),
			"created_at":       fmt.Sprintf("2026-0%d-1%dT10:00:00Z", 1+g.id%9, g.id),
			"path_cover_small": fmt.Sprintf("/cov/%d.png", g.id), "regions": []string{"World"},
		}
		if g.cart {
			r["ss_metadata"] = map[string]any{"physical_path": fmt.Sprintf("/cart/%d.png", g.id)}
		}
		roms = append(roms, r)
	}
	find := func(id int) *game {
		for i := range games {
			if games[i].id == id {
				return &games[i]
			}
		}
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/platforms", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "slug": "gba"}, {"id": 2, "slug": "gb"}, {"id": 3, "slug": "gbc"}})
	})
	mux.HandleFunc("/api/roms", func(w http.ResponseWriter, r *http.Request) {
		items := []map[string]any{}
		for _, x := range roms {
			if fmt.Sprint(x["platform_id"]) == r.URL.Query().Get("platform_ids") {
				items = append(items, x)
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items, "total": len(items)})
	})
	mux.HandleFunc("/api/roms/", func(w http.ResponseWriter, r *http.Request) {
		var id int
		if _, err := fmt.Sscanf(r.URL.Path, "/api/roms/%d/content/", &id); err != nil || find(id) == nil {
			http.NotFound(w, r)
			return
		}
		w.Write(romFile(*find(id)))
	})
	mux.HandleFunc("/cov/", func(w http.ResponseWriter, r *http.Request) {
		var n int
		fmt.Sscanf(r.URL.Path, "/cov/%d.png", &n)
		w.Write(cover(n))
	})
	mux.HandleFunc("/cart/", func(w http.ResponseWriter, r *http.Request) {
		var n int
		fmt.Sscanf(r.URL.Path, "/cart/%d.png", &n)
		w.Write(cartPhoto(n))
	})

	authed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" { // for the test script to know we're up
			w.Write([]byte("ok"))
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+*token {
			log.Printf("401 %s %s (no or wrong token)", r.Method, r.URL.Path)
			http.Error(w, `{"detail":"Not authenticated"}`, http.StatusUnauthorized)
			return
		}
		log.Printf("%s %s", r.Method, r.URL.RequestURI())
		mux.ServeHTTP(w, r)
	})
	log.Printf("fake RomM listening on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, authed))
}
