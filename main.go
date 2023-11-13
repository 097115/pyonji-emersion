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

	gitConfig, err := loadGitSendEmailConfig()
	if err != nil {
		log.Fatal(err)
	}

	if gitConfig == nil {
		p := tea.NewProgram(initialInitModel(ctx))
		if m, err := p.Run(); err != nil {
			log.Fatal(err)
		} else {
			m := m.(initModel)
			if !m.done {
				return
			}
			gitConfig = &gitSendEmailConfig{SMTP: &m.smtpConfig}
		}
	}

	p := tea.NewProgram(initialSubmitModel(ctx, gitConfig))
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

var (
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("202"))

	buttonStyle         = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("241")).Padding(0, 2)
	buttonActiveStyle   = lipgloss.NewStyle().Background(lipgloss.Color("99")).Foreground(lipgloss.Color("#ECFD66")).Padding(0, 2)
	buttonInactiveStyle = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("247")).Padding(0, 2)
)
