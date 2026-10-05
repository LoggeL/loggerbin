package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/LoggeL/loggerbin/internal/config"
	bolt "go.etcd.io/bbolt"
	berrors "go.etcd.io/bbolt/errors"
)

var ErrNotFound = errors.New("paste not found")
var ErrQuota = errors.New("storage quota reached")
var pastes = []byte("pastes")
var expirations = []byte("expirations")
var metadata = []byte("metadata")

type Envelope struct {
	Version    int    `json:"version"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}
type Paste struct {
	Envelope
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Store struct {
	db     *bolt.DB
	config config.Config
	now    func() time.Time
}

func Open(c config.Config) (*Store, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(c.DataDir, 0700); err != nil {
		return nil, fmt.Errorf("create private data directory: %w", err)
	}
	if err := os.Chmod(c.DataDir, 0700); err != nil {
		return nil, err
	}
	db, err := bolt.Open(c.DatabasePath(), 0600, &bolt.Options{Timeout: 2 * time.Second, MaxSize: int(c.MaxDatabaseBytes)})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	s := &Store{db: db, config: c, now: time.Now}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{pastes, expirations, metadata} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		m := tx.Bucket(metadata)
		if v := m.Get([]byte("schema")); v != nil && number(v) != 1 {
			return errors.New("unsupported database schema")
		}
		return m.Put([]byte("schema"), encodeUint64(1))
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	if err := os.Chmod(c.DatabasePath(), 0600); err != nil {
		db.Close()
		return nil, err
	}
	for {
		n, err := s.PruneExpired(1000)
		if err != nil {
			db.Close()
			return nil, err
		}
		if n < 1000 {
			break
		}
	}
	return s, nil
}

func ValidID(id string) bool {
	if len(id) != 22 {
		return false
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(id)
	return err == nil && len(b) == 16 && base64.RawURLEncoding.EncodeToString(b) == id
}
func encodeUint64(n uint64) []byte { b := make([]byte, 8); binary.BigEndian.PutUint64(b, n); return b }
func number(b []byte) uint64 {
	if len(b) != 8 {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}
func expiryKey(p *Paste) []byte {
	return append(encodeUint64(uint64(p.ExpiresAt.UnixNano())), []byte(p.ID)...)
}

func (s *Store) Create(e Envelope, ttl time.Duration) (*Paste, error) {
	if ttl < time.Second || ttl > time.Duration(s.config.MaxTTL)*time.Second {
		return nil, errors.New("invalid expiration")
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	p := &Paste{Envelope: e, ID: base64.RawURLEncoding.EncodeToString(random), CreatedAt: now, ExpiresAt: now.Add(ttl)}
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		if _, err := s.prune(tx, now, 100); err != nil {
			return err
		}
		m := tx.Bucket(metadata)
		used := number(m.Get([]byte("bytes")))
		count := number(m.Get([]byte("count")))
		if used+uint64(len(data)) > uint64(s.config.MaxStorageBytes) || count >= uint64(s.config.MaxPastes) {
			return ErrQuota
		}
		b := tx.Bucket(pastes)
		if b.Get([]byte(p.ID)) != nil {
			return errors.New("random identifier collision")
		}
		if err := b.Put([]byte(p.ID), data); err != nil {
			return err
		}
		if err := tx.Bucket(expirations).Put(expiryKey(p), []byte{1}); err != nil {
			return err
		}
		if err := m.Put([]byte("bytes"), encodeUint64(used+uint64(len(data)))); err != nil {
			return err
		}
		return m.Put([]byte("count"), encodeUint64(count+1))
	})
	if errors.Is(err, berrors.ErrMaxSizeReached) {
		return nil, ErrQuota
	}
	return p, err
}

func (s *Store) Get(id string) (*Paste, error) {
	if !ValidID(id) {
		return nil, ErrNotFound
	}
	var p Paste
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(pastes).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		// Decode while the read transaction owns the borrowed bytes. Never mutate them.
		return json.Unmarshal(data, &p)
	})
	if err != nil {
		return nil, err
	}
	if !s.now().Before(p.ExpiresAt) {
		return nil, ErrNotFound
	}
	return &p, nil
}

func (s *Store) prune(tx *bolt.Tx, now time.Time, limit int) (int, error) {
	index := tx.Bucket(expirations)
	b := tx.Bucket(pastes)
	m := tx.Bucket(metadata)
	used := number(m.Get([]byte("bytes")))
	count := number(m.Get([]byte("count")))
	deleted := 0
	for deleted < limit {
		key, _ := index.Cursor().First()
		if len(key) < 8 || binary.BigEndian.Uint64(key[:8]) > uint64(now.UnixNano()) {
			break
		}
		id := key[8:]
		data := b.Get(id)
		if data != nil {
			if uint64(len(data)) > used || count == 0 {
				return deleted, errors.New("database accounting mismatch")
			}
			used -= uint64(len(data))
			count--
			if err := b.Delete(id); err != nil {
				return deleted, err
			}
		}
		if err := index.Delete(key); err != nil {
			return deleted, err
		}
		deleted++
	}
	if err := m.Put([]byte("bytes"), encodeUint64(used)); err != nil {
		return deleted, err
	}
	return deleted, m.Put([]byte("count"), encodeUint64(count))
}
func (s *Store) PruneExpired(limit int) (int, error) {
	var n int
	err := s.db.Update(func(tx *bolt.Tx) error { var err error; n, err = s.prune(tx, s.now(), limit); return err })
	return n, err
}
func (s *Store) Sweep(ctx context.Context, log *slog.Logger) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.PruneExpired(1000); err != nil {
				log.Error("database cleanup failed")
			}
		}
	}
}
func (s *Store) Check() error {
	return s.db.View(func(tx *bolt.Tx) error {
		if tx.Bucket(pastes) == nil {
			return errors.New("missing paste bucket")
		}
		return nil
	})
}
func (s *Store) Close() error { return s.db.Close() }
