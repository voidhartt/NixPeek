package filters

import (
	"sort"
	"strings"

	"nixpeek/internal/models"
)

func Apply(pkgs []models.Package, query string, f models.SearchFilters) []models.Package {
	query = normalizeQuery(query)
	if query == "" {
		return pkgs
	}

	type scoredPackage struct {
		pkg     models.Package
		score   int
		attrKey string
		idx     int
	}

	scored := make([]scoredPackage, 0, len(pkgs))
	for i, p := range pkgs {
		score, ok := scorePackage(p, query, f)
		if ok {
			scored = append(scored, scoredPackage{
				pkg:     p,
				score:   score,
				attrKey: strings.ToLower(strings.TrimSpace(p.AttrPath)),
				idx:     i,
			})
		}
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if len(scored[i].pkg.AttrPath) != len(scored[j].pkg.AttrPath) {
			return len(scored[i].pkg.AttrPath) < len(scored[j].pkg.AttrPath)
		}
		if scored[i].attrKey != scored[j].attrKey {
			return scored[i].attrKey < scored[j].attrKey
		}
		return scored[i].idx < scored[j].idx
	})

	out := make([]models.Package, 0, len(scored))
	for _, item := range scored {
		out = append(out, item.pkg)
	}

	return out
}

func normalizeQuery(query string) string {
	query = strings.TrimSpace(strings.ToLower(query))
	return strings.TrimPrefix(query, "nixpkgs#")
}

func scorePackage(p models.Package, query string, f models.SearchFilters) (int, bool) {
	attr := normalizeField(p.AttrPath)
	name := normalizeField(firstNonEmpty(p.PName, p.Name))
	desc := normalizeField(p.Description)

	if f.ExactAttr {
		return 100_000, attr == query
	}

	allowContains := f.MatchMode != models.MatchPrefix
	best := -1

	if s, ok := scoreField(attr, query, 9_000, allowContains); ok && s > best {
		best = s
	}
	if s, ok := scoreField(name, query, 7_500, allowContains); ok && s > best {
		best = s
	}
	if f.Scope == models.ScopeNameDescription {
		if s, ok := scoreField(desc, query, 2_500, allowContains); ok && s > best {
			best = s
		}
	}

	if best < 0 {
		return 0, false
	}
	return best, true
}

func scoreField(field, query string, fieldWeight int, allowContains bool) (int, bool) {
	if field == "" {
		return 0, false
	}

	if field == query {
		return fieldWeight + 5_000 + proximityBonus(field, query), true
	}
	if strings.HasPrefix(field, query) {
		return fieldWeight + 4_000 + proximityBonus(field, query), true
	}

	if !allowContains {
		return 0, false
	}

	if pos := wordBoundaryIndex(field, query); pos >= 0 {
		return fieldWeight + 3_000 + proximityBonus(field, query) + positionBonus(pos), true
	}
	if pos := strings.Index(field, query); pos >= 0 {
		return fieldWeight + 2_000 + proximityBonus(field, query) + positionBonus(pos), true
	}

	return 0, false
}

func normalizeField(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func proximityBonus(field, query string) int {
	diff := len(field) - len(query)
	if diff < 0 {
		diff = -diff
	}
	b := 300 - diff*5
	if b < 0 {
		return 0
	}
	return b
}

func positionBonus(pos int) int {
	if pos <= 0 {
		return 120
	}
	b := 120 - pos*4
	if b < 0 {
		return 0
	}
	return b
}

func wordBoundaryIndex(field, query string) int {
	if query == "" {
		return -1
	}
	start := 0
	for {
		pos := strings.Index(field[start:], query)
		if pos == -1 {
			return -1
		}
		abs := start + pos
		if abs == 0 {
			return abs
		}
		prev := rune(field[abs-1])
		if isTokenSeparator(prev) {
			return abs
		}
		start = abs + 1
		if start >= len(field) {
			return -1
		}
	}
}

func isTokenSeparator(r rune) bool {
	switch r {
	case '-', '_', '.', '/', '+':
		return true
	default:
		return false
	}
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
