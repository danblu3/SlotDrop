// Package sdcard reads and writes the Slot folder layout on the SD card.
//
// Slot keeps each platform in its own folders:
//
//	Games/GBA/<name>.gba     Labels/GBA/<name>.png
//	Games/GB/<name>.gb       Labels/GB/<name>.png
//	Games/GBC/<name>.gbc     Labels/GBC/<name>.png
//
// A label must have exactly the same name as its game file (minus the
// extension). The cartridge shell colour for each game lives in
// Labels/cart_shell.txt, one line per game (outline, colour, finish):
//
//	Pokemon Radical Red = auto c0282c solid
//
// Older Slot builds called the file cart_shell.ini. Current Slot renames it
// to .txt on boot, because browsers (and so Cart Studio) refuse to open .ini
// files. Slot also keeps its own defaults in Config/ (or System/), which
// the Labels file overrides.
package sdcard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"slotlabelmaker/internal/platforms"
)

// Game is a game on the card.
type Game struct {
	Platform  *platforms.Platform
	Stem      string // file name without extension, e.g. "Pokemon UnBound"
	RomPath   string
	LabelPath string
	Size      int64
}

// HasLabel reports whether Slot will find a label for the game.
func (g *Game) HasLabel() bool {
	_, err := os.Stat(g.LabelPath)
	return err == nil
}

// Check returns a message if root doesn't look like a Slot card.
func Check(root string) error {
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return fmt.Errorf("%s isn't a folder you can open", root)
	}
	if !isDir(filepath.Join(root, "Games")) || !isDir(filepath.Join(root, "System")) {
		return errors.New("this doesn't have Slot's Games and System folders, so it doesn't look like a Slot SD card")
	}
	return nil
}

// LooksLikeSlot is a quick check used when listing drives.
func LooksLikeSlot(root string) bool {
	return isDir(filepath.Join(root, "Games")) && isDir(filepath.Join(root, "System"))
}

// IsSlotCard is the strict test: Slot's folders and Slot itself
// (System/slot). A PC drive that happens to have Games and System folders
// doesn't pass.
func IsSlotCard(root string) bool { return LooksLikeSlot(root) && HasSlotBinary(root) }

// SafeToPickAutomatically reports whether SlotDrop may open root without
// being asked: Slot is on it, it's a removable drive, and it isn't the
// drive Windows runs from. Size is no guide, since 512 GB SD cards exist.
func SafeToPickAutomatically(root string) bool {
	return IsSlotCard(root) && IsRemovable(root) && !IsSystemDrive(root)
}

// HasSlotBinary reports whether Slot itself is installed (System/slot).
func HasSlotBinary(root string) bool {
	st, err := os.Stat(filepath.Join(root, "System", "slot"))
	return err == nil && !st.IsDir()
}

