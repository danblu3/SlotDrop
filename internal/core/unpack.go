package core

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode/v2"

	"slotlabelmaker/internal/platforms"
	"slotlabelmaker/internal/romm"
)

// archiveFile is one game file inside a download.
type archiveFile struct {
	name string
	size uint64
	open func() (io.ReadCloser, error)
}

// Unpack returns the game file and its extension. RomM sends a plain game
// file, or a .zip, .7z or .rar if that's how it's stored; from an archive we
// take the biggest file of the right type.
func Unpack(data []byte, r *romm.Rom, p *platforms.Platform) ([]byte, string, error) {
	switch {
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, "", fmt.Errorf("the download for %s is a damaged zip", r.DisplayName())
		}
		var files []archiveFile
		for _, f := range zr.File {
			if !f.FileInfo().IsDir() {
				files = append(files, archiveFile{f.Name, f.UncompressedSize64, f.Open})
			}
		}
		return pick(files, r, p)
	case bytes.HasPrefix(data, []byte("7z\xbc\xaf\x27\x1c")):
		zr, err := sevenzip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, "", fmt.Errorf("couldn't open the .7z for %s: %v", r.DisplayName(), err)
		}
		var files []archiveFile
		for _, f := range zr.File {
			if !f.FileInfo().IsDir() {
				files = append(files, archiveFile{f.Name, f.UncompressedSize, f.Open})
			}
		}
		return pick(files, r, p)
	case bytes.HasPrefix(data, []byte("Rar!\x1a\x07")):
		return unrar(data, r, p)
	default: // a plain game file
		ext := strings.ToLower(filepath.Ext(r.FsName))
		if !p.HasExtension(ext) {
			ext = p.Extensions[0]
		}
		return data, ext, nil
	}
}

func pick(files []archiveFile, r *romm.Rom, p *platforms.Platform) ([]byte, string, error) {
	var best *archiveFile
	for i := range files {
		f := &files[i]
		if p.HasExtension(filepath.Ext(f.name)) && (best == nil || f.size > best.size) {
			best = f
		}
	}
	if best == nil {
		return nil, "", noGame(r, p)
	}
	if best.size > romm.MaxROMBytes {
		return nil, "", fmt.Errorf("the game file in %s is too big", r.DisplayName())
	}
	rc, err := best.open()
	if err != nil {
		return nil, "", unpackErr(r, err)
	}
	defer rc.Close()
	out, err := readLimited(rc, r)
	return out, strings.ToLower(filepath.Ext(best.name)), err
}

// unrar reads a .rar in order (that's how rar works), keeping the biggest game file.
func unrar(data []byte, r *romm.Rom, p *platforms.Platform) ([]byte, string, error) {
	// Cap the dictionary: a crafted .rar could otherwise ask for 4 GB up front.
	rr, err := rardecode.NewReader(bytes.NewReader(data), rardecode.MaxDictionarySize(256<<20))
	if err != nil {
		return nil, "", fmt.Errorf("couldn't open the .rar for %s: %v", r.DisplayName(), err)
	}
	var best []byte
	ext := ""
	for {
		h, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", unpackErr(r, err)
		}
		if h.IsDir || !p.HasExtension(filepath.Ext(h.Name)) {
			continue
		}
		if h.UnPackedSize > romm.MaxROMBytes {
			return nil, "", fmt.Errorf("the game file in %s is too big", r.DisplayName())
		}
		out, err := readLimited(rr, r)
		if err != nil {
			return nil, "", err
		}
		if len(out) > len(best) {
			best, ext = out, strings.ToLower(filepath.Ext(h.Name))
		}
	}
	if best == nil {
		return nil, "", noGame(r, p)
	}
	return best, ext, nil
}

func readLimited(rd io.Reader, r *romm.Rom) ([]byte, error) {
	out, err := io.ReadAll(io.LimitReader(rd, romm.MaxROMBytes+1))
	if err != nil {
		return nil, unpackErr(r, err)
	}
	if len(out) > romm.MaxROMBytes {
		return nil, fmt.Errorf("the game file in %s is too big", r.DisplayName())
	}
	return out, nil
}

func noGame(r *romm.Rom, p *platforms.Platform) error {
	return fmt.Errorf("the download for %s has no %s file in it", r.DisplayName(), strings.Join(p.Extensions, "/"))
}

func unpackErr(r *romm.Rom, err error) error {
	msg := err.Error()
	if strings.Contains(strings.ToLower(msg), "password") || strings.Contains(strings.ToLower(msg), "encrypt") {
		return errors.New("the archive for " + r.DisplayName() + " is password protected")
	}
	return fmt.Errorf("couldn't unpack %s: %v", r.DisplayName(), err)
}
