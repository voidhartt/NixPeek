package backend

import "testing"

func TestParseNixSearchJSON(t *testing.T) {
	raw := []byte(`{
		"nixpkgs#ripgrep": {
			"pname": "ripgrep",
			"version": "14.1.0",
			"description": "line-oriented search tool",
			"longDescription": "very fast recursive search",
			"homepage": ["https://github.com/BurntSushi/ripgrep"],
			"license": {"spdxId": "MIT"},
			"platforms": ["x86_64-linux", "aarch64-linux"]
		}
	}`)

	pkgs, err := ParseNixSearchJSON(raw)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}

	p := pkgs[0]
	if p.AttrPath != "ripgrep" {
		t.Fatalf("unexpected attrPath: %s", p.AttrPath)
	}
	if p.License != "MIT" {
		t.Fatalf("unexpected license: %s", p.License)
	}
	if p.Homepage == "" {
		t.Fatal("expected homepage")
	}
	if len(p.Platforms) != 2 {
		t.Fatalf("expected platforms, got %+v", p.Platforms)
	}
}
