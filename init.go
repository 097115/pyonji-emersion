package main

import (
	"context"
	"log"
	"net/mail"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"git.sr.ht/~emersion/pyonji/mailconfig"
)

type passwordCheckResult struct {
	err error
}

type initModel struct {
	ctx context.Context

	emailInput    textinput.Model
	passwordInput textinput.Model
	spinner       spinner.Model

	smtpConfig       smtpConfig
	discovering      bool
	showPassword     bool
	checkingPassword bool
	done             bool
	errMsg           string
}

func initialInitModel(ctx context.Context) initModel {
	emailInput := textinput.New()
	emailInput.Prompt = "E-mail address: "
	emailInput.Placeholder = "me@example.org"
	emailInput.Focus()

	defaultEmail, err := getGitConfig("user.email")
	if err != nil {
		log.Fatal(err)
	}
	emailInput.SetValue(defaultEmail)

	passwordInput := textinput.New()
	passwordInput.Prompt = "Password: "
	passwordInput.EchoMode = textinput.EchoPassword
	passwordInput.EchoCharacter = '•'

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return initModel{
		ctx: ctx,

		emailInput:    emailInput,
		passwordInput: passwordInput,
		spinner:       s,
	}
}

func (m initModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spinner.Tick)
}

func (m initModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			if m.emailInput.Focused() {
				return m.submitEmail()
			} else if m.passwordInput.Focused() {
				return m.submitPassword()
			}
		case tea.KeyCtrlC, tea.KeyEsc:
			return m.quit()
		}
	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case *mailconfig.SMTP:
		m.discovering = false
		m.smtpConfig.SMTP = *msg
		m.showPassword = true
		m.passwordInput.Focus()
	case passwordCheckResult:
		m.checkingPassword = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.passwordInput.Focus()
		} else {
			if err := saveGitSendEmailConfig(&m.smtpConfig); err != nil {
				log.Fatal(err)
			}
			m.done = true
			return m.quit()
		}
	case error:
		m.errMsg = msg.Error()
		return m.quit()
	}

	inputs := []*textinput.Model{
		&m.emailInput,
		&m.passwordInput,
	}
	cmds := make([]tea.Cmd, len(inputs))
	for i, input := range inputs {
		*input, cmds[i] = input.Update(msg)
	}
	return m, tea.Batch(cmds...)
}

func (m initModel) View() string {
	var sb strings.Builder
	sb.WriteString(m.emailInput.View() + "\n")
	if m.discovering {
		sb.WriteString(m.spinner.View() + "Checking mail server...\n")
	}
	if m.showPassword {
		sb.WriteString(m.passwordInput.View() + "\n")
	}
	if m.checkingPassword {
		sb.WriteString(m.spinner.View() + "Checking password...\n")
	}
	if m.errMsg != "" {
		sb.WriteString(errorStyle.Render("× "+m.errMsg) + "\n")
	}
	if m.done {
		sb.WriteString(successStyle.Render("✓ Saved mail server settings\n"))
	}
	return sb.String()
}

func (m initModel) quit() (tea.Model, tea.Cmd) {
	m.discovering = false
	m.checkingPassword = false
	m.emailInput.Blur()
	m.passwordInput.Blur()
	return m, tea.Quit
}

func (m initModel) submitEmail() (tea.Model, tea.Cmd) {
	addr, err := mail.ParseAddress(m.emailInput.Value())
	if err != nil {
		m.errMsg = "Invalid e-mail address"
		return m, nil
	}

	m.emailInput.Blur()
	m.discovering = true
	m.smtpConfig.Username = addr.Address

	return m, func() tea.Msg {
		cfg, err := mailconfig.DiscoverSMTP(m.ctx, addr.Address)
		if err != nil {
			return err
		}
		return cfg
	}
}

func (m initModel) submitPassword() (tea.Model, tea.Cmd) {
	m.checkingPassword = true
	m.passwordInput.Blur()
	m.smtpConfig.Password = m.passwordInput.Value()

	return m, func() tea.Msg {
		err := m.smtpConfig.check(m.ctx)
		return passwordCheckResult{err}
	}
}
