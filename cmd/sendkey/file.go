package main

// File send and receive over the chunked pipeline. Mirrors files.js: chunks
// are raw AES-GCM under one file key with the index bound as AAD, the head
// is a v1 envelope with FlagFile whose body is the manifest.

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	sendkey "github.com/realanshuman/sendkey/sendkey"
)

const (
	chunkSize      = 512 * 1024
	maxFileSize    = 8 * 1024 * 1024
	chunkViewSlack = 2
	chunkTTLSlack  = 300 // seconds; chunks outlive the head
)

type createResp struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

// postSecret uploads one ciphertext with small retries on 429/5xx: the
// limiter bucket refills roughly every two seconds.
func postSecret(base string, body map[string]any) (string, error) {
	raw, _ := json.Marshal(body)
	for attempt := 0; ; attempt++ {
		resp, err := httpClient().Post(base+"/api/secret", "application/json", bytes.NewReader(raw))
		if err != nil {
			return "", err
		}
		var out createResp
		decErr := json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK && decErr == nil {
			return out.ID, nil
		}
		if (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) && attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * 1500 * time.Millisecond)
			continue
		}
		if out.Error != "" {
			return "", fmt.Errorf("server: %s", out.Error)
		}
		return "", fmt.Errorf("upload failed: HTTP %d", resp.StatusCode)
	}
}

// sealAndUploadFile chunks, encrypts and uploads path, returning the sealed
// manifest head ready for the normal create call.
func sealAndUploadFile(base, path, passphrase string, ttlSec, views int) (*sendkey.Sealed, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("file is empty")
	}
	if len(data) > maxFileSize {
		return nil, fmt.Errorf("file is %d bytes; the cap is %d (8 MiB)", len(data), maxFileSize)
	}

	kf := make([]byte, sendkey.KeyLen)
	if err := fillRand(kf); err != nil {
		return nil, err
	}

	total := (len(data) + chunkSize - 1) / chunkSize
	chunkViews := min(10, views+chunkViewSlack)
	ids := make([]string, 0, total)
	for i := 0; i < total; i++ {
		end := min(len(data), (i+1)*chunkSize)
		ct, iv, err := sendkey.SealChunk(kf, i, data[i*chunkSize:end])
		if err != nil {
			return nil, err
		}
		id, err := postSecret(base, map[string]any{
			"ct":    base64.RawURLEncoding.EncodeToString(ct),
			"iv":    base64.RawURLEncoding.EncodeToString(iv),
			"ttl":   ttlSec + chunkTTLSlack,
			"views": chunkViews,
		})
		if err != nil {
			return nil, fmt.Errorf("chunk %d/%d: %w", i+1, total, err)
		}
		ids = append(ids, id)
		fmt.Fprintf(os.Stderr, "\rencrypting and uploading: %d/%d chunks", i+1, total)
	}
	fmt.Fprintln(os.Stderr)

	manifest := sendkey.Manifest{
		V:      1,
		Name:   filepath.Base(path),
		Mime:   "application/octet-stream",
		Size:   int64(len(data)),
		KF:     base64.RawURLEncoding.EncodeToString(kf),
		Chunks: ids,
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	return sendkey.SealWithFlags(raw, passphrase, sendkey.FlagFile)
}

// downloadManifestFile fetches and decrypts every chunk, writing the file to
// outPath (or the manifest's sanitized name in the working directory).
// Refuses to overwrite an existing file unless outPath was given explicitly.
func downloadManifestFile(base string, manifestRaw []byte, outPath string) error {
	var m sendkey.Manifest
	if err := json.Unmarshal(manifestRaw, &m); err != nil {
		return errors.New("this link carries a file, but its manifest is damaged")
	}
	if err := m.Validate(); err != nil {
		return fmt.Errorf("file manifest rejected: %w", err)
	}
	kf, _ := base64.RawURLEncoding.DecodeString(m.KF)

	explicit := outPath != ""
	if !explicit {
		outPath = sanitizeFilename(m.Name)
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !explicit {
		flags |= os.O_EXCL // never silently clobber in the implicit case
	}
	f, err := os.OpenFile(outPath, flags, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("%s already exists; pass -o to choose a path", outPath)
		}
		return err
	}
	defer f.Close()

	total := len(m.Chunks)
	for i, id := range m.Chunks {
		resp, err := httpClient().Get(base + "/api/secret/" + id)
		if err != nil {
			return fmt.Errorf("chunk %d/%d: %w", i+1, total, err)
		}
		var out struct{ CT, IV, Error string }
		decErr := json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || decErr != nil {
			return fmt.Errorf("chunk %d/%d is gone: it expired or was consumed by another download", i+1, total)
		}
		ct, _ := base64.RawURLEncoding.DecodeString(out.CT)
		iv, _ := base64.RawURLEncoding.DecodeString(out.IV)
		plain, err := sendkey.OpenChunk(kf, i, ct, iv)
		if err != nil {
			return fmt.Errorf("chunk %d/%d failed authentication", i+1, total)
		}
		if _, err := f.Write(plain); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "\rdownloading and decrypting: %d/%d chunks", i+1, total)
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "wrote %s (%d bytes)\n", outPath, m.Size)
	return nil
}

func sanitizeFilename(name string) string {
	clean := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.ReplaceAll(strings.ReplaceAll(name, "/", "_"), "\\", "_"))
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return "download.bin"
	}
	return clean
}

func fillRand(b []byte) error {
	_, err := rand.Read(b)
	return err
}
