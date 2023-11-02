package main

import (
	"context"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := tea.NewProgram(initialInitModel(ctx))
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

var (
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
)
