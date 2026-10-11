package romm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPairing(t *testing.T) {
	polls := 0
	var sawAuth bool
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			sawAuth = true
		}
		switch r.URL.Path {
		case "/api/auth/device/init":
			var in struct {
				Scopes []string `json:"requested_scopes"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			asked = in.Scopes
			w.WriteHeader(201)
			w.Write([]byte(`{"device_code":"secret","user_code":"ABCD-1234","verification_path":"/pair/device","verification_path_complete":"/pair/device?user_code=ABCD-1234","expires_in":600,"interval":5}`))
		case "/api/auth/device/token":
			polls++
			if polls < 3 {
				w.WriteHeader(400)
				w.Write([]byte(`{"detail":"authorization_pending"}`))
				return
			}
			w.Write([]byte(`{"access_token":"rmm_new","device_id":"d","scopes":["platforms.read","roms.read"],"expires_at":null}`))
		}
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "", "", "")
	p, err := c.StartPairing("pc-1", "SlotDrop on PC")
	if err != nil {
		t.Fatal(err)
	}
	if p.Link != srv.URL+"/pair/device?user_code=ABCD-1234" || p.UserCode != "ABCD-1234" {
		t.Fatalf("link %q code %q", p.Link, p.UserCode)
	}
	if strings.Join(asked, " ") != "platforms.read roms.read" {
		t.Fatalf("should ask only for the read permissions, asked %v", asked)
	}
	p.Interval = 5 * time.Millisecond
	tok, err := c.WaitForPairing(context.Background(), p)
	if err != nil || tok != "rmm_new" || polls != 3 {
		t.Fatalf("token %q after %d polls: %v", tok, polls, err)
	}
	if sawAuth {
		t.Fatal("pairing must not send any login")
	}
}

func TestPairingRefusedAndCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"detail":"access_denied"}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "", "", "")
	p := &Pairing{DeviceCode: "x", Interval: time.Millisecond, Expires: time.Now().Add(time.Minute)}
	if _, err := c.WaitForPairing(context.Background(), p); err == nil || !strings.Contains(err.Error(), "turned down") {
		t.Fatalf("refused: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.Interval = time.Hour
	if _, err := c.WaitForPairing(ctx, p); err != ErrPairingCancelled {
		t.Fatalf("cancelled: %v", err)
	}
}

func TestPairingLinkStaysOnRomM(t *testing.T) {
	c, _ := New("https://romm.example.com", "", "", "")
	for _, bad := range []string{"//evil.example/pair", "https://evil.example/pair", "pair/device", "/pair\\x", "/a b"} {
		if _, err := c.pageLink(bad); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
	if l, err := c.pageLink("/pair/device?user_code=AB-12"); err != nil || l != "https://romm.example.com/pair/device?user_code=AB-12" {
		t.Fatalf("%q %v", l, err)
	}
}