// HasBIOS reports whether the optional GBA BIOS is present.
func HasBIOS(root string) bool {
	_, err := os.Stat(filepath.Join(root, "BIOS", "gba_bios.bin"))
	return err == nil
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// LabelPath is where Slot expects a game's label.
func LabelPath(root string, p *platforms.Platform, stem string) string {
	return filepath.Join(root, "Labels", p.Key, stem+".png")
}

// GamesFolder is where a platform's games go.
func GamesFolder(root string, p *platforms.Platform) string {
	return filepath.Join(root, "Games", p.Key)
}

// FindGames lists a platform's games on the card, sorted by name.
func FindGames(root string, p *platforms.Platform) []*Game {
	entries, err := os.ReadDir(GamesFolder(root, p))
	if err != nil {
		return nil
	}
	var games []*Game
	for _, e := range entries {
		name := e.Name()
		ext := filepath.Ext(name)
		if e.IsDir() || strings.HasPrefix(name, ".") || !p.HasExtension(ext) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		stem := strings.TrimSuffix(name, ext)
		games = append(games, &Game{
			Platform: p, Stem: stem,
			RomPath:   filepath.Join(GamesFolder(root, p), name),
			LabelPath: LabelPath(root, p, stem),
			Size:      info.Size(),
		})
	}
	sort.Slice(games, func(i, j int) bool { return strings.ToLower(games[i].Stem) < strings.ToLower(games[j].Stem) })
	return games
}

var (
	badChars     = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
	reservedName = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[0-9]|lpt[0-9])(\..*)?$`)
)

// SafeFileName removes characters Windows and FAT32/exFAT cards don't allow,
// and anything that could step outside the games folder.
func SafeFileName(name string) string {
	name = badChars.ReplaceAllString(name, "")
	name = strings.TrimRight(strings.TrimSpace(name), ". ")
	name = strings.TrimLeft(name, ". ")
	if reservedName.MatchString(name) {
		name = "_" + name
	}
	if len(name) > 200 {
		name = name[:200]
	}
	if name == "" {
		return "game"
	}
	return name
}

// WriteFileSafely writes through a .part file and renames it, so a failed
// copy never looks like a finished game or label.
func WriteFileSafely(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	part := path + ".part"
	if err := os.WriteFile(part, data, 0o644); err != nil {
		os.Remove(part)
		return err
	}
	if err := os.Rename(part, path); err != nil {
		os.Remove(part)
		return err
	}
	return nil
}

// ---- cart_shell.ini -----------------------------------------------------

// Shell is a cartridge's plastic colour and finish.
type Shell struct {
	Colour string `json:"colour"` // hex without #, e.g. "c0282c"
	Finish string `json:"finish"` // "solid", "clear" or "glitter", the same choices as Cart Studio
	// Outline is Cart Studio's cartridge shape ("auto", "notched" or
	// "rounded"). It's only read, for the preview; SetShells keeps whatever
	// the line already had.
	Outline string `json:"outline,omitempty"`
}

// shellFile is where SlotDrop writes shell colours.
func shellFile(root string) string { return filepath.Join(root, "Labels", "cart_shell.txt") }

// readShellText returns a folder's cart_shell.txt, or its old cart_shell.ini.
func readShellText(dir string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(dir, "cart_shell.txt"))
	if err == nil {
		return data, nil
	}
	return os.ReadFile(filepath.Join(dir, "cart_shell.ini"))
}

// ReadShells maps game stem to its shell: Slot's own defaults from Config
// (or System), then the Labels file on top.
func ReadShells(root string) map[string]Shell {
	out := map[string]Shell{}
	system, err := readShellText(filepath.Join(root, "Config"))
	if err != nil {
		system, _ = readShellText(filepath.Join(root, "System"))
	}
	labels, _ := readShellText(filepath.Join(root, "Labels"))
	parseShells(system, out)
	parseShells(labels, out)
	return out
}

func parseShells(data []byte, out map[string]Shell) {
	for _, line := range strings.Split(string(data), "\n") {
		i := strings.LastIndex(line, "=")
		if i < 0 {
			continue
		}
		parts := strings.Fields(line[i+1:])
		if len(parts) >= 3 {
			out[strings.TrimSpace(line[:i])] = Shell{Colour: strings.ToLower(parts[1]), Finish: strings.ToLower(parts[2]), Outline: strings.ToLower(parts[0])}
		}
	}
}

var hexColour = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)

// SetShells sets the shell for each game, replacing any line it already has
// and keeping whatever mode word Slot put first.
func SetShells(root string, shells map[string]Shell) error {
	if len(shells) == 0 {
		return nil
	}
	for stem, s := range shells {
		if !hexColour.MatchString(s.Colour) || (s.Finish != "solid" && s.Finish != "clear" && s.Finish != "glitter") {
			return fmt.Errorf("%q isn't a valid shell colour for %s", s.Colour+" "+s.Finish, stem)
		}
	}
	var lines []string
	if data, err := readShellText(filepath.Join(root, "Labels")); err == nil {
		lines = strings.Split(strings.TrimRight(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"), "\n")
	}
	remaining := map[string]Shell{}
	for k, v := range shells {
		remaining[k] = v
	}
	for i, line := range lines {
		eq := strings.LastIndex(line, "=")
		if eq < 0 {
			continue
		}
		name := strings.TrimSpace(line[:eq])
		s, ok := remaining[name]
		if !ok {
			continue
		}
		delete(remaining, name)
		mode := "auto"
		if f := strings.Fields(line[eq+1:]); len(f) > 0 {
			mode = f[0]
		}
		lines[i] = fmt.Sprintf("%s = %s %s %s", name, mode, strings.ToLower(s.Colour), s.Finish)
	}
	stems := make([]string, 0, len(remaining))
	for k := range remaining {
		stems = append(stems, k)
	}
	sort.Strings(stems)
	for _, stem := range stems {
		s := remaining[stem]
		lines = append(lines, fmt.Sprintf("%s = auto %s %s", stem, strings.ToLower(s.Colour), s.Finish))
	}
	return writeShellFile(root, lines)
}

// writeShellFile writes Labels/cart_shell.txt. An old cart_shell.ini next
// to it is renamed to cart_shell.ini.bak: its lines are already in the
// .txt (read from it when there was no .txt yet), and this way Slot has
// nothing left to migrate over the new file on its next boot.
func writeShellFile(root string, lines []string) error {
	if err := WriteFileSafely(shellFile(root), []byte(strings.Join(lines, "\n")+"\n")); err != nil {
		return err
	}
	old := filepath.Join(root, "Labels", "cart_shell.ini")
	if _, err := os.Stat(old); err == nil {
		if err := os.Rename(old, old+".bak"); err != nil {
			return fmt.Errorf("saved the colours, but couldn't move the old cart_shell.ini aside: %v", err)
		}
	}
	return nil
}

// RemoveShell drops a game's line from the Labels shell file, if it has one.
func RemoveShell(root, stem string) error {
	data, err := readShellText(filepath.Join(root, "Labels"))
	if err != nil {
		return nil
	}
	var kept []string
	removed := false
	for _, line := range strings.Split(strings.TrimRight(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"), "\n") {
		if eq := strings.LastIndex(line, "="); eq >= 0 && strings.TrimSpace(line[:eq]) == stem {
			removed = true
			continue
		}
		kept = append(kept, line)
	}
	if !removed {
		return nil
	}
	return writeShellFile(root, kept)
}
