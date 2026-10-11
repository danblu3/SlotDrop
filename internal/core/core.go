// Package core is everything the app does, without the window: it ties the
// SD card, RomM and label making together. The window (package main) only
// calls into this.
package core

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"slotlabelmaker/internal/label"
	"slotlabelmaker/internal/match"
	"slotlabelmaker/internal/platforms"
	"slotlabelmaker/internal/romm"
	"slotlabelmaker/internal/sdcard"
)

// Core holds the open card, the RomM connection and caches.
type Core struct {
	mu      sync.Mutex
	root    string
	client  *romm.Client
	roms    map[string][]*romm.Rom // platform key -> games
	byID    map[int]*romm.Rom
	romPlat map[int]*platforms.Platform
	matches map[string]match.Result // "GBA/stem" -> match
	manual  map[string]int          // "GBA/stem" -> rom id you picked by hand

	imgMu    sync.Mutex
	carts    map[int]*cartImage
	cartKeys []int
}

type cartImage struct {
	raw []byte
	img *image.NRGBA
	err error
}

// New makes an empty Core.
func New() *Core {
	return &Core{carts: map[int]*cartImage{}, manual: map[string]int{}}
}

func key(p *platforms.Platform, stem string) string { return p.Key + "/" + stem }

// ---- SD card -------------------------------------------------------------

// PlatformCount is how many games one platform has on the card.
type PlatformCount struct {
	Key    string `json:"key"`
	Games  int    `json:"games"`
	Labels int    `json:"labels"`
}

// CardInfo describes the open card.
type CardInfo struct {
	Path          string          `json:"path"`
	SlotInstalled bool            `json:"slotInstalled"`
	BIOS          bool            `json:"bios"`
	CanEject      bool            `json:"canEject"`
	Games         int             `json:"games"`
	MissingLabels int             `json:"missingLabels"`
	GamesBytes    int64           `json:"gamesBytes"`
	TotalBytes    uint64          `json:"totalBytes"`
	FreeBytes     uint64          `json:"freeBytes"`
	Platforms     []PlatformCount `json:"platforms"`
}

// OpenCard checks root is a Slot card and makes it the card we work on.
func (c *Core) OpenCard(root string) (*CardInfo, error) {
	root = filepath.Clean(root)
	if err := sdcard.Check(root); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.root != root {
		c.matches = nil
		c.manual = map[string]int{}
	}
	c.root = root
	c.mu.Unlock()
	return c.Card()
}

// Root is the open card's folder, or "".
func (c *Core) Root() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.root
}

// Card describes the open card as it is right now.
func (c *Core) Card() (*CardInfo, error) {
	root := c.Root()
	if root == "" {
		return nil, errors.New("no SD card chosen yet")
	}
	info := &CardInfo{Path: root, SlotInstalled: sdcard.HasSlotBinary(root), BIOS: sdcard.HasBIOS(root), CanEject: sdcard.CanEject(root)}
	for _, p := range platforms.All {
		pc := PlatformCount{Key: p.Key}
		for _, g := range sdcard.FindGames(root, p) {
			pc.Games++
			info.GamesBytes += g.Size
			if g.HasLabel() {
				pc.Labels++
			}
		}
		info.Games += pc.Games
		info.MissingLabels += pc.Games - pc.Labels
		info.Platforms = append(info.Platforms, pc)
	}
	info.TotalBytes, info.FreeBytes, _ = sdcard.Space(root)
	return info, nil
}

// ---- RomM ----------------------------------------------------------------

// Connect logs in to RomM and loads the GBA, GB and GBC libraries.
func (c *Core) Connect(url, username, password, token string) error {
	client, err := romm.New(url, username, password, token)
	if err != nil {
		return err
	}
	all, err := client.Platforms()
	if err != nil {
		return err
	}
	roms := map[string][]*romm.Rom{}
	byID := map[int]*romm.Rom{}
	romPlat := map[int]*platforms.Platform{}
	for _, p := range platforms.All {
		for _, id := range romm.PlatformIDs(p, all) {
			list, err := client.Roms(id)
			if err != nil {
				return err
			}
			for i := range list {
				r := &list[i]
				roms[p.Key] = append(roms[p.Key], r)
				byID[r.ID] = r
				romPlat[r.ID] = p
			}
		}
	}
	c.mu.Lock()
	c.client, c.roms, c.byID, c.romPlat, c.matches = client, roms, byID, romPlat, nil
	c.mu.Unlock()
	c.imgMu.Lock()
	c.carts, c.cartKeys = map[int]*cartImage{}, nil
	c.imgMu.Unlock()
	return nil
}

