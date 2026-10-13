// Package label finds and cuts the label out of a photo of a cartridge.
//
// ScreenScraper's GBA photos are all drawn from one template, so for those we
// use the label's known position on it. For other photos we learn the plastic
// colour from the strips beside the label, mark every pixel that differs from
// it, and take the biggest solid block of marked rows and columns. If that
// doesn't produce something label-shaped, we fall back to the standard
// position. The app lets you redraw the box by hand either way.
package label

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
	"sort"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"slotlabelmaker/internal/platforms"
)

// Box is a rectangle in image pixels: left, top, right, bottom.
type Box [4]int

// MaxPixels guards against huge images.
const MaxPixels = 40_000_000

// Decode reads a PNG, JPEG, WebP or GIF into RGBA.
func Decode(data []byte) (*image.NRGBA, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("that isn't a picture this app can read")
	}
	if cfg.Width*cfg.Height > MaxPixels {
		return nil, fmt.Errorf("the picture is too big (%dx%d)", cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("the picture is damaged: %v", err)
	}
	out := image.NewNRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	return out, nil
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// CartridgeBounds is the part of the picture that is the cartridge, ignoring background.
func CartridgeBounds(img *image.NRGBA) Box {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	minAlpha := uint8(255)
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] < minAlpha {
			minAlpha = img.Pix[i]
		}
	}
	var inside func(x, y int) bool
	if minAlpha < 250 { // transparent background
		inside = func(x, y int) bool { return img.NRGBAAt(x, y).A > 32 }
	} else {
		corners := []color.NRGBA{img.NRGBAAt(0, 0), img.NRGBAAt(w-1, 0), img.NRGBAAt(0, h-1), img.NRGBAAt(w-1, h-1)}
		var bg [3]uint8
		for c := 0; c < 3; c++ {
			v := []int{}
			for _, k := range corners {
				v = append(v, int([]uint8{k.R, k.G, k.B}[c]))
			}
			sort.Ints(v)
			bg[c] = uint8(v[1])
		}
		inside = func(x, y int) bool {
			p := img.NRGBAAt(x, y)
			// Same weighting as converting the difference to grey.
			l := 0.299*float64(absDiff(p.R, bg[0])) + 0.587*float64(absDiff(p.G, bg[1])) + 0.114*float64(absDiff(p.B, bg[2]))
			return l > 30
		}
	}
	l, t, r, b := w, h, 0, 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if inside(x, y) {
				if x < l {
					l = x
				}
				if x+1 > r {
					r = x + 1
				}
				if y < t {
					t = y
				}
				if y+1 > b {
					b = y + 1
				}
			}
		}
	}
	if r <= l || b <= t {
		return Box{0, 0, w, h}
	}
	return Box{l, t, r, b}
}

func longestRun(values []float64, threshold float64) (int, int) {
	bs, be, start := 0, 0, -1
	for i := 0; i <= len(values); i++ {
		on := i < len(values) && values[i] >= threshold
		if on && start < 0 {
			start = i
		} else if !on && start >= 0 {
			if i-start > be-bs {
				bs, be = start, i
			}
			start = -1
		}
	}
	return bs, be
}

// inset pulls in a hair so the shell and the label's rounded corners stay out.
func inset(l, t, r, b float64) (float64, float64, float64, float64) {
	dx, dy := (r-l)*0.006, (b-t)*0.01
	return l + dx, t + dy, r - dx, b - dy
}

func standardBox(cart Box, p *platforms.Platform) Box {
	cl, ct := float64(cart[0]), float64(cart[1])
	cw, ch := float64(cart[2]-cart[0]), float64(cart[3]-cart[1])
	f := p.LabelBox
	l, t, r, b := inset(cl+cw*f[0], ct+ch*f[1], cl+cw*f[2], ct+ch*f[3])
	return Box{int(math.Round(l)), int(math.Round(t)), int(math.Round(r)), int(math.Round(b))}
}

