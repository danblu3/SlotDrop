package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"slotlabelmaker/internal/core"
	"slotlabelmaker/internal/label"
	"slotlabelmaker/internal/romm"
	"slotlabelmaker/internal/sdcard"
	"slotlabelmaker/internal/secret"
	"slotlabelmaker/internal/settings"
)

// App is what the window can call. Every exported method shows up in the
// page as window.go.main.App.<Method>.
type App struct {
	ctx  context.Context
	core *core.Core

	mu       sync.Mutex
	settings settings.Settings
}

func NewApp() *App { return &App{core: core.New(), settings: settings.Load()} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// friendly turns any error into something safe to show.
func friendly(err error) error {
	if err == nil {
		return nil
	}
	var re *romm.Error
	if errors.As(err, &re) {
		return re
	}
	msg := err.Error()
	if msg != "" {
		msg = strings.ToUpper(msg[:1]) + msg[1:]
	}
	return errors.New(msg)
}

// ---- start up ----------------------------------------------------------------

// Start is what the window needs when it opens.
type Start struct {
	RommURL    string         `json:"rommUrl"`
	Username   string         `json:"username"`
	UseToken   bool           `json:"useToken"`
	SavedLogin bool           `json:"savedLogin"`
	CanSave    bool           `json:"canSave"`
	LastCard   string         `json:"lastCard"`
	Drives     []sdcard.Drive `json:"drives"`
	Card       *core.CardInfo `json:"card"`
}

// Start opens the last card if it's plugged in, and says what's remembered.
func (a *App) Start() Start {
	a.mu.Lock()
	s := a.settings
	a.mu.Unlock()
	st := Start{
		RommURL: s.RommURL, Username: s.Username, UseToken: s.UseToken,
		SavedLogin: len(s.Login) > 0, CanSave: secret.Available(), LastCard: s.SDCard,
		Drives: sdcard.Drives(),
	}
	if a.core.Root() != "" {
		st.Card, _ = a.core.Card()
	} else if s.SDCard != "" && sdcard.SafeToPickAutomatically(s.SDCard) {
		// Last time's card, but only if it's still a Slot card: with the
		// card out, Windows can hand its letter to another drive.
		if info, err := a.core.OpenCard(s.SDCard); err == nil {
			st.Card = info
		}
	}
	if st.Card == nil {
		// Exactly one Slot card plugged in? Use it.
		var found []sdcard.Drive
		for _, d := range st.Drives {
			if d.IsSlot && d.Removable && !d.System {
				found = append(found, d)
			}
		}
		if len(found) == 1 {
			if info, err := a.OpenCard(found[0].Path); err == nil {
				st.Card = info
			}
		}
	}
	return st
}

// ---- SD card -----------------------------------------------------------------

// Drives lists drive letters, Slot cards first.
func (a *App) Drives() []sdcard.Drive { return sdcard.Drives() }

// BrowseForCard asks for a folder. Empty means cancelled.
func (a *App) BrowseForCard() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Choose your Slot SD card"})
}

// CardWarning says why path might not be the Slot card, or "" if it looks
// right. The window asks before opening anything it warns about.
func (a *App) CardWarning(path string) string {
	switch {
	case sdcard.IsSystemDrive(path):
		return "This is the drive Windows runs from, not an SD card. Only use it if you really keep a copy of your card here."
	case !sdcard.HasSlotBinary(path):
		return "Slot isn't installed here (there's no System\\slot), so this doesn't look like your Slot card."
	case !sdcard.IsRemovable(path):
		return "This isn't a removable drive, so it's probably not your SD card."
	}
	return ""
}

// OpenCard makes path the card we work on and remembers it.
func (a *App) OpenCard(path string) (*core.CardInfo, error) {
	info, err := a.core.OpenCard(path)
	if err != nil {
		return nil, friendly(err)
	}
	a.mu.Lock()
	a.settings.SDCard = info.Path
	s := a.settings
	a.mu.Unlock()
	_ = settings.Save(s)
	return info, nil
}

