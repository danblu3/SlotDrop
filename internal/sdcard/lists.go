package sdcard

import (
	"os"
	"path/filepath"
	"strings"

	"slotlabelmaker/internal/platforms"
)

// Slot keeps its favourites and recently played games in Config/favorites.txt
// and Config/recents.txt, one game per line as "<platform folder>/<rom stem>",
// e.g. "gba/Pokemon - Lazarus". The platform is matched without caring about
// case, the stem exactly. SlotDrop only ever adds or removes a game's own
// lines and leaves everything else in the file as it was.

const (
	favouritesFile = "favorites.txt"
	recentsFile    = "recents.txt"
)

func listPath(root, name string) string { return filepath.Join(root, "Config", name) }

func listEntry(p *platforms.Platform, stem string) string {
	return strings.ToLower(p.Key) + "/" + stem
}

// isEntry reports whether a line names this game.
func isEntry(line string, p *platforms.Platform, stem string) bool {
	plat, s, ok := strings.Cut(strings.TrimRight(line, "\r"), "/")
	return ok && strings.EqualFold(plat, p.Key) && s == stem
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

func writeLines(path string, lines []string) error {
	text := ""
	if len(lines) > 0 {
		text = strings.Join(lines, "\n") + "\n"
	}
	return WriteFileSafely(path, []byte(text))
}

// Favourites lists the "<platform>/<stem>" keys of Slot's favourites, with
// the platform in SlotDrop's own spelling ("GBA/Pokemon - Lazarus").
func Favourites(root string) map[string]bool {
	out := map[string]bool{}
	lines, _ := readLines(listPath(root, favouritesFile))
	for _, line := range lines {
		plat, stem, ok := strings.Cut(strings.TrimRight(line, "\r"), "/")
		if p := platforms.ByKey(strings.ToUpper(plat)); ok && p != nil && stem != "" {
			out[p.Key+"/"+stem] = true
		}
	}
	return out
}

// SetFavourite adds the game to Slot's favourites, or takes it off.
func SetFavourite(root string, p *platforms.Platform, stem string, on bool) error {
	path := listPath(root, favouritesFile)
	lines, err := readLines(path)
	if err != nil {
		return err
	}
	var kept []string
	found := false
	for _, line := range lines {
		if isEntry(line, p, stem) {
			if on && !found {
				kept = append(kept, line)
			}
			found = true
			continue
		}
		kept = append(kept, line)
	}
	if on && !found {
		kept = append(kept, listEntry(p, stem))
	}
	if !on && !found {
		return nil // nothing to change
	}
	return writeLines(path, kept)
}

// ForgetGame takes a game off Slot's favourites and recents, for when it's
// removed from the card.
func ForgetGame(root string, p *platforms.Platform, stem string) error {
	for _, name := range []string{favouritesFile, recentsFile} {
		path := listPath(root, name)
		lines, err := readLines(path)
		if err != nil {
			return err
		}
		var kept []string
		for _, line := range lines {
			if !isEntry(line, p, stem) {
				kept = append(kept, line)
			}
		}
		if len(kept) != len(lines) {
			if err := writeLines(path, kept); err != nil {
				return err
			}
		}
	}
	return nil
}
