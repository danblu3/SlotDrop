// Package romm is a small client for the parts of the RomM API this app needs.
//
// Security notes:
//   - The login (password or token) is only ever sent to the RomM server's
//     own host. Go's HTTP client also drops it if RomM redirects elsewhere.
//   - Error messages never include query strings, because ScreenScraper image
//     addresses can carry the RomM server's ScreenScraper login.
//   - On Windows, HTTPS certificates are checked with the Windows certificate
//     store, the same as your browser.
package romm

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"slotlabelmaker/internal/platforms"
)

// ResourcesPrefix is where RomM serves downloaded artwork.
const ResourcesPrefix = "/assets/romm/resources/"

// NeededScopes are the API token permissions SlotDrop uses: the platform
// list, the ROM list, and downloading a ROM. It only ever reads.
const NeededScopes = "platforms.read and roms.read"

// Download limits, so a bad server can't fill memory.
const (
	MaxJSONBytes  = 64 << 20
	MaxImageBytes = 32 << 20
	MaxROMBytes   = 256 << 20
)

// Error is a message that's safe to show to the user.
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

func errorf(format string, args ...any) error { return &Error{fmt.Sprintf(format, args...)} }

// Client talks to one RomM server.
type Client struct {
	BaseURL string
	host    string
	secure  bool   // the RomM address is https, so the login must never go over http
	auth    string // the Authorization header value, or ""
	http    *http.Client
}

// New makes a client. Give either a username and password or an API token.
func New(baseURL, username, password, token string) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errorf("Type your RomM address first.")
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return nil, errorf("%q doesn't look like a web address.", baseURL)
	}
	c := &Client{
		BaseURL: baseURL,
		host:    u.Host,
		secure:  u.Scheme == "https",
		http: &http.Client{
			// Never follow a redirect from https down to http.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return errors.New("too many redirects")
				}
				if req.URL.Scheme != "https" && via[0].URL.Scheme == "https" {
					return errors.New("refused a redirect from https to http")
				}
				return nil
			},
			Timeout: 0, // per-request timeouts are set with deadlines below
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
				TLSHandshakeTimeout:   15 * time.Second,
				ResponseHeaderTimeout: 60 * time.Second,
				MaxIdleConnsPerHost:   8,
				ForceAttemptHTTP2:     true,
			},
		},
	}
	switch {
	case strings.TrimSpace(token) != "":
		c.auth = "Bearer " + strings.TrimSpace(token)
	case username != "":
		c.auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
	}
	return c, nil
}

// IsHTTPS reports whether the login travels encrypted.
func (c *Client) IsHTTPS() bool { return strings.HasPrefix(c.BaseURL, "https://") }

// safeURL strips the query string so logins in image addresses never show up in errors.
func safeURL(raw string) string {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		return raw[:i]
	}
	return raw
}

// Progress is told how many bytes have arrived out of total (0 if unknown).
type Progress func(done, total int64)

type progressReader struct {
	r     io.Reader
	done  int64
	total int64
	fn    Progress
	last  time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if time.Since(p.last) > 100*time.Millisecond || err == io.EOF {
		p.last = time.Now()
		p.fn(p.done, p.total)
	}
	return n, err
}

func (c *Client) get(raw string, timeout time.Duration, limit int64) ([]byte, error) {
	return c.getProgress(raw, timeout, limit, nil)
}

func (c *Client) getProgress(raw string, timeout time.Duration, limit int64, progress Progress) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errorf("Not a web address: %s", safeURL(raw))
	}
	req, err := http.NewRequest("GET", raw, nil)
	if err != nil {
		return nil, errorf("Not a web address: %s", safeURL(raw))
	}
	req.Header.Set("User-Agent", "SlotDrop/2.0")
	req.Header.Set("Accept", "*/*")
	// Only send our login to the RomM server, never to other sites.
	if c.auth != "" && u.Host == c.host && (u.Scheme == "https" || !c.secure) {
		req.Header.Set("Authorization", c.auth)
	}
	client := *c.http
	client.Timeout = timeout
	resp, err := client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, errorf("Couldn't reach %s: %v", safeURL(raw), err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == 401:
		return nil, errorf("RomM refused the login. Check the username and password, or the API token.")
	case resp.StatusCode == 403:
		// Signed in, but not allowed: usually an API token missing a permission.
		return nil, errorf("RomM signed you in but won't allow this (HTTP 403). If you use an API token, it needs the %s permissions.", NeededScopes)
	case resp.StatusCode == 404:
		return nil, errorf("RomM couldn't find that file (HTTP 404). Try a rescan in RomM.")
	case resp.StatusCode >= 300:
		return nil, errorf("RomM returned HTTP %d for %s", resp.StatusCode, safeURL(raw))
	}
	if resp.ContentLength > limit {
		return nil, errorf("%s is too big (%d MB).", safeURL(raw), resp.ContentLength>>20)
	}
	var body io.Reader = resp.Body
	if progress != nil {
		body = &progressReader{r: resp.Body, total: max(resp.ContentLength, 0), fn: progress}
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, errorf("The download from %s was cut off: %v", safeURL(raw), err)
	}
	if int64(len(data)) > limit {
		return nil, errorf("%s is too big.", safeURL(raw))
	}
	return data, nil
}

func (c *Client) getJSON(path string, params url.Values, out any) error {
	raw := c.BaseURL + path
	if len(params) > 0 {
		raw += "?" + params.Encode()
	}
	data, err := c.get(raw, 60*time.Second, MaxJSONBytes)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errorf("%s didn't answer like RomM does. Is the address right?", c.BaseURL+path)
	}
	return nil
}

// ---- API types ----------------------------------------------------------

