// SPDX-License-Identifier: AGPL-3.0-only

// Package valkey implements the key-value store port on Valkey or another server that speaks
// the Redis protocol (kv = "valkey"; ADR-0003's and ADR-0019's Redis). Its entries are shared by
// every process on the server, so rate limits hold across instances, and it takes the load of
// caches and counters off the database.
//
// An entry is a hash, at <prefix>{kv}:e:<namespace>:<key>, of its value (field v) and its
// expiry in Unix milliseconds by the store's Clock (field e, 0 for never). Entries expire by that
// Clock, rounded up to the next millisecond, as in the other adapters: an entry whose expiry
// has passed behaves as absent. An entry with a TTL also carries the TTL and a grace on the
// server (PEXPIRE), so that the server frees it by itself but not before the store's Clock
// would, and its key is in the sorted set <prefix>{kv}:expiry, scored by its expiry, from
// which DeleteExpired removes the entries that the server has not freed yet. Every key has the
// hash tag {kv}, so that the scripts that write an entry and the sorted set together work on a cluster.
package valkey

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"

	"github.com/mycroblog/my/internal/adapter/valkey"
	"github.com/mycroblog/my/internal/app/kernel"
)

// Store is the Valkey key-value store. It is safe for concurrent use.
type Store struct {
	client valkey.Client
	clock  kernel.Clock
	prefix string // <key prefix>{kv}:
	index  string // the sorted set of the entries with a TTL
}

var _ kernel.KeyValueStore = (*Store)(nil)

// New returns the store on client whose keys start with keyPrefix and whose entries expire by
// clock. Stores with different prefixes on one server do not see each other's entries.
func New(client valkey.Client, keyPrefix string, clock kernel.Clock) *Store {
	prefix := keyPrefix + "{kv}:"
	return &Store{client: client, clock: clock, prefix: prefix, index: prefix + "expiry"}
}

// namespaceEscaper keeps the namespace and the key apart in an entry's key.
var namespaceEscaper = strings.NewReplacer("%", "%25", ":", "%3A")

func (s *Store) key(namespace, key string) string {
	return s.prefix + "e:" + namespaceEscaper.Replace(namespace) + ":" + key
}

func millis(t time.Time) string { return strconv.FormatInt(t.UnixMilli(), 10) }

// serverGrace lengthens the TTL of an entry on the server past its expiry by the store's
// Clock, so that the server, whose clock may run apart from the store's, never frees a live
// entry; the store's Clock alone tells whether an entry is live.
const serverGrace = time.Second

// ceilMillis returns d rounded up to whole milliseconds.
func ceilMillis(d time.Duration) int64 {
	ms := d / time.Millisecond
	if d%time.Millisecond != 0 {
		ms++
	}
	return int64(ms)
}

// expiry returns an entry's expiry and its TTL on the server, both in milliseconds, or "0" and
// "0" for an entry that never expires. The expiry is rounded up to the next millisecond, so
// that a positive TTL never expires early (it compares with the current time truncated to
// the millisecond).
func (s *Store) expiry(ttl time.Duration) (expiresAt, ttlMillis string) {
	if ttl <= 0 {
		return "0", "0"
	}
	at := s.clock.Now().Add(ttl)
	ms := at.UnixMilli()
	if !at.Equal(time.UnixMilli(ms)) {
		ms++
	}
	serverTTL := ttl
	if serverTTL <= math.MaxInt64-serverGrace {
		serverTTL += serverGrace
	}
	return strconv.FormatInt(ms, 10), strconv.FormatInt(ceilMillis(serverTTL), 10)
}

// writeEntry is the Lua that replaces the entry KEYS[1] with value ARGV[1] and expiry ARGV[2]
// (TTL ARGV[3] on the server) and keeps the sorted set KEYS[2] in step; setScript and incrScript
// share it.
const writeEntry = `
redis.call('DEL', KEYS[1])
redis.call('HSET', KEYS[1], 'v', ARGV[1], 'e', ARGV[2])
if ARGV[3] ~= '0' then
  redis.call('PEXPIRE', KEYS[1], ARGV[3])
  redis.call('ZADD', KEYS[2], ARGV[2], KEYS[1])
else
  redis.call('ZREM', KEYS[2], KEYS[1])
end
`

var (
	setScript = valkeygo.NewLuaScript(writeEntry + "return 1")
	// incrScript adds ARGV[4] to the counter KEYS[1], first writing "0" with expiry ARGV[2]
	// (TTL ARGV[3]) when the entry is missing or expired at ARGV[5].
	incrScript = valkeygo.NewLuaScript(`
local e = redis.call('HGET', KEYS[1], 'e')
if (not e) or (e ~= '0' and tonumber(e) <= tonumber(ARGV[5])) then
` + writeEntry + `
end
return redis.call('HINCRBY', KEYS[1], 'v', ARGV[4])
`)
	// swapScript writes the entry KEYS[1] as writeEntry does, or removes it when ARGV[6] is
	// '1', only while it holds ARGV[5] (when ARGV[4] is '1') or no live entry at ARGV[7] (when
	// ARGV[4] is '0'); it returns 1 when it did.
	swapScript = valkeygo.NewLuaScript(`
local f = redis.call('HMGET', KEYS[1], 'v', 'e')
local live = f[1] and f[2] and (f[2] == '0' or tonumber(f[2]) > tonumber(ARGV[7]))
if ARGV[4] == '1' then
  if (not live) or f[1] ~= ARGV[5] then return 0 end
elseif live then
  return 0
end
if ARGV[6] == '1' then
  redis.call('ZREM', KEYS[2], KEYS[1])
  redis.call('DEL', KEYS[1])
  return 1
end
` + writeEntry + `
return 1
`)
	deleteScript = valkeygo.NewLuaScript(`
redis.call('ZREM', KEYS[2], KEYS[1])
return redis.call('DEL', KEYS[1])
`)
	// deleteExpiredScript removes the entries KEYS[2..] whose score in the sorted set KEYS[1] is
	// still at most ARGV[1]: one that IncrBy or Set wrote again since it was listed is kept.
	deleteExpiredScript = valkeygo.NewLuaScript(`
local n = 0
for i = 2, #KEYS do
  local score = redis.call('ZSCORE', KEYS[1], KEYS[i])
  if score and tonumber(score) <= tonumber(ARGV[1]) then
    redis.call('ZREM', KEYS[1], KEYS[i])
    redis.call('DEL', KEYS[i])
    n = n + 1
  end
end
return n
`)
)

