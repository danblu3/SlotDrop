// Package platforms describes the three systems Slot plays and how each
// one's cartridge photo and label look.
package platforms

import "strings"

// Platform is one of Slot's systems.
type Platform struct {
	Key        string   // shown in the app and used as the folder name on the card
	Name       string   // full name for the UI
	RommSlugs  []string // what RomM calls this platform
	Extensions []string // ROM file types Slot loads for it
	// Where the label sits on the cartridge photo, as fractions of the
	// cartridge's width and height: left, top, right, bottom.
	LabelBox    [4]float64
	LabelAspect float64 // width / height of a real label
	// ScreenScraper draws every cartridge photo for a system from the same
	// template, so the label is always in the same spot. Photos whose shape
	// falls in this range are treated as that template. Zero means unknown.
	TemplateAspect [2]float64
	LabelW, LabelH int // size labels are saved at
}

var (
	GBA = &Platform{
		Key: "GBA", Name: "Game Boy Advance",
		RommSlugs:      []string{"gba", "game-boy-advance", "gameboy-advance"},
		Extensions:     []string{".gba"},
		LabelBox:       [4]float64{0.142, 0.228, 0.868, 0.863}, // measured on ScreenScraper's GBA photos
		LabelAspect:    1.94,
		TemplateAspect: [2]float64{1.6, 1.78},
		LabelW:         1280, LabelH: 661, // same as the Cart Studio labels already on the card
	}
	GB = &Platform{
		Key: "GB", Name: "Game Boy",
		RommSlugs:   []string{"gb", "game-boy", "gameboy"},
		Extensions:  []string{".gb"},
		LabelBox:    [4]float64{0.10, 0.27, 0.90, 0.88}, // rough guess until we measure a real GB photo
		LabelAspect: 1.12,
		LabelW:      1280, LabelH: 1143,
	}
	GBC = &Platform{
		Key: "GBC", Name: "Game Boy Color",
		RommSlugs:   []string{"gbc", "game-boy-color", "gameboy-color"},
		Extensions:  []string{".gbc", ".gb"},
		LabelBox:    GB.LabelBox,
		LabelAspect: GB.LabelAspect,
		LabelW:      GB.LabelW, LabelH: GB.LabelH,
	}
	All = []*Platform{GBA, GB, GBC}
)

// ByKey finds a platform by its key ("GBA", "GB", "GBC").
func ByKey(key string) *Platform {
	for _, p := range All {
		if strings.EqualFold(p.Key, key) {
			return p
		}
	}
	return nil
}

// BySlug finds the platform RomM calls slug, or nil.
func BySlug(slug string) *Platform {
	slug = strings.ToLower(slug)
	for _, p := range All {
		for _, s := range p.RommSlugs {
			if s == slug {
				return p
			}
		}
	}
	return nil
}

// HasExtension reports whether ext (with dot, any case) is a ROM type for p.
func (p *Platform) HasExtension(ext string) bool {
	ext = strings.ToLower(ext)
	for _, e := range p.Extensions {
		if e == ext {
			return true
		}
	}
	return false
}
