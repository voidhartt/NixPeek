package actions

import "fmt"

func AttrPath(attr string) string {
	return attr
}

func NixProfileInstall(attr string) string {
	return fmt.Sprintf("nix profile install nixpkgs#%s", attr)
}

func NixRun(attr string) string {
	return fmt.Sprintf("nix run nixpkgs#%s", attr)
}
