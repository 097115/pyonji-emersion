package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/emersion/go-message/mail"
)

var hashStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

type submission struct {
	commits []logCommit
}

type submissionConfig struct {
	baseBranch  string
	to          string
	rerollCount string
}

type submissionComplete struct{}

type submitState int

const (
	submitStateTo submitState = iota
	submitStateConfirm
)

var (
	labelStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	activeLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))

	activeTextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	textStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("247"))
)

type submitModel struct {
	ctx        context.Context
	smtpConfig *smtpConfig

	spinner spinner.Model
	to      textinput.Model

	state       submitState
	headBranch  string
	baseBranch  string
	rerollCount string
	commits     []logCommit
	loadingMsg  string
	errMsg      string
	done        bool
}

func initialSubmitModel(ctx context.Context, smtpConfig *smtpConfig) submitModel {
	headBranch := findGitCurrentBranch()

	cfg := new(submissionConfig)
	if headBranch != "" {
		var err error
		if cfg, err = loadSubmissionConfig(headBranch); err != nil {
			log.Fatal(err)
		}
	}

	if cfg.baseBranch == "" {
		cfg.baseBranch = findGitDefaultBranch()
	}
	if cfg.baseBranch == "" {
		// TODO: allow user to pick
		log.Fatal("failed to find base branch")
	}

	if cfg.to == "" {
		to, err := loadGitSendEmailTo()
		if err != nil {
			log.Fatal(err)
		} else if to != nil {
			cfg.to = to.Address
		}
	}

	rerollCount, err := getNextRerollCount(headBranch, cfg.rerollCount)
	if err != nil {
		log.Fatal(err)
	}

	state := submitStateConfirm
	if cfg.to == "" {
		state = submitStateTo
	}

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	to := textinput.New()
	to.Prompt = "To "
	to.PromptStyle = labelStyle.Copy()
	to.TextStyle = textStyle.Copy()
	to.SetValue(cfg.to)

	return submitModel{
		ctx:         ctx,
		smtpConfig:  smtpConfig,
		spinner:     s,
		to:          to,
		headBranch:  headBranch,
		baseBranch:  cfg.baseBranch,
		rerollCount: rerollCount,
		loadingMsg:  "Loading submission...",
	}.setState(state)
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
			switch m.state {
			case submitStateTo:
				m = m.setState(submitStateConfirm)
			case submitStateConfirm:
				if !m.canSubmit() {
					break
				}
				m.loadingMsg = "Submitting patches..."
				return m, func() tea.Msg {
					cfg := submissionConfig{
						to:          m.to.Value(),
						baseBranch:  m.baseBranch,
						rerollCount: m.rerollCount,
					}
					return submitPatches(m.ctx, m.headBranch, &cfg, m.smtpConfig)
				}
			}
		case tea.KeyUp:
			if m.state > 0 {
				m = m.setState(m.state - 1)
			}
		case tea.KeyDown:
			if m.state < submitStateConfirm {
				m = m.setState(m.state + 1)
			}
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		}
	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case submission:
		m.loadingMsg = ""
		m.commits = msg.commits
	case submissionComplete:
		m.loadingMsg = ""
		m.done = true
		return m, tea.Quit
	case error:
		m.loadingMsg = ""
		m.errMsg = msg.Error()
		return m, tea.Quit
	}

	m.to, cmd = m.to.Update(msg)
	return m, cmd
}

