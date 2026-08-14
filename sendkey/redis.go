package sendkey

// RedisStore is the backend for serverless deployments (Vercel, Lambda, …),
// where MemStore cannot work: every invocation may land on a different
// instance, so process memory is not shared storage.
//
// It speaks the Upstash REST protocol — plain HTTPS, no connection pool —
// because serverless functions cannot hold a TCP connection between
// invocations. Vercel KV is Upstash under the hood, so both env var spellings
// are accepted.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

// burnScript performs the read-decrement-or-delete as one indivisible server
// side operation. Doing it as separate GET and DEL calls would open a window
// where two concurrent readers both observe the last view and both receive
// the secret — the exact failure this product exists to prevent.
//
// KEEPTTL preserves the original expiry so a multi-view secret cannot have
// its lifetime silently extended by being read.
const burnScript = `
local v = redis.call('GET', KEYS[1])
if not v then return nil end
local d = cjson.decode(v)
d.views = d.views - 1
if d.views <= 0 then
  redis.call('DEL', KEYS[1])
else
  redis.call('SET', KEYS[1], cjson.encode(d), 'KEEPTTL')
end
return {v, tostring(d.views)}
`

type storedSecret struct {
	CT    string `json:"ct"`    // base64url
	IV    string `json:"iv"`    // base64url
	Views int    `json:"views"` // remaining reads
}

// RedisStore implements Backend against an Upstash-compatible REST endpoint.
type RedisStore struct {
	url    string
	token  string
	client *http.Client
	prefix string
}

var _ Backend = (*RedisStore)(nil)

// NewRedisStore builds a backend from an endpoint and token.
func NewRedisStore(url, token string) *RedisStore {
	return &RedisStore{
		url:    url,
		token:  token,
		client: &http.Client{Timeout: 10 * time.Second},
		prefix: "sk:",
	}
}

// RedisFromEnv returns a RedisStore configured from the environment, or nil
// when no credentials are present. Both Vercel KV and native Upstash names
// are honoured so either integration works untouched.
func RedisFromEnv() *RedisStore {
	url := firstEnv("KV_REST_API_URL", "UPSTASH_REDIS_REST_URL")
	token := firstEnv("KV_REST_API_TOKEN", "UPSTASH_REDIS_REST_TOKEN")
	if url == "" || token == "" {
		return nil
	}
	return NewRedisStore(url, token)
}

func firstEnv(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}

// do issues one Redis command over the REST API and returns the raw result.
func (r *RedisStore) do(ctx context.Context, args ...any) (json.RawMessage, error) {
	body, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("redis: bad response (HTTP %d)", resp.StatusCode)
	}
	if out.Error != "" {
		return nil, fmt.Errorf("redis: %s", out.Error)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("redis: HTTP %d", resp.StatusCode)
	}
	return out.Result, nil
}

func (r *RedisStore) Put(ctx context.Context, ct, iv []byte, ttl time.Duration, views int) (string, error) {
	if views < 1 {
		views = 1
	}
	id, err := NewID()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(storedSecret{
		CT:    base64.RawURLEncoding.EncodeToString(ct),
		IV:    base64.RawURLEncoding.EncodeToString(iv),
		Views: views,
	})
	if err != nil {
		return "", err
	}

	// NX means a (vanishingly unlikely) id collision fails loudly rather than
	// overwriting somebody else's secret.
	secs := int(ttl.Seconds())
	if secs < 1 {
		secs = 1
	}
	res, err := r.do(ctx, "SET", r.prefix+id, string(payload), "EX", strconv.Itoa(secs), "NX")
	if err != nil {
		return "", err
	}
	if string(res) == "null" {
		return "", errors.New("redis: id collision")
	}
	return id, nil
}

func (r *RedisStore) Peek(ctx context.Context, id string) (Meta, error) {
	key := r.prefix + id

	raw, err := r.do(ctx, "GET", key)
	if err != nil {
		return Meta{}, err
	}
	var payload string
	if err := json.Unmarshal(raw, &payload); err != nil || payload == "" {
		return Meta{}, ErrNotFound
	}
	var stored storedSecret
	if err := json.Unmarshal([]byte(payload), &stored); err != nil {
		return Meta{}, ErrNotFound
	}

	ttlRaw, err := r.do(ctx, "TTL", key)
	if err != nil {
		return Meta{}, err
	}
	var ttl int64
	_ = json.Unmarshal(ttlRaw, &ttl)
	if ttl < 0 {
		return Meta{}, ErrNotFound
	}

	return Meta{
		ExpiresAt: time.Now().Add(time.Duration(ttl) * time.Second),
		Views:     stored.Views,
	}, nil
}

func (r *RedisStore) Consume(ctx context.Context, id string) (*Secret, error) {
	key := r.prefix + id

	raw, err := r.do(ctx, "EVAL", burnScript, "1", key)
	if err != nil {
		return nil, err
	}
	// A missing key returns null; the script returns [payload, viewsLeft].
	var pair []string
	if err := json.Unmarshal(raw, &pair); err != nil || len(pair) != 2 {
		return nil, ErrNotFound
	}

	var stored storedSecret
	if err := json.Unmarshal([]byte(pair[0]), &stored); err != nil {
		return nil, ErrNotFound
	}
	ct, err1 := base64.RawURLEncoding.DecodeString(stored.CT)
	iv, err2 := base64.RawURLEncoding.DecodeString(stored.IV)
	if err1 != nil || err2 != nil {
		return nil, ErrNotFound
	}
	viewsLeft, _ := strconv.Atoi(pair[1])
	if viewsLeft < 0 {
		viewsLeft = 0
	}

	return &Secret{CT: ct, IV: iv, Views: viewsLeft}, nil
}

// Ping verifies credentials and reachability at startup.
func (r *RedisStore) Ping(ctx context.Context) error {
	_, err := r.do(ctx, "PING")
	return err
}