// Get implements kernel.KeyValueStore.
func (s *Store) Get(ctx context.Context, namespace, key string) ([]byte, error) {
	fields, err := s.client.Do(ctx, s.client.B().Hmget().Key(s.key(namespace, key)).Field("v", "e").Build()).ToArray()
	if err != nil {
		return nil, fmt.Errorf("kv/valkey: get %s/%s: %w", namespace, key, err)
	}
	if len(fields) != 2 || fields[0].IsNil() || fields[1].IsNil() {
		return nil, fmt.Errorf("kv/valkey: %s/%s: %w", namespace, key, kernel.ErrKeyNotFound)
	}
	e, err := fields[1].AsInt64()
	if err != nil {
		return nil, fmt.Errorf("kv/valkey: get %s/%s: expiry: %w", namespace, key, err)
	}
	if e != 0 && e <= s.clock.Now().UnixMilli() {
		return nil, fmt.Errorf("kv/valkey: %s/%s: %w", namespace, key, kernel.ErrKeyNotFound)
	}
	v, err := fields[0].ToString()
	if err != nil {
		return nil, fmt.Errorf("kv/valkey: get %s/%s: %w", namespace, key, err)
	}
	return []byte(v), nil
}

// Set implements kernel.KeyValueStore.
func (s *Store) Set(ctx context.Context, namespace, key string, value []byte, ttl time.Duration) error {
	expiresAt, ttlMillis := s.expiry(ttl)
	err := setScript.Exec(ctx, s.client, []string{s.key(namespace, key), s.index}, []string{string(value), expiresAt, ttlMillis}).Error()
	if err != nil {
		return fmt.Errorf("kv/valkey: set %s/%s: %w", namespace, key, err)
	}
	return nil
}

// Delete implements kernel.KeyValueStore.
func (s *Store) Delete(ctx context.Context, namespace, key string) error {
	if err := deleteScript.Exec(ctx, s.client, []string{s.key(namespace, key), s.index}, nil).Error(); err != nil {
		return fmt.Errorf("kv/valkey: delete %s/%s: %w", namespace, key, err)
	}
	return nil
}

// IncrBy implements kernel.KeyValueStore.
func (s *Store) IncrBy(ctx context.Context, namespace, key string, n int64, ttl time.Duration) (int64, error) {
	expiresAt, ttlMillis := s.expiry(ttl)
	args := []string{"0", expiresAt, ttlMillis, strconv.FormatInt(n, 10), millis(s.clock.Now())}
	v, err := incrScript.Exec(ctx, s.client, []string{s.key(namespace, key), s.index}, args).AsInt64()
	if err != nil {
		return 0, fmt.Errorf("kv/valkey: %s/%s is not a counter or the server failed: %w", namespace, key, err)
	}
	return v, nil
}

// CompareAndSwap implements kernel.KeyValueStore.
func (s *Store) CompareAndSwap(ctx context.Context, namespace, key string, old, value []byte, ttl time.Duration) (bool, error) {
	expiresAt, ttlMillis := s.expiry(ttl)
	hasOld, del := "0", "0"
	if old != nil {
		hasOld = "1"
	}
	if value == nil {
		del = "1"
	}
	args := []string{string(value), expiresAt, ttlMillis, hasOld, string(old), del, millis(s.clock.Now())}
	n, err := swapScript.Exec(ctx, s.client, []string{s.key(namespace, key), s.index}, args).AsInt64()
	if err != nil {
		return false, fmt.Errorf("kv/valkey: compare and swap %s/%s: %w", namespace, key, err)
	}
	return n == 1, nil
}

// DeleteExpired implements kernel.KeyValueStore. It removes the entries that the server has
// not freed by their TTL yet and the sorted set's records of those it has. It lists a batch
// and removes the listed entries that are still expired; when a concurrent write renewed some
// of them meanwhile, it lists again, so that a short batch still means that none are left.
func (s *Store) DeleteExpired(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	now := millis(s.clock.Now())
	var total int64
	for total < int64(limit) {
		want := int64(limit) - total
		keys, err := s.client.Do(ctx, s.client.B().Zrange().Key(s.index).Min("-inf").Max(now).Byscore().Limit(0, want).Build()).AsStrSlice()
		if err != nil {
			return total, fmt.Errorf("kv/valkey: list expired entries: %w", err)
		}
		if len(keys) == 0 {
			break
		}
		n, err := deleteExpiredScript.Exec(ctx, s.client, append([]string{s.index}, keys...), []string{now}).AsInt64()
		if err != nil {
			return total, fmt.Errorf("kv/valkey: delete expired entries: %w", err)
		}
		total += n
		if int64(len(keys)) < want {
			break // the listing held every expired entry
		}
	}
	return total, nil
}
