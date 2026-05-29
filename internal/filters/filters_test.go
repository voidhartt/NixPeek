package filters

import (
	"testing"

	"nixpeek/internal/models"
)

func TestApplyContainsAndPrefixAndExact(t *testing.T) {
	pkgs := []models.Package{
		{AttrPath: "firefox", PName: "firefox", Description: "Web browser"},
		{AttrPath: "floorp", PName: "floorp", Description: "Forked browser"},
		{AttrPath: "ripgrep", PName: "ripgrep", Description: "Search tool"},
	}

	contains := Apply(pkgs, "bro", models.SearchFilters{Scope: models.ScopeNameDescription, MatchMode: models.MatchContains})
	if len(contains) != 2 {
		t.Fatalf("expected 2 contains matches, got %d", len(contains))
	}

	prefix := Apply(pkgs, "fi", models.SearchFilters{Scope: models.ScopeNameOnly, MatchMode: models.MatchPrefix})
	if len(prefix) != 1 || prefix[0].AttrPath != "firefox" {
		t.Fatalf("unexpected prefix result: %+v", prefix)
	}

	exact := Apply(pkgs, "ripgrep", models.SearchFilters{ExactAttr: true})
	if len(exact) != 1 || exact[0].AttrPath != "ripgrep" {
		t.Fatalf("unexpected exact result: %+v", exact)
	}
}

func TestApplyRanksMostRelevantFirst(t *testing.T) {
	pkgs := []models.Package{
		{AttrPath: "steam-run", PName: "steam-run", Description: "Run Steam apps"},
		{AttrPath: "protonup-rs", PName: "protonup-rs", Description: "Steam helper"},
		{AttrPath: "steam", PName: "steam", Description: "Steam client"},
		{AttrPath: "my-steam-tool", PName: "my-steam-tool", Description: "Tooling"},
	}

	out := Apply(pkgs, "steam", models.SearchFilters{
		Scope:     models.ScopeNameDescription,
		MatchMode: models.MatchContains,
	})
	if len(out) != 4 {
		t.Fatalf("expected 4 results, got %d", len(out))
	}
	if out[0].AttrPath != "steam" {
		t.Fatalf("expected exact match first, got %q", out[0].AttrPath)
	}
	if out[1].AttrPath != "steam-run" {
		t.Fatalf("expected prefix match second, got %q", out[1].AttrPath)
	}
}

func TestApplyPrefixModeDoesNotIncludeContainsOnly(t *testing.T) {
	pkgs := []models.Package{
		{AttrPath: "steam", PName: "steam"},
		{AttrPath: "my-steam-tool", PName: "my-steam-tool"},
	}

	out := Apply(pkgs, "steam", models.SearchFilters{
		Scope:     models.ScopeNameDescription,
		MatchMode: models.MatchPrefix,
	})
	if len(out) != 1 || out[0].AttrPath != "steam" {
		t.Fatalf("unexpected prefix-only results: %+v", out)
	}
}