// snapToEdges moves each side of a roughly placed box to where the label
// really ends. Starting a little inside the label, it steps outwards line by
// line until the line is mostly cartridge plastic. A side that never finds
// plastic nearby stays where it was.
func snapToEdges(img *image.NRGBA, cart Box, rough [4]float64) [4]float64 {
	l, t, r, b := rough[0], rough[1], rough[2], rough[3]
	w, h := r-l, b-t
	cl, ct, cr, cb := float64(cart[0]), float64(cart[1]), float64(cart[2]), float64(cart[3])
	rnd := func(v float64) int { return int(math.Round(v)) }

	// Plastic colour: the strips of shell either side of the label.
	var plastic [3]float64
	for c := 0; c < 3; c++ {
		sum := 0.0
		for _, s := range [][2]float64{{cl + (l-cl)*0.25, cl + (l-cl)*0.75}, {r + (cr-r)*0.25, r + (cr-r)*0.75}} {
			var vals []int
			for y := rnd(t + h*0.3); y < rnd(t+h*0.7); y++ {
				for x := rnd(s[0]); x < rnd(s[1]); x++ {
					px := img.NRGBAAt(x, y)
					vals = append(vals, int([]uint8{px.R, px.G, px.B}[c]))
				}
			}
			if len(vals) > 0 {
				sum += float64(median(vals))
			}
		}
		plastic[c] = sum / 2
	}
	isPlastic := func(x, y int) bool {
		p := img.NRGBAAt(x, y)
		d := math.Max(math.Abs(float64(p.R)-plastic[0]), math.Max(math.Abs(float64(p.G)-plastic[1]), math.Abs(float64(p.B)-plastic[2])))
		return d < 24
	}
	x0, x1 := rnd(l+w*0.2), rnd(r-w*0.2)
	y0, y1 := rnd(t+h*0.2), rnd(b-h*0.2)
	rowShare := func(y int) float64 {
		n := 0
		for x := x0; x < x1; x++ {
			if isPlastic(x, y) {
				n++
			}
		}
		return float64(n) / float64(max(1, x1-x0))
	}
	colShare := func(x int) float64 {
		n := 0
		for y := y0; y < y1; y++ {
			if isPlastic(x, y) {
				n++
			}
		}
		return float64(n) / float64(max(1, y1-y0))
	}
	// Two plastic lines in a row, so a dark stripe in the artwork isn't taken for the edge.
	search := func(start, stop float64, step int, share func(int) float64) (int, bool) {
		for v := rnd(start); (step < 0 && v > rnd(stop)) || (step > 0 && v < rnd(stop)); v += step {
			if share(v) >= 0.85 && share(v+step) >= 0.85 {
				return v, true
			}
		}
		return 0, false
	}
	reachX, reachY := w*0.06, h*0.08
	out := rough
	if v, ok := search(t+reachY, math.Max(ct, t-reachY), -1, rowShare); ok {
		out[1] = float64(v + 1)
	}
	if v, ok := search(b-reachY, math.Min(cb, b+reachY), 1, rowShare); ok {
		out[3] = float64(v)
	}
	if v, ok := search(l+reachX, math.Max(cl, l-reachX), -1, colShare); ok {
		out[0] = float64(v + 1)
	}
	if v, ok := search(r-reachX, math.Min(cr, r+reachX), 1, colShare); ok {
		out[2] = float64(v)
	}
	return out
}

func median(v []int) int {
	sort.Ints(v)
	return v[len(v)/2]
}

