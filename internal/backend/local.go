package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"nixpeek/internal/models"
)

type LocalBackend struct{}

func NewLocalBackend() *LocalBackend { return &LocalBackend{} }

func (b *LocalBackend) Name() string { return "local" }

func (b *LocalBackend) Search(ctx context.Context, query string) ([]models.Package, error) {
	if strings.TrimSpace(query) == "" {
		return []models.Package{}, nil
	}

	cmd := exec.CommandContext(ctx, "nix", "search", "nixpkgs", query, "--json")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, ctx.Err()
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("nix search timed out; try typing a longer query")
		}
		return nil, fmt.Errorf("nix search failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	pkgs, err := ParseNixSearchJSON(out)
	if err != nil {
		return nil, err
	}
	return pkgs, nil
}

type nixSearchRecord struct {
	PName           string      `json:"pname"`
	Name            string      `json:"name"`
	Version         string      `json:"version"`
	Description     string      `json:"description"`
	LongDescription string      `json:"longDescription"`
	Homepage        interface{} `json:"homepage"`
	License         interface{} `json:"license"`
	Platforms       interface{} `json:"platforms"`
}

func ParseNixSearchJSON(raw []byte) ([]models.Package, error) {
	raw = extractJSONObject(raw)
	var root map[string]nixSearchRecord
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse nix json: %w", err)
	}

	pkgs := make([]models.Package, 0, len(root))
	for attr, rec := range root {
		pkgs = append(pkgs, models.Package{
			AttrPath:        strings.TrimPrefix(attr, "nixpkgs#"),
			Name:            rec.Name,
			PName:           rec.PName,
			Version:         rec.Version,
			Description:     rec.Description,
			LongDescription: rec.LongDescription,
			Homepage:        flattenToString(rec.Homepage),
			License:         flattenToString(rec.License),
			Platforms:       flattenToSlice(rec.Platforms),
		})
	}
	return pkgs, nil
}

func flattenToString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]interface{}:
		if full, ok := t["fullName"].(string); ok {
			return full
		}
		if spdx, ok := t["spdxId"].(string); ok {
			return spdx
		}
	case []interface{}:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			if s := flattenToString(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

func flattenToSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func extractJSONObject(raw []byte) []byte {
	start := bytes.IndexByte(raw, '{')
	end := bytes.LastIndexByte(raw, '}')
	if start == -1 || end == -1 || end < start {
		return raw
	}
	return raw[start : end+1]
}
