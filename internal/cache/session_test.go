package cache

import (
	"testing"

	"nixpeek/internal/models"
)

func TestSessionCacheGetSetCopy(t *testing.T) {
	c := NewSession()
	original := []models.Package{{AttrPath: "ripgrep"}}
	c.Set("ripgrep", original)

	original[0].AttrPath = "changed"
	got, ok := c.Get("ripgrep")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got[0].AttrPath != "ripgrep" {
		t.Fatalf("expected stored copy to be immutable, got %q", got[0].AttrPath)
	}

	got[0].AttrPath = "tampered"
	again, _ := c.Get("ripgrep")
	if again[0].AttrPath != "ripgrep" {
		t.Fatalf("expected returned copy to be isolated, got %q", again[0].AttrPath)
	}
}