// FindBox returns the label's box and how it was found: "template",
// "detected", "whole image" or "standard position".
func FindBox(img *image.NRGBA, p *platforms.Platform) (Box, string) {
	cart := CartridgeBounds(img)
	cw, ch := cart[2]-cart[0], cart[3]-cart[1]
	if cw < 10 || ch < 10 {
		return Box{0, 0, img.Bounds().Dx(), img.Bounds().Dy()}, "whole image"
	}
	aspect := float64(cw) / float64(ch)
	// Already just a label? Then the picture's shape matches a label.
	if math.Abs(aspect-p.LabelAspect)/p.LabelAspect < 0.06 {
		return cart, "whole image"
	}
	if p.TemplateAspect[0] > 0 && aspect >= p.TemplateAspect[0] && aspect <= p.TemplateAspect[1] {
		f := p.LabelBox
		cl, ct := float64(cart[0]), float64(cart[1])
		rough := [4]float64{cl + float64(cw)*f[0], ct + float64(ch)*f[1], cl + float64(cw)*f[2], ct + float64(ch)*f[3]}
		s := snapToEdges(img, cart, rough)
		l, t, r, b := inset(s[0], s[1], s[2], s[3])
		return Box{int(math.Round(l)), int(math.Round(t)), int(math.Round(r)), int(math.Round(b))}, "template"
	}

	// Work on a small copy, it's much faster and just as good for this.
	factor := math.Min(1, 300/float64(max(cw, ch)))
	sw, sh := max(1, int(math.Round(float64(cw)*factor))), max(1, int(math.Round(float64(ch)*factor)))
	small := image.NewNRGBA(image.Rect(0, 0, sw, sh))
	draw.BiLinear.Scale(small, small.Bounds(), img, image.Rect(cart[0], cart[1], cart[2], cart[3]), draw.Src, nil)

	// Plastic colour: thin strips down the left and right sides, beside the label.
	var plastic [3]int
	for c := 0; c < 3; c++ {
		var meds []int
		for _, s := range [][2]float64{{0.015, 0.06}, {0.94, 0.985}} {
			var vals []int
			for y := int(math.Round(float64(sh) * 0.35)); y < int(math.Round(float64(sh)*0.85)); y++ {
				for x := int(math.Round(float64(sw) * s[0])); x < int(math.Round(float64(sw)*s[1])); x++ {
					px := small.NRGBAAt(x, y)
					vals = append(vals, int([]uint8{px.R, px.G, px.B}[c]))
				}
			}
			if len(vals) > 0 {
				meds = append(meds, median(vals))
			}
		}
		if len(meds) > 0 {
			sum := 0
			for _, m := range meds {
				sum += m
			}
			plastic[c] = int(math.Round(float64(sum) / float64(len(meds))))
		}
	}
	mask := make([]bool, sw*sh)
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			px := small.NRGBAAt(x, y)
			d := max(absDiff(px.R, uint8(plastic[0])), absDiff(px.G, uint8(plastic[1])), absDiff(px.B, uint8(plastic[2])))
			mask[y*sw+x] = d > 38
		}
	}
	x0, x1 := int(math.Round(float64(sw)*0.15)), int(math.Round(float64(sw)*0.85))
	rows := make([]float64, sh)
	for y := 0; y < sh; y++ {
		n := 0
		for x := x0; x < x1; x++ {
			if mask[y*sw+x] {
				n++
			}
		}
		rows[y] = float64(n) / float64(max(1, x1-x0))
	}
	top, bottom := longestRun(rows, 0.6)
	if float64(bottom-top) < float64(sh)*0.4 {
		return standardBox(cart, p), "standard position"
	}
	cols := make([]float64, sw)
	for x := 0; x < sw; x++ {
		n := 0
		for y := top; y < bottom; y++ {
			if mask[y*sw+x] {
				n++
			}
		}
		cols[x] = float64(n) / float64(max(1, bottom-top))
	}
	left, right := longestRun(cols, 0.6)
	width, height := float64(right-left), float64(bottom-top)
	if width < float64(sw)*0.6 || math.Abs(width/height-p.LabelAspect)/p.LabelAspect > 0.3 {
		return standardBox(cart, p), "standard position"
	}
	l, t, r, b := inset(float64(left), float64(top), float64(right), float64(bottom))
	s := 1 / factor
	return Box{
		int(math.Round(l*s)) + cart[0], int(math.Round(t*s)) + cart[1],
		int(math.Round(r*s)) + cart[0], int(math.Round(b*s)) + cart[1],
	}, "detected"
}

