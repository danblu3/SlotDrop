package label

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

// cart draws a cartridge in plastic on a transparent background, with a
// busy label in the middle, and returns it with the label's box.
func cart(plastic color.NRGBA) (*image.NRGBA, Box) {
	img := image.NewNRGBA(image.Rect(0, 0, 400, 260))
	draw.Draw(img, image.Rect(20, 20, 380, 240), image.NewUniform(plastic), image.Point{}, draw.Src)
	// A highlight along the top, like a photo has.
	draw.Draw(img, image.Rect(20, 20, 380, 40), image.NewUniform(color.NRGBA{255, 255, 255, 255}), image.Point{}, draw.Src)
	lbl := Box{70, 70, 330, 200}
	for y := lbl[1]; y < lbl[3]; y++ {
		for x := lbl[0]; x < lbl[2]; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), 200, 255})
		}
	}
	return img, lbl
}

func TestPlasticColour(t *testing.T) {
	img, lbl := cart(color.NRGBA{200, 30, 30, 255})
	if hex, ok := PlasticColour(img, lbl); !ok || hex != "c81e1e" {
		t.Fatalf("red cart: %q %v", hex, ok)
	}
	for _, grey := range []color.NRGBA{{58, 52, 49, 255}, {150, 150, 155, 255}, {20, 20, 22, 255}} {
		img, lbl := cart(grey)
		if hex, ok := PlasticColour(img, lbl); ok {
			t.Fatalf("grey cart %v should keep the default, got %q", grey, hex)
		}
	}
}