// Platform is a RomM platform.
type Platform struct {
	ID       int    `json:"id"`
	Slug     string `json:"slug"`
	FsSlug   string `json:"fs_slug"`
	Name     string `json:"name"`
	RomCount int    `json:"rom_count"`
}

// File is one file of a RomM game.
type File struct {
	FileName string `json:"file_name"`
	CRC      string `json:"crc_hash"`
}

// SSMetadata holds the ScreenScraper fields we use.
type SSMetadata struct {
	PhysicalPath string `json:"physical_path"`
	PhysicalURL  string `json:"physical_url"`
	Box2DPath    string `json:"box2d_path"`
	Box2DURL     string `json:"box2d_url"`
}

// Rom is a game in RomM.
type Rom struct {
	ID             int         `json:"id"`
	PlatformID     int         `json:"platform_id"`
	Name           string      `json:"name"`
	FsName         string      `json:"fs_name"`
	FsNameNoExt    string      `json:"fs_name_no_ext"`
	FsNameNoTags   string      `json:"fs_name_no_tags"`
	FsSizeBytes    int64       `json:"fs_size_bytes"`
	CRC            string      `json:"crc_hash"`
	Files          []File      `json:"files"`
	PathCoverSmall string      `json:"path_cover_small"`
	PathCoverLarge string      `json:"path_cover_large"`
	URLCover       string      `json:"url_cover"`
	Regions        []string    `json:"regions"`
	Summary        string      `json:"summary"`
	MissingFromFS  bool        `json:"missing_from_fs"`
	CreatedAt      string      `json:"created_at"` // when it was added to RomM
	SS             *SSMetadata `json:"ss_metadata"`
}

// DisplayName is how a game is shown in the app.
func (r *Rom) DisplayName() string {
	for _, s := range []string{r.Name, r.FsNameNoExt, r.FsName} {
		if s != "" {
			return s
		}
	}
	return "Game " + strconv.Itoa(r.ID)
}

// ---- API calls ----------------------------------------------------------

// Platforms lists RomM's platforms. It's also how we check the login works.
func (c *Client) Platforms() ([]Platform, error) {
	var out []Platform
	return out, c.getJSON("/api/platforms", nil, &out)
}

// PlatformIDs returns RomM's ids for one of Slot's systems.
func PlatformIDs(p *platforms.Platform, all []Platform) []int {
	var ids []int
	for _, rp := range all {
		if platforms.BySlug(rp.Slug) == p || platforms.BySlug(rp.FsSlug) == p {
			ids = append(ids, rp.ID)
		}
	}
	return ids
}

// Roms lists every game on a platform, following RomM's pages.
func (c *Client) Roms(platformID int) ([]Rom, error) {
	var roms []Rom
	const pageSize = 500
	for offset := 0; ; {
		var page struct {
			Items []Rom `json:"items"`
			Total int   `json:"total"`
		}
		// with_files gives each file's checksum, used to match renamed romhacks.
		params := url.Values{
			"platform_ids": {strconv.Itoa(platformID)},
			"with_files":   {"true"},
			"limit":        {strconv.Itoa(pageSize)},
			"offset":       {strconv.Itoa(offset)},
		}
		if err := c.getJSON("/api/roms", params, &page); err != nil {
			return nil, err
		}
		roms = append(roms, page.Items...)
		offset += len(page.Items)
		total := page.Total
		if total == 0 {
			total = len(roms)
		}
		if len(page.Items) == 0 || offset >= total {
			return roms, nil
		}
	}
}

// ResourceURL turns a RomM media path into a full address.
func (c *Client) ResourceURL(path string) string {
	switch {
	case path == "":
		return ""
	case strings.HasPrefix(path, "http://"), strings.HasPrefix(path, "https://"):
		return path
	case strings.HasPrefix(path, "/"):
		return c.BaseURL + path
	default:
		return c.BaseURL + ResourcesPrefix + path
	}
}

// CartridgeURLs lists where to find a game's cartridge photo, best first.
//
// RomM keeps ScreenScraper's "support-2D" image (the cartridge) as
// ss_metadata.physical_path when "physical" media is enabled in its config,
// and the original ScreenScraper address as physical_url.
func (c *Client) CartridgeURLs(r *Rom) []string {
	var urls []string
	if r.SS != nil {
		if r.SS.PhysicalPath != "" {
			urls = append(urls, c.ResourceURL(r.SS.PhysicalPath))
		}
		if r.SS.PhysicalURL != "" {
			urls = append(urls, r.SS.PhysicalURL)
		}
	}
	return urls
}

// ArtURLs lists pictures for a game's tile, best first: the cartridge photo,
// then RomM's cover, then ScreenScraper's box art.
func (c *Client) ArtURLs(r *Rom) []string {
	urls := c.CartridgeURLs(r)
	for _, p := range []string{r.PathCoverSmall, r.PathCoverLarge, r.URLCover} {
		if p != "" {
			urls = append(urls, c.ResourceURL(p))
		}
	}
	if r.SS != nil {
		for _, p := range []string{r.SS.Box2DPath, r.SS.Box2DURL} {
			if p != "" {
				urls = append(urls, c.ResourceURL(p))
			}
		}
	}
	return urls
}

// Image downloads a picture.
func (c *Client) Image(raw string) ([]byte, error) {
	return c.get(raw, 60*time.Second, MaxImageBytes)
}

// DownloadRom fetches the game file itself, or a zip if RomM stores it as a
// zip or as several files.
func (c *Client) DownloadRom(r *Rom, progress Progress) ([]byte, error) {
	name := r.FsName
	if name == "" {
		name = fmt.Sprintf("rom-%d", r.ID)
	}
	return c.getProgress(fmt.Sprintf("%s/api/roms/%d/content/%s", c.BaseURL, r.ID, url.PathEscape(name)),
		10*time.Minute, MaxROMBytes, progress)
}