// Clamp keeps a box inside the image and at least a few pixels big.
func Clamp(b Box, img image.Image) (Box, error) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if b[0] > b[2] {
		b[0], b[2] = b[2], b[0]
	}
	if b[1] > b[3] {
		b[1], b[3] = b[3], b[1]
	}
	b[0], b[1] = max(0, b[0]), max(0, b[1])
	b[2], b[3] = min(w, b[2]), min(h, b[3])
	if b[2]-b[0] < 8 || b[3]-b[1] < 8 {
		return b, fmt.Errorf("the label box is too small")
	}
	return b, nil
}

// Room is how far a box may stick out past a picture you chose (mx left
// and right, my above and below): enough to fit the whole picture in the
// label's shape (width / height), with a little white to spare.
func Room(img image.Image, aspect float64) (mx, my int) {
	w, h := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	bw := max(w, h*aspect) * 1.1
	bh := bw / aspect
	return int(math.Ceil((bw - w) / 2)), int(math.Ceil((bh - h) / 2))
}

// ClampRoom is Clamp for a picture you chose: the box may stick out past it
// by up to mx and my (see Room), but must still cover some of it.
func ClampRoom(b Box, img image.Image, mx, my int) (Box, error) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if b[0] > b[2] {
		b[0], b[2] = b[2], b[0]
	}
	if b[1] > b[3] {
		b[1], b[3] = b[3], b[1]
	}
	b[0], b[1] = max(-mx, b[0]), max(-my, b[1])
	b[2], b[3] = min(w+mx, b[2]), min(h+my, b[3])
	if b[2]-b[0] < 8 || b[3]-b[1] < 8 {
		return b, fmt.Errorf("the label box is too small")
	}
	if b[2] <= 0 || b[3] <= 0 || b[0] >= w || b[1] >= h {
		return b, fmt.Errorf("the label box is off the picture")
	}
	return b, nil
}

// Crop cuts out the label and scales it to the size Slot uses. Any part of
// the box past the picture's edge comes out white.
func Crop(img *image.NRGBA, b Box, p *platforms.Platform) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, p.LabelW, p.LabelH))
	// Labels are opaque: fill white first so any see-through edge isn't black.
	draw.Draw(out, out.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	r := image.Rect(b[0], b[1], b[2], b[3])
	src := img
	if !r.In(img.Bounds()) {
		// Copy what there is into a see-through picture the box's size.
		src = image.NewNRGBA(r)
		draw.Draw(src, r, img, r.Min, draw.Src)
	}
	draw.CatmullRom.Scale(out, out.Bounds(), src, r, draw.Over, nil)
	return out
}

// EncodePNG encodes a label.
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	err := enc.Encode(&buf, img)
	return buf.Bytes(), err
}

