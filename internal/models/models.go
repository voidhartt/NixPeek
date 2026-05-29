package models

type Package struct {
	AttrPath        string
	Name            string
	PName           string
	Version         string
	Description     string
	LongDescription string
	License         string
	Homepage        string
	Platforms       []string
	Installed       bool
}

type SearchFilters struct {
	Scope     Scope
	MatchMode MatchMode
	ExactAttr bool
}

type Scope string

const (
	ScopeNameOnly        Scope = "name"
	ScopeNameDescription Scope = "name_desc"
)

type MatchMode string

const (
	MatchContains MatchMode = "contains"
	MatchPrefix   MatchMode = "prefix"
)
