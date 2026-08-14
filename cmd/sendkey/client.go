package main

// CLI client: `sendkey send` and `sendkey get`. Encryption happens here, in
// the client, before anything touches the network — the server is treated as
// untrusted storage.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	sendkey "github.com/realanshuman/sendkey/sendkey"
)

// maxPassphraseTries bounds interactive retries. Unbounded retrying is fine
// in principle — the ciphertext is already local, so nothing burns — but a
// loop that re-prompts on EOF never terminates when stdin is a pipe or
// /dev/null, which is how this spins forever in scripts and CI.
const maxPassphraseTries = 5

func cmdSend(args []string) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	server := fs.String("server", envOr("SENDKEY_SERVER", "http://localhost:8080"), "sendkey server URL")
	ttl := fs.Duration("ttl", 24*time.Hour, "time before the secret self-destructs (1m..168h)")
	views := fs.Int("views", 1, "reads before the secret burns (1..10)")
	withPass := fs.Bool("passphrase", false, "add a passphrase layer (prompted; or SENDKEY_PASSPHRASE)")
	filePath := fs.String("file", "", "send a file instead of a text secret (up to 8 MiB, chunked)")
	_ = fs.Parse(args)

	var secret []byte
	var err error
	if *filePath == "" {
		secret, err = readSecret(fs.Args())
		if err != nil {
			fatal("read secret: %v", err)
		}
		if len(secret) == 0 {
			fatal("empty secret; pass it as an argument, pipe it on stdin, or use -file")
		}
	} else if fs.NArg() > 0 {
		fatal("-file and a text secret are mutually exclusive")
	}

	passphrase := ""
	if *withPass {
		if passphrase = os.Getenv("SENDKEY_PASSPHRASE"); passphrase == "" {
			passphrase, err = promptSecret("Passphrase: ")
			if err != nil || passphrase == "" {
				fatal("passphrase required with -passphrase")
			}
		}
	}

	base := strings.TrimRight(*server, "/")
	var sealed *sendkey.Sealed
	if *filePath != "" {
		sealed, err = sealAndUploadFile(base, *filePath, passphrase, int(ttl.Seconds()), *views)
	} else {
		sealed, err = sendkey.Seal(secret, passphrase)
	}
	if err != nil {
		fatal("encrypt: %v", err)
	}

	reqBody, _ := json.Marshal(map[string]any{
		"ct":    base64.RawURLEncoding.EncodeToString(sealed.CT),
		"iv":    base64.RawURLEncoding.EncodeToString(sealed.IV),
		"ttl":   int(ttl.Seconds()),
		"views": *views,
	})

	resp, err := httpClient().Post(base+"/api/secret", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		fatal("upload: %v", err)
	}
	defer resp.Body.Close()

	var out struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != http.StatusOK {
		if out.Error != "" {
			fatal("server: %s", out.Error)
		}
		fatal("upload failed: HTTP %d", resp.StatusCode)
	}

	link := fmt.Sprintf("%s/s/%s#%s", base, out.ID,
		base64.RawURLEncoding.EncodeToString(sealed.Key))

	// The link goes to stdout so it pipes cleanly; the human-readable summary
	// goes to stderr so it does not.
	fmt.Fprintf(os.Stderr, "one-time link (burns after %d view(s), expires in %s):\n",
		sendkey.ClampViews(*views), sendkey.ClampTTL(int(ttl.Seconds())))
	fmt.Println(link)
}

