package installed

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Checker interface {
	IsInstalled(ctx context.Context, attrPath string) bool
}

type NixProfileChecker struct {
	mu        sync.RWMutex
	installed map[string]struct{}
	aliases   map[string]struct{}
	loadedAt  time.Time
	ttl       time.Duration
}

func NewNixProfileChecker() *NixProfileChecker {
	return &NixProfileChecker{ttl: 20 * time.Second}
}

func (c *NixProfileChecker) IsInstalled(ctx context.Context, attrPath string) bool {
	_ = ctx // installed-state loading should not be tied to cancelable live-search contexts
	if err := c.ensureLoaded(context.Background()); err != nil {
		return false
	}

	candidates := attrCandidates(attrPath)
	if len(candidates) == 0 {
		return false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, cand := range candidates {
		if _, ok := c.installed[cand]; ok {
			return true
		}
		if _, ok := c.aliases[cand]; ok {
			return true
		}
	}

	for key := range c.installed {
		for _, cand := range candidates {
			if strings.HasSuffix(key, "."+cand) {
				return true
			}
		}
	}

	return false
}

func (c *NixProfileChecker) ensureLoaded(ctx context.Context) error {
	c.mu.RLock()
	fresh := c.installed != nil && c.aliases != nil && time.Since(c.loadedAt) < c.ttl
	c.mu.RUnlock()
	if fresh {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.installed != nil && c.aliases != nil && time.Since(c.loadedAt) < c.ttl {
		return nil
	}

	installed, aliases, err := loadInstalled(ctx)
	if err != nil {
		return err
	}

	c.installed = installed
	c.aliases = aliases
	c.loadedAt = time.Now()
	return nil
}

func loadInstalled(ctx context.Context) (map[string]struct{}, map[string]struct{}, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "nix", "profile", "list", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, err
	}

	installed, aliases, hmStorePaths, err := parseInstalledSet(out)
	if err != nil {
		return nil, nil, err
	}

	hmStorePaths = append(hmStorePaths, discoverHomeManagerStorePaths()...)
	hmStorePaths = uniqueStrings(hmStorePaths)

	for _, hmPath := range hmStorePaths {
		hmAliases, err := loadHomeManagerAliases(ctx, hmPath)
		if err != nil {
			continue
		}
		mergeSet(aliases, hmAliases)
	}

	mergeSet(aliases, loadProfileBinAliases())

	return installed, aliases, nil
}

type profileList struct {
	Elements map[string]profileElement `json:"elements"`
}

type profileElement struct {
	AttrPath   string   `json:"attrPath"`
	StorePaths []string `json:"storePaths"`
}

func parseInstalledSet(raw []byte) (map[string]struct{}, map[string]struct{}, []string, error) {
	var p profileList
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, nil, nil, err
	}

	installed := map[string]struct{}{}
	aliases := map[string]struct{}{}
	hmStorePaths := []string{}
	seenHM := map[string]struct{}{}

	for name, element := range p.Elements {
		addNormalized(installed, name)
		addNormalized(installed, element.AttrPath)

		attr := strings.TrimSpace(element.AttrPath)
		if trimmed, ok := trimSystemPrefix(attr); ok {
			addNormalized(installed, trimmed)
		}

		for _, storePath := range element.StorePaths {
			addStorePathAliases(aliases, storePath)
			if isHomeManagerPath(storePath) {
				if _, ok := seenHM[storePath]; !ok {
					seenHM[storePath] = struct{}{}
					hmStorePaths = append(hmStorePaths, storePath)
				}
			}
		}
	}

	return installed, aliases, hmStorePaths, nil
}

func isHomeManagerPath(storePath string) bool {
	storePath = strings.TrimSpace(storePath)
	if storePath == "" {
		return false
	}
	base := strings.ToLower(filepath.Base(storePath))
	return strings.HasSuffix(base, "-home-manager-path") ||
		strings.HasSuffix(base, "-home-path") ||
		strings.Contains(base, "home-manager-path")
}

type derivationMap map[string]derivationEntry

