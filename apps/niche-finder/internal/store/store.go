// Package store persists discovered Niches in a single-file, pure-Go embedded
// database (bbolt) so results survive restarts with no external service.
package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.etcd.io/bbolt"

	"github.com/3thanHead/iot_ai/niche-finder/internal/model"
)

var (
	bucket = []byte("niches")
	seeds  = []byte("seeds") // key = lowercased category, value = display form
)

// Store is the Niche repository (plus the managed seed-category list) used by the
// finder and web server.
type Store interface {
	Save(n *model.Niche) error
	Get(id string) (*model.Niche, error)
	List() ([]*model.Niche, error) // ranked by opportunity (best first)
	Delete(id string) error

	Seeds() ([]string, error) // sorted seed categories
	AddSeed(category string) error
	RemoveSeed(category string) error

	Close() error
}

type boltStore struct {
	db *bbolt.DB
	mu sync.Mutex
}

// Open creates/opens the bbolt database at path.
func Open(path string) (Store, error) {
	db, err := bbolt.Open(path, 0o600, nil)
	if err != nil {
		return nil, fmt.Errorf("open bolt: %w", err)
	}
	err = db.Update(func(tx *bbolt.Tx) error {
		if _, e := tx.CreateBucketIfNotExists(bucket); e != nil {
			return e
		}
		_, e := tx.CreateBucketIfNotExists(seeds)
		return e
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return &boltStore{db: db}, nil
}

func (s *boltStore) Save(n *model.Niche) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(n)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucket).Put([]byte(n.ID), data)
	})
}

func (s *boltStore) Get(id string) (*model.Niche, error) {
	var n model.Niche
	err := s.db.View(func(tx *bbolt.Tx) error {
		v := tx.Bucket(bucket).Get([]byte(id))
		if v == nil {
			return fmt.Errorf("niche %q not found", id)
		}
		return json.Unmarshal(v, &n)
	})
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *boltStore) List() ([]*model.Niche, error) {
	var niches []*model.Niche
	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucket).ForEach(func(_, v []byte) error {
			var n model.Niche
			if e := json.Unmarshal(v, &n); e != nil {
				return e
			}
			niches = append(niches, &n)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	// Best opportunity first; tie-break newest first.
	sort.Slice(niches, func(i, k int) bool {
		if niches[i].Opportunity != niches[k].Opportunity {
			return niches[i].Opportunity > niches[k].Opportunity
		}
		return niches[i].CreatedAt.After(niches[k].CreatedAt)
	})
	return niches, nil
}

func (s *boltStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucket).Delete([]byte(id))
	})
}

// Seeds returns the managed seed categories, sorted.
func (s *boltStore) Seeds() ([]string, error) {
	var out []string
	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(seeds).ForEach(func(_, v []byte) error {
			out = append(out, string(v))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// AddSeed stores a category (idempotent, keyed by lowercase). Blank is ignored.
func (s *boltStore) AddSeed(category string) error {
	category = strings.TrimSpace(category)
	if category == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(seeds).Put([]byte(strings.ToLower(category)), []byte(category))
	})
}

func (s *boltStore) RemoveSeed(category string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(seeds).Delete([]byte(strings.ToLower(strings.TrimSpace(category))))
	})
}

func (s *boltStore) Close() error { return s.db.Close() }
