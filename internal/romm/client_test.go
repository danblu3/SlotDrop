package romm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMissingScopeAndBadLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer limited":
			w.WriteHeader(http.StatusForbidden)
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "", "", "limited")
	_, err := c.Platforms()
	if err == nil || !strings.Contains(err.Error(), "platforms.read and roms.read") {
		t.Fatalf("a token missing a permission should say which it needs: %v", err)
	}
	c, _ = New(srv.URL, "", "", "wrong")
	_, err = c.Platforms()
	if err == nil || !strings.Contains(err.Error(), "refused the login") {
		t.Fatalf("a bad token should say the login was refused: %v", err)
	}
}