// Card is the open card's counts and space, freshly read.
func (a *App) Card() (*core.CardInfo, error) {
	info, err := a.core.Card()
	return info, friendly(err)
}

// ---- RomM --------------------------------------------------------------------

// Connect logs in to RomM. secretText is the password, or the API token when
// useToken is set. With remember, the login is saved encrypted for next time.
func (a *App) Connect(rommURL, username, secretText string, useToken, remember bool) error {
	user, pass, token := username, secretText, ""
	if useToken {
		user, pass, token = "", "", secretText
	}
	if err := a.core.Connect(rommURL, user, pass, token); err != nil {
		return friendly(err)
	}
	a.mu.Lock()
	a.settings.RommURL = a.core.Client().BaseURL
	a.settings.Username = username
	a.settings.UseToken = useToken
	a.settings.Login = nil
	if remember && secret.Available() {
		if enc, err := secret.Protect([]byte(secretText)); err == nil {
			a.settings.Login = enc
		}
	}
	s := a.settings
	a.mu.Unlock()
	return friendly(settings.Save(s))
}

// ConnectSaved logs in with the remembered login.
func (a *App) ConnectSaved() error {
	a.mu.Lock()
	s := a.settings
	a.mu.Unlock()
	if len(s.Login) == 0 {
		return errors.New("There's no saved login.")
	}
	plain, err := secret.Unprotect(s.Login)
	if err != nil {
		return errors.New("The saved login couldn't be unlocked. Please sign in again.")
	}
	user, pass, token := s.Username, string(plain), ""
	if s.UseToken {
		user, pass, token = "", "", string(plain)
	}
	return friendly(a.core.Connect(s.RommURL, user, pass, token))
}

// SignOut disconnects and forgets the saved password or token.
func (a *App) SignOut() error {
	a.core.Disconnect()
	a.mu.Lock()
	a.settings.Login = nil
	s := a.settings
	a.mu.Unlock()
	return friendly(settings.Save(s))
}

// Connection says who we're connected to.
type Connection struct {
	Connected bool   `json:"connected"`
	URL       string `json:"url"`
	Secure    bool   `json:"secure"`
}

func (a *App) Connection() Connection {
	c := a.core.Client()
	if c == nil {
		return Connection{}
	}
	return Connection{true, c.BaseURL, c.IsHTTPS()}
}

// ---- games -------------------------------------------------------------------

// Library lists the card's games and RomM's GBA, GB and GBC games.
func (a *App) Library() []core.Game { return a.core.Library() }

// AddGame copies a game from RomM to the card, with its label. Progress is
// sent to the page as "add-progress" events.
func (a *App) AddGame(romID int) (*core.Added, error) {
	added, err := a.core.AddGame(romID, func(step string, frac float64) {
		runtime.EventsEmit(a.ctx, "add-progress", map[string]any{"romId": romID, "step": step, "frac": frac})
	})
	return added, friendly(err)
}

// MakeMissingLabels labels every card game that has none.
type MissingResult struct {
	Made     int      `json:"made"`
	Problems []string `json:"problems"`
}

func (a *App) MakeMissingLabels() MissingResult {
	made, problems := a.core.MissingLabels(func(done, total int) {
		runtime.EventsEmit(a.ctx, "labels-progress", map[string]int{"done": done, "total": total})
	})
	return MissingResult{made, problems}
}

// PlanLabel finds the label on a game's cartridge photo.
func (a *App) PlanLabel(romID int) (*core.LabelPlan, error) {
	p, err := a.core.PlanLabel(romID)
	return p, friendly(err)
}

// SuggestShell picks a shell colour for a label box.
func (a *App) SuggestShell(romID int, box label.Box) (string, error) {
	c, err := a.core.SuggestShell(romID, box)
	return c, friendly(err)
}