func cmdGet(args []string) {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	outPath := fs.String("o", "", "write a received file to this path (file links only)")
	_ = fs.Parse(args)

	if fs.NArg() != 1 {
		fatal("usage: sendkey get <link>")
	}

	u, err := url.Parse(fs.Arg(0))
	if err != nil || u.Fragment == "" || !strings.HasPrefix(u.Path, "/s/") {
		fatal("not a sendkey link (expected .../s/<id>#<key>)")
	}
	id := strings.TrimPrefix(u.Path, "/s/")
	key, err := base64.RawURLEncoding.DecodeString(u.Fragment)
	if err != nil || len(key) != sendkey.KeyLen {
		fatal("link is missing a valid key fragment")
	}

	// Only scheme://host/api/... is requested: the fragment never leaves this
	// process, exactly as it never leaves a browser.
	resp, err := httpClient().Get(u.Scheme + "://" + u.Host + "/api/secret/" + url.PathEscape(id))
	if err != nil {
		fatal("fetch: %v", err)
	}
	defer resp.Body.Close()

	var out struct {
		CT    string `json:"ct"`
		IV    string `json:"iv"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != http.StatusOK {
		if out.Error != "" {
			fatal("server: %s", out.Error)
		}
		fatal("fetch failed: HTTP %d", resp.StatusCode)
	}

	ct, err1 := base64.RawURLEncoding.DecodeString(out.CT)
	iv, err2 := base64.RawURLEncoding.DecodeString(out.IV)
	if err1 != nil || err2 != nil {
		fatal("server returned malformed data")
	}

	secret, err := decrypt(ct, iv, key)
	if err != nil {
		fatal("%v", err)
	}

	// File links carry a manifest, not the payload: finish the download
	// instead of printing chunk ids to the terminal.
	if flags, ferr := sendkey.EnvelopeFlags(ct, iv, key); ferr == nil && flags&sendkey.FlagFile != 0 {
		if err := downloadManifestFile(u.Scheme+"://"+u.Host, secret, *outPath); err != nil {
			fatal("%v", err)
		}
		return
	}
	os.Stdout.Write(secret)
	if len(secret) > 0 && secret[len(secret)-1] != '\n' && isTerminal(os.Stdout) {
		fmt.Println()
	}
}

// decrypt opens the envelope, prompting for a passphrase when one is needed.
// Retries are bounded and stop as soon as stdin cannot supply another answer,
// so a wrong SENDKEY_PASSPHRASE in a script fails fast instead of spinning.
func decrypt(ct, iv, key []byte) ([]byte, error) {
	needsPass, err := sendkey.NeedsPassphrase(ct, iv, key)
	if err != nil {
		return nil, err
	}

	passphrase, fromEnv := os.Getenv("SENDKEY_PASSPHRASE"), os.Getenv("SENDKEY_PASSPHRASE") != ""
	for try := 0; ; try++ {
		if needsPass && passphrase == "" {
			if !isTerminal(os.Stdin) {
				return nil, errors.New("this secret needs a passphrase; set SENDKEY_PASSPHRASE " +
					"or run with a terminal attached")
			}
			if try >= maxPassphraseTries {
				return nil, fmt.Errorf("giving up after %d attempts", maxPassphraseTries)
			}
			if passphrase, err = promptSecret("Passphrase: "); err != nil {
				return nil, fmt.Errorf("read passphrase: %w", err)
			}
			if passphrase == "" { // EOF or an empty line
				return nil, errors.New("no passphrase supplied")
			}
		}

		secret, err := sendkey.Open(ct, iv, key, passphrase)
		if err == nil {
			return secret, nil
		}
		if err != sendkey.ErrBadPassphrase {
			return nil, err
		}
		if fromEnv {
			return nil, errors.New("SENDKEY_PASSPHRASE is wrong for this secret")
		}
		// The ciphertext is already local — retrying costs nothing and cannot
		// burn anything, since the single server fetch already happened.
		fmt.Fprintln(os.Stderr, "wrong passphrase, try again (the secret is safe — retries are local)")
		passphrase = ""
	}
}

// readSecret takes the secret from the CLI args or, when absent or "-", from
// stdin (so `pass show x | sendkey send` works).
func readSecret(args []string) ([]byte, error) {
	if len(args) > 0 && args[0] != "-" {
		return []byte(strings.Join(args, " ")), nil
	}
	if isTerminal(os.Stdin) {
		fmt.Fprintln(os.Stderr, "reading secret from stdin — end with Ctrl-D:")
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	return bytes.TrimRight(data, "\n"), err
}

// promptSecret reads a line from the terminal, disabling echo via stty when
// available (which keeps the binary dependency-free).
func promptSecret(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)

	echoOff := exec.Command("stty", "-echo")
	echoOff.Stdin = os.Stdin
	if echoOff.Run() == nil {
		defer func() {
			echoOn := exec.Command("stty", "echo")
			echoOn.Stdin = os.Stdin
			_ = echoOn.Run()
			fmt.Fprintln(os.Stderr)
		}()
	}

	var line strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			if buf[0] != '\r' {
				line.WriteByte(buf[0])
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
	}
	return line.String(), nil
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sendkey: "+format+"\n", args...)
	os.Exit(1)
}