// ShellColour picks the label's most common strong colour, as hex for
// cart_shell.ini. Colourful beats grey and white.
func ShellColour(lbl image.Image) string {
	small := image.NewNRGBA(image.Rect(0, 0, 64, 33))
	draw.ApproxBiLinear.Scale(small, small.Bounds(), lbl, lbl.Bounds(), draw.Src, nil)
	// Group colours into buckets of 32 per channel, then average each bucket.
	type acc struct{ r, g, b, n int }
	buckets := map[int]*acc{}
	for i := 0; i < len(small.Pix); i += 4 {
		r, g, b := int(small.Pix[i]), int(small.Pix[i+1]), int(small.Pix[i+2])
		k := (r>>5)<<6 | (g>>5)<<3 | b>>5
		a := buckets[k]
		if a == nil {
			a = &acc{}
			buckets[k] = a
		}
		a.r, a.g, a.b, a.n = a.r+r, a.g+g, a.b+b, a.n+1
	}
	best, bestScore := [3]int{128, 128, 128}, -1.0
	keys := make([]int, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Ints(keys) // deterministic
	for _, k := range keys {
		a := buckets[k]
		r, g, b := a.r/a.n, a.g/a.n, a.b/a.n
		sat := max(r, g, b) - min(r, g, b)
		score := float64(a.n) * (0.3 + float64(sat)/255)
		if score > bestScore {
			best, bestScore = [3]int{r, g, b}, score
		}
	}
	return fmt.Sprintf("%02x%02x%02x", best[0], best[1], best[2])
}

// PlasticColour is the colour of the cartridge's own plastic in a photo,
// read from the margins around the label (lbl). ok is false when the
// plastic is grey, black or white, or there's too little to go on, so the
// caller can keep its usual grey.
func PlasticColour(img *image.NRGBA, lbl Box) (hex string, ok bool) {
	cart := CartridgeBounds(img)
	ml, mr, mb := lbl[0]-cart[0], cart[2]-lbl[2], cart[3]-lbl[3]
	if ml < 4 || mr < 4 || mb < 4 {
		return "", false
	}
	// The middle of each margin, away from the edges' shading and the
	// label's border: left and right of the label, and under it.
	areas := []image.Rectangle{
		image.Rect(cart[0]+ml/4, lbl[1]+(lbl[3]-lbl[1])/5, lbl[0]-ml/4, lbl[3]-(lbl[3]-lbl[1])/5),
		image.Rect(lbl[2]+mr/4, lbl[1]+(lbl[3]-lbl[1])/5, cart[2]-mr/4, lbl[3]-(lbl[3]-lbl[1])/5),
		image.Rect(lbl[0]+(lbl[2]-lbl[0])/6, lbl[3]+mb/4, lbl[2]-(lbl[2]-lbl[0])/6, cart[3]-mb/3),
	}
	type px struct{ r, g, b, lum int }
	var all []px
	b := img.Bounds()
	for _, a := range areas {
		a = a.Intersect(b)
		step := max(1, max(a.Dx(), a.Dy())/60)
		for y := a.Min.Y; y < a.Max.Y; y += step {
			for x := a.Min.X; x < a.Max.X; x += step {
				i := img.PixOffset(x, y)
				if img.Pix[i+3] < 200 {
					continue
				}
				r, g, bl := int(img.Pix[i]), int(img.Pix[i+1]), int(img.Pix[i+2])
				all = append(all, px{r, g, bl, 3*r + 6*g + bl})
			}
		}
	}
	if len(all) < 30 {
		return "", false
	}
	// Skip the shadows and the glare: average the brighter middle by
	// brightness, since photos come out a little darker than the plastic.
	sort.Slice(all, func(i, j int) bool { return all[i].lum < all[j].lum })
	mid := all[len(all)*2/5 : len(all)*7/8]
	var r, g, bl int
	for _, p := range mid {
		r, g, bl = r+p.r, g+p.g, bl+p.b
	}
	r, g, bl = r/len(mid), g/len(mid), bl/len(mid)
	hi, lo := max(r, g, bl), min(r, g, bl)
	if hi < 50 || float64(hi-lo)/float64(hi) < 0.3 {
		return "", false // grey, black or white plastic
	}
	return fmt.Sprintf("%02x%02x%02x", r, g, bl), true
}

// Shrink scales a picture down so its longer side is at most size pixels.
func Shrink(img *image.NRGBA, size int) *image.NRGBA {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if w <= size && h <= size {
		return img
	}
	f := float64(size) / float64(max(w, h))
	out := image.NewNRGBA(image.Rect(0, 0, max(1, int(float64(w)*f)), max(1, int(float64(h)*f))))
	draw.CatmullRom.Scale(out, out.Bounds(), img, img.Bounds(), draw.Src, nil)
	return out
}

// FitBox is the biggest centred box of the given shape (width / height) that
// fits in the picture, so a label isn't stretched.
func FitBox(img image.Image, aspect float64) Box {
	w, h := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	bw, bh := w, w/aspect
	if bh > h {
		bw, bh = h*aspect, h
	}
	l, t := (w-bw)/2, (h-bh)/2
	return Box{int(math.Round(l)), int(math.Round(t)), int(math.Round(l + bw)), int(math.Round(t + bh))}
}
