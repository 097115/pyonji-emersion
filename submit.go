package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/emersion/go-message/mail"
)

var hashStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

type submission struct {
	to      *mail.Address
	commits []logCommit
}

type submissionComplete struct{}

type submitModel struct {
	ctx        context.Context
	smtpConfig *smtpConfig

	spinner spinner.Model
	to      textinput.Model

	baseBranch string
	submission submission
	loadingMsg string
	errMsg     string
	done       bool
}

func initialSubmitModel(ctx context.Context, smtpConfig *smtpConfig) submitModel {
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("247"))

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	to := textinput.New()
	to.Prompt = "To "
	to.PromptStyle = labelStyle.Copy()
	to.TextStyle = textStyle.Copy()

	return submitModel{
		ctx:        ctx,
		smtpConfig: smtpConfig,
		spinner:    s,
		to:         to,
		baseBranch: "master", // TODO: find default branch
		loadingMsg: "Loading submission...",
	}
}

func (m submitModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, textinput.Blink, func() tea.Msg {
		return loadSubmission(m.ctx, m.baseBranch)
	})
}

func (m submitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			m.loadingMsg = "Submitting patches..."
			return m, func() tea.Msg {
				return submitPatches(m.ctx, m.baseBranch, m.smtpConfig, &m.submission)
			}
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		}
	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case submission:
		m.submission = msg
		m.loadingMsg = ""
		m.to.SetValue(msg.to.Address)
	case submissionComplete:
		m.loadingMsg = ""
		m.done = true
		return m, tea.Quit
	case error:
		m.loadingMsg = ""
		m.errMsg = msg.Error()
		return m, tea.Quit
	}
	return m, nil
}

func (m submitModel) View() string {
	if m.loadingMsg != "" {
		return m.spinner.View() + m.loadingMsg + "\n"
	}

	var sb strings.Builder

	sb.WriteString(m.to.View() + "\n")
	sb.WriteString("\n")

	sb.WriteString(buttonActiveStyle.Render("Submit") + "\n")
	sb.WriteString("\n")

	if len(m.submission.commits) > 0 {
		sb.WriteString(pluralize("commit", len(m.submission.commits)) + "\n")
		for _, commit := range m.submission.commits {
			sb.WriteString(hashStyle.Render(commit.Hash) + " " + commit.Subject + "\n")
		}
	} else if m.errMsg == "" {
		sb.WriteString(warningStyle.Render("⚠ There are no changes\n"))
	}

	if m.errMsg != "" {
		sb.WriteString(errorStyle.Render("× " + m.errMsg + "\n"))
	}
	if m.done {
		sb.WriteString(successStyle.Render("✓ Patches sent\n"))
	}

	return lipgloss.NewStyle().Padding(1).Render(sb.String())
}

func loadSubmission(ctx context.Context, baseBranch string) tea.Msg {
	to, err := loadGitSendEmailTo()
	if err != nil {
		return err
	} else if to == nil {
		// TODO: ask for email addr & save it
		return fmt.Errorf("missing sendemail.to")
	}

	commits, err := loadGitLog(ctx, baseBranch+"..")
	if err != nil {
		return err
	}

	return submission{to: to, commits: commits}
}

func submitPatches(ctx context.Context, baseBranch string, cfg *smtpConfig, s *submission) tea.Msg {
	from, err := getGitConfig("user.email")
	if err != nil {
		return err
	}

	patches, err := formatGitPatches(ctx, baseBranch)
	if err != nil {
		return err
	}

	c, err := cfg.dialAndAuth(ctx)
	if err != nil {
		return err
	}
	defer c.Close()

	for _, patch := range patches {
		to := []string{s.to.Address}
		err := c.SendMail(from, to, bytes.NewReader(patch))
		if err != nil {
			return err
		}
	}

	return submissionComplete{}
}

func pluralize(name string, n int) string {
	s := fmt.Sprintf("%v %v", n, name)
	if n > 1 {
		s += "s"
	}
	return s
}