type derivationShowRoot struct {
	Derivations map[string]derivationEntry `json:"derivations"`
}

type derivationEntry struct {
	StructuredAttrs structuredAttrs `json:"structuredAttrs"`
}

type structuredAttrs struct {
	ChosenOutputs []chosenOutput `json:"chosenOutputs"`
}

type chosenOutput struct {
	Paths []string `json:"paths"`
}

func loadHomeManagerAliases(ctx context.Context, hmStorePath string) (map[string]struct{}, error) {
	aliases := map[string]struct{}{}

	refsAliases, refsErr := loadHomeManagerAliasesFromReferences(ctx, hmStorePath)
	if refsErr == nil {
		mergeSet(aliases, refsAliases)
	}

	// Fallback for environments where references are unavailable or too sparse.
	if len(aliases) == 0 {
		drvAliases, drvErr := loadHomeManagerAliasesFromDerivation(ctx, hmStorePath)
		if drvErr == nil {
			mergeSet(aliases, drvAliases)
		}
		if refsErr != nil && drvErr != nil {
			return nil, refsErr
		}
	}

	return aliases, nil
}

func loadHomeManagerAliasesFromReferences(ctx context.Context, hmStorePath string) (map[string]struct{}, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "nix-store", "-q", "--references", hmStorePath)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	aliases := map[string]struct{}{}
	for _, line := range strings.Split(string(out), "\n") {
		addStorePathAliases(aliases, line)
	}
	return aliases, nil
}

func discoverHomeManagerStorePaths() []string {
	paths := []string{}
	seen := map[string]struct{}{}
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}

	for _, profilePath := range discoverProfilePaths() {
		resolved, ok := resolvePath(profilePath)
		if !ok {
			continue
		}
		if isHomeManagerPath(resolved) {
			add(resolved)
		}
		homePath := filepath.Join(resolved, "home-path")
		if hpResolved, ok := resolvePath(homePath); ok {
			add(hpResolved)
		}
	}

	return paths
}

func loadProfileBinAliases() map[string]struct{} {
	aliases := map[string]struct{}{}
	for _, dir := range discoverProfileBinDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			addNormalized(aliases, entry.Name())
		}
	}
	return aliases
}

func discoverProfileBinDirs() []string {
	dirs := []string{}
	seen := map[string]struct{}{}
	addDir := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			return
		}
		seen[p] = struct{}{}
		dirs = append(dirs, p)
	}

	for _, profilePath := range discoverProfilePaths() {
		addDir(filepath.Join(profilePath, "bin"))
		if resolved, ok := resolvePath(profilePath); ok {
			addDir(filepath.Join(resolved, "bin"))
			homePath := filepath.Join(resolved, "home-path")
			if hpResolved, ok := resolvePath(homePath); ok {
				addDir(filepath.Join(hpResolved, "bin"))
			}
		}
	}

	addDir("/run/current-system/sw/bin")
	return dirs
}

func discoverProfilePaths() []string {
	paths := []string{}
	seen := map[string]struct{}{}
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}

	home := strings.TrimSpace(os.Getenv("HOME"))
	if home != "" {
		add(filepath.Join(home, ".nix-profile"))
		add(filepath.Join(home, ".local/state/nix/profiles/profile"))
		add(filepath.Join(home, ".local/state/nix/profiles/home-manager"))
	}

	user := strings.TrimSpace(os.Getenv("USER"))
	if user != "" {
		add(filepath.Join("/nix/var/nix/profiles/per-user", user, "profile"))
		add(filepath.Join("/nix/var/nix/profiles/per-user", user, "home-manager"))
	}

	add(os.Getenv("HOME_MANAGER_PROFILE"))
	return paths
}

func resolvePath(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	if resolved == "" {
		return "", false
	}
	return resolved, true
}

func uniqueStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func loadHomeManagerAliasesFromDerivation(ctx context.Context, hmStorePath string) (map[string]struct{}, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	deriverCmd := exec.CommandContext(cmdCtx, "nix-store", "-q", "--deriver", hmStorePath)
	deriverOut, err := deriverCmd.Output()
	if err != nil {
		return nil, err
	}
	deriver := strings.TrimSpace(string(deriverOut))
	if deriver == "" || deriver == "unknown-deriver" {
		return map[string]struct{}{}, nil
	}

	showCmd := exec.CommandContext(cmdCtx, "nix", "derivation", "show", deriver)
	showOut, err := showCmd.Output()
	if err != nil {
		return nil, err
	}

	entries, err := parseDerivationEntries(showOut)
	if err != nil {
		return nil, err
	}

	aliases := map[string]struct{}{}
	for _, entry := range entries {
		for _, chosen := range entry.StructuredAttrs.ChosenOutputs {
			for _, p := range chosen.Paths {
				addStorePathAliases(aliases, p)
			}
		}
	}
	return aliases, nil
}

func parseDerivationEntries(raw []byte) ([]derivationEntry, error) {
	var root derivationShowRoot
	if err := json.Unmarshal(raw, &root); err == nil && len(root.Derivations) > 0 {
		out := make([]derivationEntry, 0, len(root.Derivations))
		for _, entry := range root.Derivations {
			out = append(out, entry)
		}
		return out, nil
	}

	var dm derivationMap
	if err := json.Unmarshal(raw, &dm); err != nil {
		return nil, err
	}
	out := make([]derivationEntry, 0, len(dm))
	for _, entry := range dm {
		out = append(out, entry)
	}
	return out, nil
}

var versionSuffixRe = regexp.MustCompile(`-[0-9][0-9A-Za-z.+_-]*$`)

func addStorePathAliases(dst map[string]struct{}, storePath string) {
	storePath = strings.TrimSpace(storePath)
	if storePath == "" {
		return
	}

	base := filepath.Base(storePath)
	if base == "" {
		return
	}

	name := base
	if i := strings.IndexRune(name, '-'); i > 0 && i < len(name)-1 {
		if isLikelyStoreHash(name[:i]) {
			name = name[i+1:]
		}
	}

	if name == "" {
		return
	}

	variants := []string{name}
	for _, suffix := range []string{"-nixgl-wrapper", "-wrapper", "-man", "-bin", "-lib", "-dev", "-doc"} {
		if strings.HasSuffix(name, suffix) {
			variants = append(variants, strings.TrimSuffix(name, suffix))
		}
	}

	for _, v := range variants {
		v = versionSuffixRe.ReplaceAllString(v, "")
		addNormalized(dst, v)
		if i := strings.LastIndex(v, "-"); i > 0 {
			addNormalized(dst, v[i+1:])
		}
	}
}

func isLikelyStoreHash(s string) bool {
	if len(s) < 20 || len(s) > 40 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

func addNormalized(dst map[string]struct{}, value string) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return
	}
	dst[value] = struct{}{}
	dst[strings.ReplaceAll(value, "_", "-")] = struct{}{}
}

func attrCandidates(attrPath string) []string {
	attrPath = strings.TrimSpace(strings.ToLower(attrPath))
	if attrPath == "" {
		return nil
	}

	seen := map[string]struct{}{}
	out := []string{}
	add := func(v string) {
		v = strings.TrimSpace(strings.ToLower(v))
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}

	add(attrPath)
	add(strings.ReplaceAll(attrPath, "_", "-"))
	if idx := strings.LastIndex(attrPath, "."); idx >= 0 && idx < len(attrPath)-1 {
		last := attrPath[idx+1:]
		add(last)
		add(strings.ReplaceAll(last, "_", "-"))
	}
	return out
}

func mergeSet(dst, src map[string]struct{}) {
	for k := range src {
		dst[k] = struct{}{}
	}
}

func trimSystemPrefix(attr string) (string, bool) {
	parts := strings.Split(attr, ".")
	if len(parts) < 3 {
		return "", false
	}

	if parts[0] == "legacyPackages" || parts[0] == "packages" {
		return strings.Join(parts[2:], "."), true
	}
	return "", false
}
