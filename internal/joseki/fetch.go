package joseki

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"easygo/internal/sgf"
)

// KogoURL is where Kogo's Joseki Dictionary is published. Its header asks
// that it not be redistributed without the author's permission, so the app
// downloads it for the person running it instead of shipping a copy.
const KogoURL = "http://waterfire.us/2014.02.25_KJD.zip"

// KogoFile is the cached dictionary name inside the data directory.
const KogoFile = "kogo-joseki-dictionary.sgf"

// FetchKogo downloads the dictionary into dir and returns the SGF path.
func FetchKogo(ctx context.Context, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, KogoURL, nil)
	if err != nil {
		return "", err
	}
	cl := &http.Client{Timeout: 2 * time.Minute}
	resp, err := cl.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", KogoURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: %s", KogoURL, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return "", err
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", fmt.Errorf("not a zip: %w", err)
	}
	for _, f := range zr.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".sgf") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		data, err := io.ReadAll(io.LimitReader(rc, 64<<20))
		rc.Close()
		if err != nil {
			return "", err
		}
		// Refuse to cache something that does not parse as SGF.
		if _, err := sgf.Parse(string(data)); err != nil {
			return "", fmt.Errorf("downloaded file is not valid SGF: %w", err)
		}
		dest := filepath.Join(dir, KogoFile)
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return "", err
		}
		return dest, nil
	}
	return "", fmt.Errorf("no .sgf file in the downloaded archive")
}

// LoadFile builds a database from one SGF file written in the top-right corner.
func LoadFile(path, source string) (*DB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	t, err := sgf.Parse(string(data))
	if err != nil {
		return nil, err
	}
	db := NewDB()
	db.AddTree(t, TopRight, source)
	return db, nil
}