// Disconnect forgets the RomM connection.
func (c *Core) Disconnect() {
	c.mu.Lock()
	c.client, c.roms, c.byID, c.romPlat, c.matches = nil, nil, nil, nil, nil
	c.mu.Unlock()
}

// Client is the RomM connection, or nil.
func (c *Core) Client() *romm.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}

// Rom finds a RomM game by id.
func (c *Core) Rom(id int) (*romm.Rom, *platforms.Platform) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.byID[id], c.romPlat[id]
}

// ---- the library -----------------------------------------------------------

// Game is one tile in the app: a game on the card, in RomM, or both.
type Game struct {
	Platform string        `json:"platform"`
	Title    string        `json:"title"`
	File     string        `json:"file"`
	Size     int64         `json:"size"`
	RomID    int           `json:"romId"`
	HasCover bool          `json:"hasCover"`
	HasCart  bool          `json:"hasCart"`
	Regions  []string      `json:"regions,omitempty"`
	Added    string        `json:"added,omitempty"` // when it was added to RomM
	OnCard   bool          `json:"onCard"`
	Stem     string        `json:"stem,omitempty"` // name on the card
	HasLabel bool          `json:"hasLabel"`
	Shell    *sdcard.Shell `json:"shell,omitempty"`
	Match    string        `json:"match,omitempty"` // how the card game was paired with RomM
}

func (c *Core) ensureMatches() {
	// Called with c.mu held.
	if c.matches != nil || c.root == "" || c.roms == nil {
		return
	}
	c.matches = map[string]match.Result{}
	for _, p := range platforms.All {
		for stem, res := range match.Games(sdcard.FindGames(c.root, p), c.roms[p.Key]) {
			c.matches[key(p, stem)] = res
		}
	}
}

func (c *Core) romGame(r *romm.Rom, p *platforms.Platform) Game {
	return Game{
		Platform: p.Key, Title: r.DisplayName(), File: r.FsName, Size: r.FsSizeBytes, RomID: r.ID,
		HasCover: r.PathCoverSmall != "" || r.PathCoverLarge != "" || r.URLCover != "" ||
			(r.SS != nil && (r.SS.Box2DPath != "" || r.SS.Box2DURL != "")),
		HasCart: r.SS != nil && (r.SS.PhysicalPath != "" || r.SS.PhysicalURL != ""),
		Regions: r.Regions,
		Added:   r.CreatedAt,
	}
}

// Library lists every game on the card and every GBA/GB/GBC game in RomM,
// with card games first.
func (c *Core) Library() []Game {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureMatches()
	var out []Game
	used := map[int]bool{}
	shells := map[string]sdcard.Shell{}
	if c.root != "" {
		shells = sdcard.ReadShells(c.root)
	}
	for _, p := range platforms.All {
		if c.root == "" {
			break
		}
		for _, g := range sdcard.FindGames(c.root, p) {
			item := Game{Platform: p.Key, Title: g.Stem, File: filepath.Base(g.RomPath), Size: g.Size,
				OnCard: true, Stem: g.Stem, HasLabel: g.HasLabel()}
			if s, ok := shells[g.Stem]; ok {
				s := s
				item.Shell = &s
			}
			res := c.matches[key(p, g.Stem)]
			if id, ok := c.manual[key(p, g.Stem)]; ok {
				res = match.Result{Rom: c.byID[id], How: "picked"}
			}
			if res.Rom != nil {
				rg := c.romGame(res.Rom, p)
				item.RomID, item.HasCover, item.HasCart, item.Regions = rg.RomID, rg.HasCover, rg.HasCart, rg.Regions
				item.Title = rg.Title
				item.Match = res.How
				used[res.Rom.ID] = true
			}
			out = append(out, item)
		}
	}
	var rest []Game
	for _, p := range platforms.All {
		for _, r := range c.roms[p.Key] {
			if !used[r.ID] && !r.MissingFromFS { // RomM can't send a game whose file is gone
				rest = append(rest, c.romGame(r, p))
			}
		}
	}
	// A to Z by name, ignoring punctuation, so "Pokemon - Gaia" sits between
	// "Pokemon Emerald" and "Pokemon Radical Red".
	sortKey := func(g Game) string { return match.Normalise(g.Title) + "\x00" + strings.ToLower(g.Title) }
	sort.SliceStable(rest, func(i, j int) bool { return sortKey(rest[i]) < sortKey(rest[j]) })
	return append(out, rest...)
}

