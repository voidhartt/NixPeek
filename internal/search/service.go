package search

import (
	"context"
	"strings"
	"time"

	"nixpeek/internal/backend"
	"nixpeek/internal/cache"
	"nixpeek/internal/filters"
	"nixpeek/internal/installed"
	"nixpeek/internal/models"
)

type Service struct {
	backend backend.SearchBackend
	cache   *cache.Session
	checker installed.Checker
	timeout time.Duration
}

func NewService(b backend.SearchBackend, c *cache.Session, checker installed.Checker) *Service {
	return &Service{backend: b, cache: c, checker: checker, timeout: 45 * time.Second}
}

func (s *Service) Search(ctx context.Context, query string, f models.SearchFilters) ([]models.Package, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []models.Package{}, nil
	}

	key := cacheKey(query)
	if cached, ok := s.cache.Get(key); ok {
		return filters.Apply(withInstalled(ctx, s.checker, cached), query, f), nil
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	pkgs, err := s.backend.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	s.cache.Set(key, pkgs)
	return filters.Apply(withInstalled(ctx, s.checker, pkgs), query, f), nil
}

func cacheKey(query string) string {
	return strings.ToLower(strings.TrimSpace(query))
}

func withInstalled(ctx context.Context, checker installed.Checker, pkgs []models.Package) []models.Package {
	out := make([]models.Package, len(pkgs))
	for i, p := range pkgs {
		p.Installed = checker.IsInstalled(ctx, p.AttrPath)
		out[i] = p
	}
	return out
}
