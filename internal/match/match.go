// Package match pairs the games on the SD card with the games in RomM.
//
// Romhacks often have different file names on the card than in RomM, so we
// try a few things in order: the exact file name, a cleaned-up title, then
// the ROM's checksum (CRC32), which is the same wherever the file lives.
package match

import (
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"regexp"
	"strings"

	"slotlabelmaker/internal/romm"
	"slotlabelmaker/internal/sdcard"
)

var (
	extRe    = regexp.MustCompile(`(?i)\.(gba|gbc|gb|zip)$`)
	tagRe    = regexp.MustCompile(`[\(\[][^\)\]]*[\)\]]`)
	nonAlnum = regexp.MustCompile(`[^a-z0-9]`)
)

// Normalise turns "Pokemon - Emerald Version (USA, Europe)" into "pokemonemeraldversion".
func Normalise(name string) string {
	name = extRe.ReplaceAllString(name, "")
	name = tagRe.ReplaceAllString(name, "") // drop (USA), [!], (v1.1) ...
	name = strings.ToLower(name)
	name = strings.NewReplacer("&", "and", "é", "e").Replace(name)
	return nonAlnum.ReplaceAllString(name, "")
}

func cleanCRC(c string) string { return strings.TrimLeft(strings.ToLower(c), "0") }

// RomCRCs is every checksum RomM knows for a game.
func RomCRCs(r *romm.Rom) map[string]bool {
	out := map[string]bool{}
	if r.CRC != "" {
		out[cleanCRC(r.CRC)] = true
	}
	for _, f := range r.Files {
		if f.CRC != "" {
			out[cleanCRC(f.CRC)] = true
		}
	}
	return out
}

// FileCRC is a file's CRC32 as hex without leading zeros.
func FileCRC(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := crc32.NewIEEE()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return cleanCRC(fmt.Sprintf("%08x", h.Sum32())), nil
}

// ByName matches on the file name, then on the cleaned-up title.
func ByName(stem string, roms []*romm.Rom) *romm.Rom {
	lower := strings.ToLower(stem)
	for _, r := range roms {
		if strings.ToLower(r.FsNameNoExt) == lower {
			return r
		}
	}
	key := Normalise(stem)
	if key == "" {
		return nil
	}
	for _, r := range roms {
		for _, field := range []string{r.FsNameNoTags, r.FsNameNoExt, r.Name} {
			if field != "" && Normalise(field) == key {
				return r
			}
		}
	}
	return nil
}

// ByCRC matches on the ROM's checksum.
func ByCRC(g *sdcard.Game, roms []*romm.Rom) *romm.Rom {
	crc, err := FileCRC(g.RomPath)
	if err != nil {
		return nil
	}
	for _, r := range roms {
		if RomCRCs(r)[crc] {
			return r
		}
	}
	return nil
}

// Result is how one card game matched.
type Result struct {
	Rom *romm.Rom
	How string // "name", "checksum" or ""
}

// Games matches each card game to a RomM game. Key is the game's stem.
func Games(games []*sdcard.Game, roms []*romm.Rom) map[string]Result {
	out := map[string]Result{}
	for _, g := range games {
		if r := ByName(g.Stem, roms); r != nil {
			out[g.Stem] = Result{r, "name"}
		} else if r := ByCRC(g, roms); r != nil {
			out[g.Stem] = Result{r, "checksum"}
		} else {
			out[g.Stem] = Result{}
		}
	}
	return out
}
