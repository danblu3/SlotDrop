package label

import (
	"image"
	"image/color"
	"testing"

	"slotlabelmaker/internal/platforms"
)

func TestRoomFitsTallPictures(t *testing.T) {
	tall := image.NewNRGBA(image.Rect(0, 0, 300, 900))
	aspect := float64(platforms.GBA.LabelW) / float64(platforms.GBA.LabelH)
	mx, my := Room(tall, aspect)
	// Zoomed right out, the box is in the label's shape and holds the whole picture.
	w, h := 300+2*mx, 900+2*my
	if h < 900 || w < int(900*aspect) {
		t.Fatalf("room %d %d leaves a %dx%d box", mx, my, w, h)
	}
	if _, err := ClampRoom(Box{-mx, -my, 300 + mx, 900 + my}, tall, mx, my); err != nil {
		t.Fatal(err)
	}
	if _, err := ClampRoom(Box{-mx, -my, -mx + 20, -my + 20}, tall, mx, my); err == nil {
		t.Fatal("a box off the picture should be refused")
	}
	if b, _ := ClampRoom(Box{-5000, 0, 100, 100}, tall, mx, my); b[0] != -mx {
		t.Fatalf("box not kept in the room: %v", b)
	}
}

func TestCropPastTheEdge(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+3] = 200, 255 // red
	}
	out := Crop(img, Box{-100, 0, 100, 100}, platforms.GBA) // left half past the edge
	if c := out.NRGBAAt(10, 300); c != (color.NRGBA{255, 255, 255, 255}) {
		t.Fatalf("past the edge should be white, got %v", c)
	}
	if c := out.NRGBAAt(1000, 300); c.R < 190 || c.G > 10 {
		t.Fatalf("the picture should be red, got %v", c)
	}
}
