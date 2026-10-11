package romm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Pairing is a "sign in with your phone" request (RomM 5.4's device sign-in).
// SlotDrop asks RomM for a short code, shows it as a QR code, and waits while
// you approve it in RomM on your phone. RomM then hands over an API token
// with only the permissions SlotDrop asked for.
//
// DeviceCode is the secret SlotDrop polls with: it is never shown or logged.
type Pairing struct {
	DeviceCode string
	UserCode   string
	Link       string // the RomM page to approve on, with the code filled in
	Interval   time.Duration
	Expires    time.Time
}

// PairingScopes are the permissions SlotDrop asks for when pairing.
var PairingScopes = []string{"platforms.read", "roms.read"}

// ErrPairingCancelled is returned by WaitForPairing when ctx is cancelled.
var ErrPairingCancelled = errors.New("pairing cancelled")

// postJSON sends a JSON body to a RomM API path without any login, and
// returns the HTTP status and body.
func (c *Client) postJSON(path string, in any) (int, []byte, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequest("POST", c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, errorf("%q doesn't look like a web address.", c.BaseURL)
	}
	req.Header.Set("User-Agent", "SlotDrop/2.0")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := *c.http
	client.Timeout = 30 * time.Second
	resp, err := client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return 0, nil, errorf("Couldn't reach %s: %v", c.BaseURL, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, nil, errorf("The answer from %s was cut off: %v", c.BaseURL, err)
	}
	return resp.StatusCode, data, nil
}

// StartPairing asks RomM for a pairing code. deviceID should stay the same
// for this PC, so approving again replaces the old token rather than adding
// another device in RomM.
func (c *Client) StartPairing(deviceID, name string) (*Pairing, error) {
	status, data, err := c.postJSON("/api/auth/device/init", map[string]any{
		"client_device_identifier": deviceID,
		"name":                     name,
		"client":                   "SlotDrop",
		"platform":                 "Windows",
		"requested_scopes":         PairingScopes,
	})
	if err != nil {
		return nil, err
	}
	switch {
	case status == 404 || status == 405:
		return nil, errorf("This RomM doesn't have phone sign-in. It needs RomM 5.4 or newer; use a password or API token instead.")
	case status == 429:
		return nil, errorf("RomM says too many tries. Wait a minute and try again.")
	case status >= 300:
		return nil, errorf("RomM returned HTTP %d when asked for a pairing code.", status)
	}
	var r struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
		Path       string `json:"verification_path_complete"`
		ExpiresIn  int    `json:"expires_in"`
		Interval   int    `json:"interval"`
	}
	if json.Unmarshal(data, &r) != nil || r.DeviceCode == "" || r.UserCode == "" {
		return nil, errorf("%s didn't answer like RomM does. Is the address right?", c.BaseURL)
	}
	link, err := c.pageLink(r.Path)
	if err != nil {
		return nil, err
	}
	p := &Pairing{
		DeviceCode: r.DeviceCode,
		UserCode:   r.UserCode,
		Link:       link,
		Interval:   time.Duration(min(max(r.Interval, 2), 30)) * time.Second,
		Expires:    time.Now().Add(time.Duration(min(max(r.ExpiresIn, 30), 1800)) * time.Second),
	}
	return p, nil
}

// pageLink joins a RomM web page path onto the RomM address, and refuses
// anything that would point somewhere other than this RomM server, so the
// QR code can only ever send your phone to your own RomM.
func (c *Client) pageLink(path string) (string, error) {
	bad := errorf("RomM sent back an odd pairing link, so SlotDrop won't show it.")
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "\\\r\n\t ") {
		return "", bad
	}
	link := c.BaseURL + path
	u, err := url.Parse(link)
	if err != nil || u.Host != c.host || u.User != nil {
		return "", bad
	}
	return link, nil
}

// WaitForPairing polls RomM until the pairing is approved, refused, runs
// out, or ctx is cancelled. It returns the new API token.
func (c *Client) WaitForPairing(ctx context.Context, p *Pairing) (string, error) {
	wait := p.Interval
	for {
		select {
		case <-ctx.Done():
			if time.Now().After(p.Expires) {
				return "", errorf("The pairing code ran out before it was approved. Try again.")
			}
			return "", ErrPairingCancelled
		case <-time.After(wait):
		}
		if time.Now().After(p.Expires) {
			return "", errorf("The pairing code ran out before it was approved. Try again.")
		}
		status, data, err := c.postJSON("/api/auth/device/token", map[string]string{"device_code": p.DeviceCode})
		if err != nil {
			continue // a blip on the network: keep waiting until the code runs out
		}
		if status == 200 {
			var r struct {
				Token string `json:"access_token"`
			}
			if json.Unmarshal(data, &r) != nil || strings.TrimSpace(r.Token) == "" {
				return "", errorf("RomM approved SlotDrop but didn't send a token. Try again.")
			}
			return r.Token, nil
		}
		var e struct {
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal(data, &e)
		switch {
		case e.Detail == "authorization_pending":
		case e.Detail == "slow_down" || status == 429:
			wait += 5 * time.Second
		case e.Detail == "access_denied":
			return "", errorf("The pairing was turned down in RomM.")
		case e.Detail == "expired_token":
			return "", errorf("The pairing code ran out before it was approved. Try again.")
		default:
			return "", errorf("RomM returned HTTP %d while waiting for the pairing.", status)
		}
	}
}
