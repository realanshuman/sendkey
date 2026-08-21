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

// burnScript performs the read-and-decrement as one indivisible server side
// operation. Doing it as separate GET and SET calls would open a window
// where two concurrent readers both observe the last view and both receive
// the secret — the exact failure this product exists to prevent.
//
// On the last view the payload is blanked rather than deleted, leaving a
// tombstone that carries only the open timestamps so the sender can still
// collect a receipt. An already-burned record (views <= 0) returns nil, so
// the caller sees the same not-found it saw when the key was deleted.
//
// ARGV[1] is the open time in unix millis, supplied by the caller: Lua's
// clock is off limits for replicated scripts, and the app already trusts its
// own clock for every other TTL computation.
//
// KEEPTTL preserves the original expiry so a multi-view secret cannot have
// its lifetime silently extended by being read.
// putScript stores a secret and bumps the aggregate counters in the same
// atomic step, because the Upstash REST transport pays one round trip per
// command and a create would otherwise cost seven. The counters live in one
// small hash with no TTL; HSETNX pins 'since' on the first write so /stats
// can say when its window started. ARGV[7] marks an ask answer landing.
//
// The SET happens first: if NX loses the race nothing is counted, so the
// totals only describe secrets that were actually accepted.
const putScript = `
local ok = redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2], 'NX')
if not ok then return nil end
redis.call('HSETNX', KEYS[2], 'since', ARGV[6])
redis.call('HINCRBY', KEYS[2], 'created', 1)
redis.call('HINCRBY', KEYS[2], 'sealed', ARGV[3])
redis.call('HINCRBY', KEYS[2], ARGV[4], 1)
redis.call('HINCRBY', KEYS[2], ARGV[5], 1)
if ARGV[7] == '1' then
  redis.call('HINCRBY', KEYS[2], 'answered', 1)
end
return 1
`

const burnScript = `
local v = redis.call('GET', KEYS[1])
if not v then return nil end
local d = cjson.decode(v)
if d.views <= 0 then return nil end
d.opens = d.opens or {}
table.insert(d.opens, tonumber(ARGV[1]))
d.views = d.views - 1
redis.call('HINCRBY', KEYS[2], 'opened', 1)
if d.views <= 0 then
  d.ct = ''
  d.iv = ''
  redis.call('HINCRBY', KEYS[2], 'burned', 1)
end
redis.call('SET', KEYS[1], cjson.encode(d), 'KEEPTTL')
return {v, tostring(d.views)}
`

type storedSecret struct {
	CT    string  `json:"ct"`              // base64url
	IV    string  `json:"iv"`              // base64url
	Views int     `json:"views"`           // remaining reads
	Opens []int64 `json:"opens,omitempty"` // unix millis, one per read
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
	res, err := r.putCounted(ctx, id, payload, ttl, len(ct), views, false)
	if err != nil {
		return "", err
	}
	if string(res) == "null" {
		return "", errors.New("redis: id collision")
	}
	return id, nil
}

// putCounted runs putScript: the SET NX plus the stats counters, one round
// trip, nothing counted unless the SET wins.
func (r *RedisStore) putCounted(ctx context.Context, id string, payload []byte, ttl time.Duration, ctLen, views int, answered bool) (json.RawMessage, error) {
	secs := int(ttl.Seconds())
	if secs < 1 {
		secs = 1
	}
	ans := "0"
	if answered {
		ans = "1"
	}
	return r.do(ctx, "EVAL", putScript, "2", r.prefix+id, r.prefix+"stats",
		string(payload), strconv.Itoa(secs), strconv.Itoa(ctLen),
		ttlBucket(ttl), viewsBucket(views), strconv.FormatInt(time.Now().UnixMilli(), 10), ans)
}

// PutAt stores under a caller-chosen id. SET NX gives first-write-wins on
// Redis itself, so two concurrent answers to one ask race safely.
func (r *RedisStore) PutAt(ctx context.Context, id string, ct, iv []byte, ttl time.Duration, views int) error {
	if views < 1 {
		views = 1
	}
	payload, err := json.Marshal(storedSecret{
		CT:    base64.RawURLEncoding.EncodeToString(ct),
		IV:    base64.RawURLEncoding.EncodeToString(iv),
		Views: views,
	})
	if err != nil {
		return err
	}
	res, err := r.putCounted(ctx, id, payload, ttl, len(ct), views, true)
	if err != nil {
		return err
	}
	if string(res) == "null" {
		return ErrExists
	}
	return nil
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
	// A burned tombstone is invisible to every recipient-facing caller.
	// Checked before the TTL round trip, which would otherwise be wasted.
	if stored.Views <= 0 {
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

	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	raw, err := r.do(ctx, "EVAL", burnScript, "2", key, r.prefix+"stats", now)
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

// Receipts reports when this secret was opened, including after it burned:
// Redis expiry removes the tombstone on its own, so a missing key is the
// only not-found case here.
func (r *RedisStore) Receipts(ctx context.Context, id string) ([]time.Time, error) {
	raw, err := r.do(ctx, "GET", r.prefix+id)
	if err != nil {
		return nil, err
	}
	var payload string
	if err := json.Unmarshal(raw, &payload); err != nil || payload == "" {
		return nil, ErrNotFound
	}
	var stored storedSecret
	if err := json.Unmarshal([]byte(payload), &stored); err != nil {
		return nil, ErrNotFound
	}

	opens := make([]time.Time, 0, len(stored.Opens))
	for _, ms := range stored.Opens {
		opens = append(opens, time.UnixMilli(ms))
	}
	return opens, nil
}

// Ping verifies credentials and reachability at startup.
// Stats reads the aggregate counter hash. An absent hash is a valid zero
// window that simply has not started yet.
func (r *RedisStore) Stats(ctx context.Context) (Stats, error) {
	raw, err := r.do(ctx, "HGETALL", r.prefix+"stats")
	if err != nil {
		return Stats{}, err
	}
	// Upstash returns the hash flattened: [field, value, field, value, ...]
	var flat []string
	if err := json.Unmarshal(raw, &flat); err != nil {
		return Stats{}, fmt.Errorf("redis: bad stats shape: %w", err)
	}
	var st Stats
	for i := 0; i+1 < len(flat); i += 2 {
		n, _ := strconv.ParseInt(flat[i+1], 10, 64)
		switch flat[i] {
		case "created":
			st.Created = n
		case "opened":
			st.Opened = n
		case "burned":
			st.Burned = n
		case "answered":
			st.Answered = n
		case "sealed":
			st.Sealed = n
		case "ttlHour":
			st.TTLHour = n
		case "ttlDay":
			st.TTLDay = n
		case "ttlWeek":
			st.TTLWeek = n
		case "views1":
			st.Views1 = n
		case "views2":
			st.Views2 = n
		case "views5":
			st.Views5 = n
		case "since":
			st.Since = time.UnixMilli(n).UTC()
		}
	}
	return st, nil
}

func (r *RedisStore) Ping(ctx context.Context) error {
	_, err := r.do(ctx, "PING")
	return err
}
