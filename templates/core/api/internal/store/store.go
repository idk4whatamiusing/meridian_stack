// Package store - everything Redis: sessions, 7-day caches, chat-history fast path.
// Postgres (via the Rust db service) stays primary; this is the speed layer.
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

const SessionPrefix = "session:"

type Store struct {
	rdb *redis.Client
	ttl time.Duration
}

func New(rdb *redis.Client, ttl time.Duration) *Store {
	return &Store{rdb: rdb, ttl: ttl}
}

func Hash(parts ...string) string {
	h := sha256.Sum256([]byte(join(parts)))
	return hex.EncodeToString(h[:])
}

func join(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "\x00"
		}
		out += p
	}
	return out
}

// ---- sessions ----

func (s *Store) CreateSession(ctx context.Context, id, email string) error {
	return s.rdb.Set(ctx, SessionPrefix+id, email, s.ttl).Err()
}

func (s *Store) SessionEmail(ctx context.Context, id string) (string, error) {
	return s.rdb.Get(ctx, SessionPrefix+id).Result()
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	return s.rdb.Del(ctx, SessionPrefix+id).Err()
}

// ---- generic JSON value cache (7d) ----

func (s *Store) GetJSON(ctx context.Context, key string, out any) bool {
	raw, err := s.rdb.Get(ctx, "cache:"+key).Bytes()
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}

func (s *Store) SetJSON(ctx context.Context, key string, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.rdb.Set(ctx, "cache:"+key, raw, s.ttl)
}

func (s *Store) Del(ctx context.Context, keys ...string) {
	for _, k := range keys {
		s.rdb.Del(ctx, "cache:"+k)
	}
}

// ---- chat history fast path: chat:session:{id} ----

type Message struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

func historyKey(sessionID string) string { return "chat:session:" + sessionID }

// HistoryCached reports whether the 7d cache holds this session's messages.
func (s *Store) HistoryCached(ctx context.Context, sessionID string) ([]Message, bool) {
	raw, err := s.rdb.Get(ctx, historyKey(sessionID)).Bytes()
	if err != nil {
		return nil, false
	}
	var msgs []Message
	if json.Unmarshal(raw, &msgs) != nil {
		return nil, false
	}
	return msgs, true
}

func (s *Store) SetHistory(ctx context.Context, sessionID string, msgs []Message) {
	raw, err := json.Marshal(msgs)
	if err != nil {
		return
	}
	s.rdb.Set(ctx, historyKey(sessionID), raw, s.ttl)
}

func (s *Store) DropHistory(ctx context.Context, sessionID string) {
	s.rdb.Del(ctx, historyKey(sessionID))
}