// SaveLabel writes a card game's label using the box you drew. A null shell
// leaves the game's colour as it is.
func (a *App) SaveLabel(platform, stem string, romID int, box label.Box, shell *sdcard.Shell) error {
	return friendly(a.core.SaveLabel(platform, stem, romID, &box, shell))
}

// ChooseLabel asks for a picture and makes it the game's label. It returns
// false if you cancelled.
func (a *App) ChooseLabel(platform, stem string) (bool, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Choose a label picture",
		Filters: []runtime.FileFilter{{DisplayName: "Pictures (*.png, *.jpg, *.webp)", Pattern: "*.png;*.jpg;*.jpeg;*.webp;*.gif"}},
	})
	if err != nil || path == "" {
		return false, friendly(err)
	}
	f, err := os.Open(path)
	if err != nil {
		return false, friendly(err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, romm.MaxImageBytes+1))
	if err != nil {
		return false, friendly(err)
	}
	if len(data) > romm.MaxImageBytes {
		return false, errors.New("That picture is too big.")
	}
	return true, friendly(a.core.LabelFromImage(platform, stem, data))
}

// SetShell changes just a game's shell colour.
func (a *App) SetShell(stem string, shell sdcard.Shell) error {
	return friendly(a.core.SetShell(stem, shell))
}

// RemoveGame deletes a game and its label from the card (saves are kept).
func (a *App) RemoveGame(platform, stem, file string) error {
	return friendly(a.core.RemoveGame(platform, stem, file))
}

// Eject safely ejects the SD card.
func (a *App) Eject() error { return friendly(a.core.Eject()) }

// SetMatch pairs a card game with a different RomM game.
func (a *App) SetMatch(platform, stem string, romID int) { a.core.SetMatch(platform, stem, romID) }

// ---- pictures ----------------------------------------------------------------

// imageHandler serves pictures to the page, fetched through the app so the
// RomM login never has to be in the page:
//
//	/img/thumb/<romId>         small tile picture: cartridge photo, else box art
//	/img/cover/<romId>         box art
//	/img/cart/<romId>          cartridge photo
//	/img/label/<GBA>/<stem>    a label already on the card
//	/img/crop/<romId>?box=...  the label cut from the cartridge photo
func (a *App) imageHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.SplitN(strings.TrimPrefix(r.URL.EscapedPath(), "/img/"), "/", 2)
		if len(parts) != 2 || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		var data []byte
		var err error
		switch parts[0] {
		case "thumb":
			id, convErr := strconv.Atoi(parts[1])
			if convErr != nil {
				http.NotFound(w, r)
				return
			}
			data, err = a.core.Thumb(id)
		case "cover", "cart":
			id, convErr := strconv.Atoi(parts[1])
			if convErr != nil {
				http.NotFound(w, r)
				return
			}
			if parts[0] == "cover" {
				data, err = a.core.CoverImage(id)
			} else {
				data, _, err = a.core.CartImage(id)
			}
		case "crop":
			// /img/crop/<romId>?box=l,t,r,b  (no box: found automatically)
			id, convErr := strconv.Atoi(parts[1])
			if convErr != nil {
				http.NotFound(w, r)
				return
			}
			var box *label.Box
			if f := strings.Split(r.URL.Query().Get("box"), ","); len(f) == 4 {
				var b label.Box
				for i := range f {
					b[i], _ = strconv.Atoi(f[i])
				}
				box = &b
			}
			data, err = a.core.CropPNG(id, box)
		case "label":
			lp := strings.SplitN(parts[1], "/", 2)
			if len(lp) != 2 {
				http.NotFound(w, r)
				return
			}
			stem, _ := url.PathUnescape(lp[1])
			data, err = a.core.CardLabel(lp[0], stem)
			w.Header().Set("Cache-Control", "no-store")
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", http.DetectContentType(data))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Write(data)
	})
}
