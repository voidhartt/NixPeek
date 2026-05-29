package backend

import (
	"context"

	"nixpeek/internal/models"
)

type SearchBackend interface {
	Search(ctx context.Context, query string) ([]models.Package, error)
	Name() string
}