// SetMatch pairs a card game with a different RomM game (0 = no match).
func (c *Core) SetMatch(platform, stem string, romID int) {
	p := platforms.ByKey(platform)
	if p == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.manual[key(p, stem)] = romID
}

// ---- pictures ------------------------------------------------------------

// thumbSlots limits how many tile pictures download at once.
var thumbSlots = make(chan struct{}, 6)

// Thumb is a small picture for a game's tile: the cartridge photo if RomM
// has one, otherwise its box art. Thumbs are kept on disk so the library
// opens quickly next time.
func (c *Core) Thumb(romID int) ([]byte, error) {
	r, _ := c.Rom(romID)
	client := c.Client()
	if r == nil || client == nil {
		return nil, errors.New("unknown game")
	}
	cacheFile := ""
	if dir, err := os.UserCacheDir(); err == nil {
		server := fmt.Sprintf("%x", sha256.Sum256([]byte(client.BaseURL)))[:12]
		cacheFile = filepath.Join(dir, "SlotDrop", "thumbs", server, fmt.Sprintf("%d.png", romID))
		if data, err := os.ReadFile(cacheFile); err == nil {
			return data, nil
		}
	}
	thumbSlots <- struct{}{}
	defer func() { <-thumbSlots }()
	lastErr := errors.New("RomM has no picture for this game")
	for _, u := range client.ArtURLs(r) {
		raw, err := client.Image(u)
		if err != nil {
			lastErr = err
			continue
		}
		img, err := label.Decode(raw)
		if err != nil {
			lastErr = err
			continue
		}
		data, err := label.EncodePNG(label.Shrink(img, 360))
		if err != nil {
			return nil, err
		}
		if cacheFile != "" {
			_ = sdcard.WriteFileSafely(cacheFile, data)
		}
		return data, nil
	}
	return nil, lastErr
}

