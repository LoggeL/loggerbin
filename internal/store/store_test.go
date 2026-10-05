package store

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/LoggeL/loggerbin/internal/config"
	bolt "go.etcd.io/bbolt"
)

func encrypted(t *testing.T, plain []byte) (Envelope, []byte) {
	t.Helper()
	key := make([]byte, 32)
	rand.Read(key)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, 12)
	rand.Read(nonce)
	data := gcm.Seal(nil, nonce, plain, []byte("loggerbin:v1"))
	return Envelope{Version: 1, Nonce: base64.RawURLEncoding.EncodeToString(nonce), Ciphertext: base64.RawURLEncoding.EncodeToString(data)}, key
}
func fixture(t *testing.T, c config.Config) *Store {
	t.Helper()
	s, err := Open(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCiphertextNeverMutatesOrLeaksAtLargeSizes(t *testing.T) {
	c := config.Defaults()
	c.DataDir = t.TempDir()
	c.MaxPasteBytes = 1 << 20
	s := fixture(t, c)
	marker := []byte("LOGGERBIN_SYNTHETIC_PRIVATE_MARKER")
	plain := append(bytes.Repeat([]byte("L"), 900000), marker...)
	e, key := encrypted(t, plain)
	p, err := s.Create(e, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		got, err := s.Get(p.ID)
		if err != nil || got.Ciphertext != e.Ciphertext {
			t.Fatalf("repeated read mutated data: %v", err)
		}
	}
	data, err := os.ReadFile(c.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, marker) || bytes.Contains(data, key) || bytes.Contains(data, []byte(base64.RawURLEncoding.EncodeToString(key))) {
		t.Fatal("plaintext or encryption key found on disk")
	}
	if !bytes.Contains(data, []byte(e.Ciphertext[:100])) {
		t.Fatal("expected opaque ciphertext in database")
	}
}
func TestExpiryRemovesAllMetadataAndReleasesQuota(t *testing.T) {
	c := config.Defaults()
	c.DataDir = t.TempDir()
	c.MaxPastes = 1
	s := fixture(t, c)
	now := time.Now()
	s.now = func() time.Time { return now }
	e, _ := encrypted(t, []byte("first"))
	p, err := s.Create(e, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(e, time.Hour); !errors.Is(err, ErrQuota) {
		t.Fatal("count quota not enforced")
	}
	now = now.Add(2 * time.Second)
	if _, err := s.Get(p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("expired paste returned")
	}
	if n, err := s.PruneExpired(100); err != nil || n != 1 {
		t.Fatalf("cleanup failed: %d %v", n, err)
	}
	err = s.db.View(func(tx *bolt.Tx) error {
		if tx.Bucket(pastes).Stats().KeyN != 0 || tx.Bucket(expirations).Stats().KeyN != 0 || number(tx.Bucket(metadata).Get([]byte("bytes"))) != 0 || number(tx.Bucket(metadata).Get([]byte("count"))) != 0 {
			t.Fatal("expired metadata retained")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(e, time.Hour); err != nil {
		t.Fatal("cleanup did not release quota")
	}
}
func TestConcurrentQuotaIsAtomic(t *testing.T) {
	c := config.Defaults()
	c.DataDir = t.TempDir()
	c.MaxPastes = 2
	s := fixture(t, c)
	e, _ := encrypted(t, []byte("bounded"))
	var wg sync.WaitGroup
	var mu sync.Mutex
	created := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Create(e, time.Hour)
			if err == nil {
				mu.Lock()
				created++
				mu.Unlock()
			} else if !errors.Is(err, ErrQuota) {
				t.Errorf("unexpected create error: %v", err)
			}
		}()
	}
	wg.Wait()
	if created != 2 {
		t.Fatalf("quota race: created %d", created)
	}
}
func TestPersistenceAndDatabasePermissions(t *testing.T) {
	c := config.Defaults()
	c.DataDir = t.TempDir()
	s, err := Open(c)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := encrypted(t, []byte("survives restart"))
	p, err := s.Create(e, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(c)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got, err := s.Get(p.ID); err != nil || got.Ciphertext != e.Ciphertext {
		t.Fatal("restart lost record")
	}
	info, _ := os.Stat(c.DatabasePath())
	if info.Mode().Perm() != 0600 {
		t.Fatal("database permissions are not private")
	}
}

func TestHardDatabaseFileQuota(t *testing.T) {
	c := config.Defaults()
	c.DataDir = t.TempDir()
	c.MaxDatabaseBytes = 2 << 20
	s := fixture(t, c)
	e, _ := encrypted(t, bytes.Repeat([]byte("Q"), 50000))
	reached := false
	for i := 0; i < 100; i++ {
		_, err := s.Create(e, time.Hour)
		if errors.Is(err, ErrQuota) {
			reached = true
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reached {
		t.Fatal("physical database quota not reached")
	}
	info, err := os.Stat(c.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > c.MaxDatabaseBytes {
		t.Fatalf("file quota exceeded: %d", info.Size())
	}
	if err := s.Check(); err != nil {
		t.Fatal("quota rejection damaged database")
	}
}