func (m submitModel) View() string {
	if m.loadingMsg != "" {
		return m.spinner.View() + m.loadingMsg + "\n"
	}

	var sb strings.Builder

	fmt.Fprintf(&sb, "%v %v\n", labelStyle.Render("Base"), textStyle.Render(m.baseBranch))

	version := m.rerollCount
	if version == "" {
		version = "1"
	}
	fmt.Fprintf(&sb, "%v %v\n", labelStyle.Render("Version"), textStyle.Render(version))

	sb.WriteString(m.to.View() + "\n")
	sb.WriteString("\n")

	style := buttonStyle
	if m.state == submitStateConfirm {
		if m.canSubmit() {
			style = buttonActiveStyle
		} else {
			style = buttonInactiveStyle
		}
	}
	sb.WriteString(style.Render("Submit") + "\n")
	sb.WriteString("\n")

	if len(m.commits) > 0 {
		sb.WriteString(pluralize("commit", len(m.commits)) + "\n")

		n := len(m.commits)
		if n > 10 {
			n = 10
		}
		for _, commit := range m.commits[:n] {
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

func (m submitModel) setState(state submitState) submitModel {
	m.to.Blur()
	m.to.PromptStyle = labelStyle
	m.to.TextStyle = textStyle

	m.state = state
	switch state {
	case submitStateTo:
		m.to.Focus()
		m.to.PromptStyle = activeLabelStyle
		m.to.TextStyle = activeTextStyle
	}
	return m
}

func (m submitModel) canSubmit() bool {
	return len(m.commits) > 0 && checkAddress(m.to.Value())
}

func loadSubmission(ctx context.Context, baseBranch string) tea.Msg {
	commits, err := loadGitLog(ctx, baseBranch+"..")
	if err != nil {
		return err
	}

	return submission{commits: commits}
}

func submitPatches(ctx context.Context, headBranch string, submission *submissionConfig, smtp *smtpConfig) tea.Msg {
	if err := saveSubmissionConfig(headBranch, submission); err != nil {
		return err
	}

	from, err := getGitConfig("user.email")
	if err != nil {
		return err
	}
	_, fromHostname, _ := strings.Cut(from, "@")

	patches, err := formatGitPatches(ctx, submission.baseBranch, submission.rerollCount)
	if err != nil {
		return err
	}

	c, err := smtp.dialAndAuth(ctx)
	if err != nil {
		return err
	}
	defer c.Close()

	var firstMsgID string
	for _, patch := range patches {
		patch.header.SetAddressList("To", []*mail.Address{{Address: submission.to}})
		if err := patch.header.GenerateMessageIDWithHostname(fromHostname); err != nil {
			return err
		}
		if firstMsgID == "" {
			firstMsgID, _ = patch.header.MessageID()
		} else {
			patch.header.SetMsgIDList("In-Reply-To", []string{firstMsgID})
		}

		err := c.SendMail(from, []string{submission.to}, bytes.NewReader(patch.Bytes()))
		if err != nil {
			return err
		}
	}

	if err := saveLastSentHash(headBranch); err != nil {
		return err
	}

	return submissionComplete{}
}

func loadSubmissionConfig(branch string) (*submissionConfig, error) {
	if branch == "" {
		return &submissionConfig{}, nil
	}

	var cfg submissionConfig
	entries := map[string]*string{
		"pyonjiTo":          &cfg.to,
		"pyonjiBase":        &cfg.baseBranch,
		"pyonjiRerollCount": &cfg.rerollCount,
	}
	for k, ptr := range entries {
		v, err := getGitConfig("branch." + branch + "." + k)
		if err != nil {
			return nil, err
		}
		*ptr = v
	}

	return &cfg, nil
}

func saveSubmissionConfig(branch string, cfg *submissionConfig) error {
	if branch == "" {
		return nil
	}

	kvs := []struct{ k, v string }{
		{"pyonjiTo", cfg.to},
		{"pyonjiBase", cfg.baseBranch},
		{"pyonjiRerollCount", cfg.rerollCount},
	}
	for _, kv := range kvs {
		k := "branch." + branch + "." + kv.k
		if err := setGitConfig(k, kv.v); err != nil {
			return err
		}
	}

	return nil
}

func getLastSentHash(branch string) string {
	if branch == "" {
		return ""
	}
	commit, _ := getGitConfig("branch." + branch + ".pyonjiLastSentHash")
	return commit
}

func saveLastSentHash(branch string) error {
	if branch == "" {
		return nil
	}

	commit, err := getGitCurrentCommit()
	if err != nil {
		return err
	}

	k := "branch." + branch + ".pyonjiLastSentHash"
	return setGitConfig(k, commit)
}

func getNextRerollCount(branch, rerollCount string) (string, error) {
	last := getLastSentHash(branch)
	if last == "" {
		return rerollCount, nil
	}

	if cur, err := getGitCurrentCommit(); err != nil {
		return "", err
	} else if cur == last {
		return rerollCount, nil
	}

	v := 1
	if rerollCount != "" {
		var err error
		if v, err = strconv.Atoi(rerollCount); err != nil {
			return "", fmt.Errorf("failed to parse reroll count: %v", err)
		}
	}

	return strconv.Itoa(v + 1), nil
}

func pluralize(name string, n int) string {
	s := fmt.Sprintf("%v %v", n, name)
	if n > 1 {
		s += "s"
	}
	return s
}

func checkAddress(addr string) bool {
	_, err := mail.ParseAddress("<" + addr + ">")
	return err == nil
}