// CoverImage downloads a game's box art.
func (c *Core) CoverImage(romID int) ([]byte, error) {
	r, _ := c.Rom(romID)
	client := c.Client()
	if r == nil || client == nil {
		return nil, errors.New("unknown game")
	}
	lastErr := errors.New("no cover")
	for _, u := range client.ArtURLs(r)[len(client.CartridgeURLs(r)):] {
		data, err := client.Image(u)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// CartImage downloads (once) a game's cartridge photo.
func (c *Core) CartImage(romID int) ([]byte, *image.NRGBA, error) {
	c.imgMu.Lock()
	if ci, ok := c.carts[romID]; ok {
		c.imgMu.Unlock()
		return ci.raw, ci.img, ci.err
	}
	c.imgMu.Unlock()

	r, _ := c.Rom(romID)
	client := c.Client()
	if r == nil || client == nil {
		return nil, nil, errors.New("unknown game")
	}
	ci := &cartImage{err: errors.New("RomM has no cartridge photo for this game")}
	for _, u := range client.CartridgeURLs(r) {
		raw, err := client.Image(u)
		if err != nil {
			ci.err = err
			continue
		}
		img, err := label.Decode(raw)
		if err != nil {
			ci.err = err
			continue
		}
		ci = &cartImage{raw: raw, img: img}
		break
	}
	c.imgMu.Lock()
	c.carts[romID] = ci
	c.cartKeys = append(c.cartKeys, romID)
	if len(c.cartKeys) > 40 { // keep memory in check
		delete(c.carts, c.cartKeys[0])
		c.cartKeys = c.cartKeys[1:]
	}
	c.imgMu.Unlock()
	return ci.raw, ci.img, ci.err
}

// CardLabel reads a label PNG from the card.
func (c *Core) CardLabel(platform, stem string) ([]byte, error) {
	p := platforms.ByKey(platform)
	root := c.Root()
	if p == nil || root == "" || sdcard.SafeFileName(stem) != stem {
		return nil, errors.New("unknown label")
	}
	return os.ReadFile(sdcard.LabelPath(root, p, stem))
}

// ---- labels --------------------------------------------------------------

// LabelPlan is where the label sits on a game's cartridge photo.
type LabelPlan struct {
	Width     int          `json:"width"`
	Height    int          `json:"height"`
	Box       label.Box    `json:"box"`
	How       string       `json:"how"`
	Aspect    float64      `json:"aspect"` // label width / height Slot uses
	Suggested sdcard.Shell `json:"suggested"`
	// CartColour is the plastic's colour in the photo, if it's coloured.
	CartColour string `json:"cartColour,omitempty"`
}

// PlanLabel finds the label on a game's cartridge photo.
func (c *Core) PlanLabel(romID int) (*LabelPlan, error) {
	_, p := c.Rom(romID)
	_, img, err := c.CartImage(romID)
	if err != nil {
		return nil, err
	}
	box, how := label.FindBox(img, p)
	plan := &LabelPlan{
		Width: img.Bounds().Dx(), Height: img.Bounds().Dy(), Box: box, How: how,
		Aspect:    float64(p.LabelW) / float64(p.LabelH),
		Suggested: sdcard.Shell{Colour: label.ShellColour(label.Crop(img, box, p)), Finish: "solid"},
	}
	if how == "template" || how == "detected" {
		plan.CartColour, _ = label.PlasticColour(img, box)
	}
	return plan, nil
}

// SuggestShell picks a shell colour for a label box.
func (c *Core) SuggestShell(romID int, box label.Box) (string, error) {
	_, p := c.Rom(romID)
	_, img, err := c.CartImage(romID)
	if err != nil {
		return "", err
	}
	box, err = label.Clamp(box, img)
	if err != nil {
		return "", err
	}
	return label.ShellColour(label.Crop(img, box, p)), nil
}

// SaveLabel writes a card game's label (and shell colour, if given).
// A nil box means "find it automatically".
func (c *Core) SaveLabel(platform, stem string, romID int, box *label.Box, shell *sdcard.Shell) error {
	p := platforms.ByKey(platform)
	root := c.Root()
	if p == nil || root == "" {
		return errors.New("open the SD card first")
	}
	if sdcard.SafeFileName(stem) != stem {
		return fmt.Errorf("%q isn't a game on the card", stem)
	}
	if _, err := os.Stat(filepath.Join(sdcard.GamesFolder(root, p))); err != nil {
		return err
	}
	_, img, err := c.CartImage(romID)
	if err != nil {
		return err
	}
	var b label.Box
	how := "chosen"
	if box == nil {
		b, how = label.FindBox(img, p)
	} else if b, err = label.Clamp(*box, img); err != nil {
		return err
	}
	lbl := label.Crop(img, b, p)
	data, err := label.EncodePNG(lbl)
	if err != nil {
		return err
	}
	if err := sdcard.WriteFileSafely(sdcard.LabelPath(root, p, stem), data); err != nil {
		return fmt.Errorf("couldn't write the label: %v", err)
	}
	if shell == nil {
		return giveDefaultShell(root, stem, img, b, how)
	}
	return sdcard.SetShells(root, map[string]sdcard.Shell{stem: *shell})
}

// DefaultShell is the grey of a real GBA cartridge, the same as Cart
// Studio's "Automatic". Slot draws a game with no shell line in plain black,
// so new games get this written out.
var DefaultShell = sdcard.Shell{Colour: "3a3431", Finish: "solid"}

// giveDefaultShell gives a game with no shell yet the colour of the
// cartridge in its photo (FireRed's red, LeafGreen's green), or
// DefaultShell when the photo's cart is grey or there's no cart photo. A
// colour already set (here or in Cart Studio) is left alone.
func giveDefaultShell(root, stem string, img *image.NRGBA, box label.Box, how string) error {
	if _, ok := sdcard.ReadShells(root)[stem]; ok {
		return nil
	}
	shell := DefaultShell
	if how == "template" || how == "detected" {
		if hex, ok := label.PlasticColour(img, box); ok {
			shell.Colour = hex
		}
	}
	return sdcard.SetShells(root, map[string]sdcard.Shell{stem: shell})
}

// LabelFromImage makes a card game's label from a picture you chose. A whole
// cartridge photo has its label found and cut out; anything else is used as
// the label, trimmed to the label's shape.
func (c *Core) LabelFromImage(platform, stem string, data []byte) error {
	p := platforms.ByKey(platform)
	root := c.Root()
	if p == nil || root == "" {
		return errors.New("open the SD card first")
	}
	if sdcard.SafeFileName(stem) != stem {
		return fmt.Errorf("%q isn't a game on the card", stem)
	}
	img, err := label.Decode(data)
	if err != nil {
		return err
	}
	box, how := label.FindBox(img, p)
	if how != "template" && how != "detected" {
		box = label.FitBox(img, float64(p.LabelW)/float64(p.LabelH))
	}
	out, err := label.EncodePNG(label.Crop(img, box, p))
	if err != nil {
		return err
	}
	if err := sdcard.WriteFileSafely(sdcard.LabelPath(root, p, stem), out); err != nil {
		return fmt.Errorf("couldn't write the label: %v", err)
	}
	return giveDefaultShell(root, stem, img, box, how)
}

// SetShell changes just a game's shell colour.
func (c *Core) SetShell(stem string, shell sdcard.Shell) error {
	root := c.Root()
	if root == "" {
		return errors.New("open the SD card first")
	}
	if sdcard.SafeFileName(stem) != stem {
		return fmt.Errorf("%q isn't a game on the card", stem)
	}
	return sdcard.SetShells(root, map[string]sdcard.Shell{stem: shell})
}

// RemoveGame deletes a game and its label from the card, and its line in
// cart_shell.txt. Saves and save states are left alone. file is the game's
// file name, since GBC can hold both "Tetris.gb" and "Tetris.gbc"; the label
// and colour are only removed when no game of that name is left.
func (c *Core) RemoveGame(platform, stem, file string) error {
	p := platforms.ByKey(platform)
	root := c.Root()
	if p == nil || root == "" {
		return errors.New("open the SD card first")
	}
	var game *sdcard.Game
	twin := false
	for _, g := range sdcard.FindGames(root, p) {
		if g.Stem != stem {
			continue
		}
		if filepath.Base(g.RomPath) == file {
			game = g
		} else {
			twin = true
		}
	}
	if game == nil {
		return fmt.Errorf("%s isn't on the card", stem)
	}
	if err := os.Remove(game.RomPath); err != nil {
		return fmt.Errorf("couldn't delete %s: %v", filepath.Base(game.RomPath), err)
	}
	if twin { // another file of the same name still uses the label and colour
		c.mu.Lock()
		c.matches = nil
		c.mu.Unlock()
		return nil
	}
	if err := os.Remove(game.LabelPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("the game is gone but its label couldn't be deleted: %v", err)
	}
	if err := sdcard.RemoveShell(root, stem); err != nil {
		return err
	}
	c.mu.Lock()
	c.matches = nil
	delete(c.manual, key(p, stem))
	c.mu.Unlock()
	return nil
}

// Eject safely ejects the card so it can be pulled out, then forgets it.
func (c *Core) Eject() error {
	root := c.Root()
	if root == "" {
		return errors.New("no SD card chosen")
	}
	if err := sdcard.Eject(root); err != nil {
		return err
	}
	c.mu.Lock()
	c.root, c.matches = "", nil
	c.manual = map[string]int{}
	c.mu.Unlock()
	return nil
}

// ---- adding games ----------------------------------------------------------

// CardStem is the name a RomM game gets on the card (and so its label too).
func CardStem(r *romm.Rom) string {
	name := r.FsNameNoExt
	if name == "" {
		name = r.Name
	}
	if name == "" {
		name = fmt.Sprintf("rom-%d", r.ID)
	}
	return sdcard.SafeFileName(name)
}

// Added is what AddGame did.
type Added struct {
	Stem      string `json:"stem"`
	Label     bool   `json:"label"`
	LabelNote string `json:"labelNote,omitempty"`
}

// AddGame downloads a game from RomM onto the card, with its label.
// stage is told what's happening: "download" (with a 0-1 fraction), "label", "done".
func (c *Core) AddGame(romID int, stage func(step string, frac float64)) (*Added, error) {
	r, p := c.Rom(romID)
	client := c.Client()
	root := c.Root()
	switch {
	case root == "":
		return nil, errors.New("open the SD card first")
	case r == nil || client == nil:
		return nil, errors.New("connect to RomM first")
	}
	stem := CardStem(r)
	for _, g := range sdcard.FindGames(root, p) {
		if strings.EqualFold(g.Stem, stem) {
			return nil, fmt.Errorf("%s is already on the card", stem)
		}
	}
	if _, free, err := sdcard.Space(root); err == nil && r.FsSizeBytes > 0 && uint64(r.FsSizeBytes)+(8<<20) > free {
		return nil, fmt.Errorf("the card is full: %s needs %d MB and there's %d MB free", r.DisplayName(), r.FsSizeBytes>>20, free>>20)
	}

	data, err := client.DownloadRom(r, func(done, total int64) {
		if total <= 0 {
			total = r.FsSizeBytes
		}
		if total > 0 {
			stage("download", min(1, float64(done)/float64(total)))
		}
	})
	if err != nil {
		return nil, err
	}
	game, ext, err := Unpack(data, r, p)
	if err != nil {
		return nil, err
	}
	romPath := filepath.Join(sdcard.GamesFolder(root, p), stem+ext)
	if err := sdcard.WriteFileSafely(romPath, game); err != nil {
		return nil, fmt.Errorf("couldn't copy the game to the card: %v", err)
	}
	c.mu.Lock()
	c.matches = nil // the card changed
	c.manual[key(p, stem)] = romID
	c.mu.Unlock()

	stage("label", 1)
	added := &Added{Stem: stem}
	if err := c.SaveLabel(p.Key, stem, romID, nil, nil); err != nil {
		added.LabelNote = err.Error()
	} else {
		added.Label = true
	}
	stage("done", 1)
	return added, nil
}

// MissingLabels makes a label for every card game that has none and is
// matched to a RomM game with a cartridge photo. It returns how many it made
// and why any others couldn't be made.
func (c *Core) MissingLabels(progress func(done, total int)) (int, []string) {
	var todo []Game
	for _, g := range c.Library() {
		if g.OnCard && !g.HasLabel {
			todo = append(todo, g)
		}
	}
	made := 0
	var problems []string
	for i, g := range todo {
		progress(i, len(todo))
		switch {
		case g.RomID == 0:
			problems = append(problems, g.Stem+": not found in RomM")
		case !g.HasCart:
			problems = append(problems, g.Stem+": RomM has no cartridge photo")
		default:
			if err := c.SaveLabel(g.Platform, g.Stem, g.RomID, nil, nil); err != nil {
				problems = append(problems, g.Stem+": "+err.Error())
			} else {
				made++
			}
		}
	}
	progress(len(todo), len(todo))
	return made, problems
}

// CropPNG cuts the label out of a game's cartridge photo, at box or, when
// box is nil, where it's found automatically. It's what the preview shows.
func (c *Core) CropPNG(romID int, box *label.Box) ([]byte, error) {
	_, p := c.Rom(romID)
	_, img, err := c.CartImage(romID)
	if err != nil {
		return nil, err
	}
	b, _ := label.FindBox(img, p)
	if box != nil {
		if b, err = label.Clamp(*box, img); err != nil {
			return nil, err
		}
	}
	return label.EncodePNG(label.Crop(img, b, p))
}
