package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"nixpeek/internal/backend"
	"nixpeek/internal/cache"
	"nixpeek/internal/installed"
	"nixpeek/internal/search"
	"nixpeek/internal/ui"
)

func main() {
	query := flag.String("query", "", "prefill search query")
	backendName := flag.String("backend", "local", "search backend (local)")
	flag.Parse()

	if *backendName != "local" {
		fmt.Fprintf(os.Stderr, "unsupported backend %q, only local is available\n", *backendName)
		os.Exit(2)
	}

	b := backend.NewLocalBackend()
	c := cache.NewSession()
	checker := installed.NewNixProfileChecker()
	svc := search.NewService(b, c, checker)
	app := ui.New(svc, *query)

	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "app error: %v\n", err)
		os.Exit(1)
	}
}
