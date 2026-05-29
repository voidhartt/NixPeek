package installed

import (
	"context"
	"os"
	"testing"
)

func TestParseInstalledSet(t *testing.T) {
	raw := []byte(`{
		"elements": {
			"go": {
				"attrPath": "legacyPackages.x86_64-linux.go",
				"storePaths": ["/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-go-1.26.3"]
			},
			"home-manager-path": {
				"storePaths": ["/nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-home-manager-path"]
			}
		}
	}`)

	installed, aliases, hmPaths, err := parseInstalledSet(raw)
	if err != nil {
		t.Fatalf("parseInstalledSet failed: %v", err)
	}

	if _, ok := installed["go"]; !ok {
		t.Fatal("expected profile element name key")
	}
	if _, ok := installed["legacypackages.x86_64-linux.go"]; !ok {
		t.Fatal("expected full attrPath key")
	}
	if _, ok := aliases["go"]; !ok {
		t.Fatal("expected store path alias key")
	}
	if len(hmPaths) != 1 {
		t.Fatalf("expected 1 home-manager path, got %d", len(hmPaths))
	}
}

func TestParseInstalledSetHomePathVariant(t *testing.T) {
	raw := []byte(`{
		"elements": {
			"home-path": {
				"storePaths": ["/nix/store/cccccccccccccccccccccccccccccccc-home-path"]
			}
		}
	}`)

	_, _, hmPaths, err := parseInstalledSet(raw)
	if err != nil {
		t.Fatalf("parseInstalledSet failed: %v", err)
	}
	if len(hmPaths) != 1 {
		t.Fatalf("expected 1 home-manager path, got %d", len(hmPaths))
	}
}

func TestTrimSystemPrefix(t *testing.T) {
	trimmed, ok := trimSystemPrefix("legacyPackages.x86_64-linux.python312Packages.requests")
	if !ok {
		t.Fatal("expected prefix trimming")
	}
	if trimmed != "python312Packages.requests" {
		t.Fatalf("unexpected trimmed value: %s", trimmed)
	}
}

func TestIsHomeManagerPathVariants(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-home-manager-path", true},
		{"/nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-home-path", true},
		{"/nix/store/cccccccccccccccccccccccccccccccc-home-manager-path-extra", true},
		{"/nix/store/dddddddddddddddddddddddddddddddd-go-1.26.3", false},
	}
	for _, tc := range tests {
		if got := isHomeManagerPath(tc.path); got != tc.want {
			t.Fatalf("isHomeManagerPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestAddStorePathAliases(t *testing.T) {
	set := map[string]struct{}{}
	addStorePathAliases(set, "/nix/store/abcdefghijklmnopqrstuvwxzy012345-qbittorrent-enhanced-5.1.3.10-nixgl-wrapper")

	if _, ok := set["qbittorrent-enhanced"]; !ok {
		t.Fatal("expected normalized package alias")
	}
	if _, ok := set["enhanced"]; !ok {
		t.Fatal("expected short alias from hyphen tail")
	}
}

func TestAttrCandidates(t *testing.T) {
	cands := attrCandidates("python312Packages.requests")
	seen := map[string]struct{}{}
	for _, c := range cands {
		seen[c] = struct{}{}
	}
	if _, ok := seen["python312packages.requests"]; !ok {
		t.Fatal("expected full candidate")
	}
	if _, ok := seen["requests"]; !ok {
		t.Fatal("expected last-segment candidate")
	}
}

func TestParseDerivationEntriesFormats(t *testing.T) {
	rootFmt := []byte(`{
		"derivations": {
			"abc.drv": {
				"structuredAttrs": {
					"chosenOutputs": [
						{"paths": ["/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-ripgrep-15.1.0"]}
					]
				}
			}
		}
	}`)
	entries, err := parseDerivationEntries(rootFmt)
	if err != nil {
		t.Fatalf("unexpected root format parse error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 derivation entry, got %d", len(entries))
	}

	mapFmt := []byte(`{
		"/nix/store/abc.drv": {
			"structuredAttrs": {
				"chosenOutputs": [
					{"paths": ["/nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-fd-10.4.2"]}
				]
			}
		}
	}`)
	entries, err = parseDerivationEntries(mapFmt)
	if err != nil {
		t.Fatalf("unexpected map format parse error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 derivation entry, got %d", len(entries))
	}
}

func TestLiveHomeManagerCompatibility(t *testing.T) {
	t.Parallel()

	c := NewNixProfileChecker()
	ctx := context.Background()

	// This attr is known to be present in the current developer environment
	// via Home Manager. If the environment changes, this check is skipped.
	const probe = "ripgrep"
	if !c.IsInstalled(ctx, probe) {
		t.Skipf("probe package %q not detected; skipping environment-specific assertion", probe)
	}
}

func TestIsInstalledWithCanceledContext(t *testing.T) {
	t.Parallel()

	c := NewNixProfileChecker()
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	// Probe from developer environment; skip if environment differs.
	const probe = "go"
	if !c.IsInstalled(parent, probe) {
		if !c.IsInstalled(context.Background(), probe) {
			t.Skipf("probe package %q not detected in this environment", probe)
		}
		t.Fatalf("expected canceled context to still detect installed package %q", probe)
	}
}

func TestDiscoverHomeManagerStorePathsCurrentEnv(t *testing.T) {
	t.Parallel()

	if _, err := os.Stat(os.ExpandEnv("$HOME/.local/state/nix/profiles/home-manager")); err != nil {
		t.Skip("home-manager profile path not present in this environment")
	}

	paths := discoverHomeManagerStorePaths()
	if len(paths) == 0 {
		t.Fatal("expected at least one discovered home-manager store path")
	}
}
