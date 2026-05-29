package cache

import (
	"sync"

	"nixpeek/internal/models"
)

type Session struct {
	mu sync.RWMutex
	m  map[string][]models.Package
}

func NewSession() *Session {
	return &Session{m: map[string][]models.Package{}}
}

func (s *Session) Get(key string) ([]models.Package, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[key]
	if !ok {
		return nil, false
	}
	out := make([]models.Package, len(v))
	copy(out, v)
	return out, true
}

func (s *Session) Set(key string, v []models.Package) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyV := make([]models.Package, len(v))
	copy(copyV, v)
	s.m[key] = copyV
}
