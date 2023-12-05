package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/emersion/go-message/mail"
	"github.com/muesli/reflow/truncate"
	"github.com/pborman/getopt/v2"
)

var hashStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

type submissionLog struct {
	commits              []logCommit
	sameAsPrevSubmission bool
}

type submissionConfig struct {
	baseBranch    string
	to            string
	rerollCount   string
	subjectPrefix string
}

type coverLetterUpdated struct {
	coverLetter string
}

type submissionProgress struct {
	mailsSent  int
	mailsTotal int
	done       bool
}

type submitState int

const (
	submitStateTo submitState = iota
	submitStateVersion
	submitStateCoverLetter
	submitStateConfirm
)

var (
	labelStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	activeLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))

	activeTextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	textStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("247"))
)

type submitModel struct {
	ctx       context.Context
	gitConfig *gitSendEmailConfig
	progress  chan submissionProgress

	spinner spinner.Model
	to      textinput.Model
	version textinput.Model

	state                submitState
	headBranch           string
	baseBranch           string
	coverLetter          string
	subjectPrefix        string
	commits              []logCommit
	sameAsPrevSubmission bool
	loadingMsg           string
	errMsg               string
	done                 bool
}

func initialSubmitModel(ctx context.Context, gitConfig *gitSendEmailConfig) submitModel {
	headBranch := findGitCurrentBranch()

	cfg, err := loadSubmissionConfig(headBranch)
	if err != nil {
		log.Fatal(err)
	}

	getopt.FlagLong(&cfg.baseBranch, "base", 0, "base branch")
	getopt.FlagLong(&cfg.to, "to", 0, "recipient")
	getopt.FlagLong(&cfg.rerollCount, "reroll-count", 'v', "iteration number")
	getopt.Parse()

	if err := loadB4ProjectDefaults(cfg); err != nil {
		log.Fatal(err)
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

	coverLetter, err := loadGitBranchDescription(headBranch)
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

	toInput := textinput.New()
	toInput.Prompt = "To "
	toInput.PromptStyle = labelStyle.Copy()
	toInput.TextStyle = textStyle.Copy()
	toInput.SetValue(cfg.to)

	versionInput := textinput.New()
	versionInput.Prompt = "Version "
	versionInput.Placeholder = "1"
	versionInput.PromptStyle = labelStyle.Copy()
	versionInput.TextStyle = textStyle.Copy()
	versionInput.SetValue(rerollCount)

	return submitModel{
		ctx:           ctx,
		gitConfig:     gitConfig,
		progress:      make(chan submissionProgress, 1),
		spinner:       s,
		to:            toInput,
		version:       versionInput,
		headBranch:    headBranch,
		baseBranch:    cfg.baseBranch,
		coverLetter:   coverLetter,
		subjectPrefix: cfg.subjectPrefix,
		loadingMsg:    "Loading submission...",
	}.setState(state)
}

func (m submitModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, textinput.Blink, func() tea.Msg {
		return <-m.progress
	}, func() tea.Msg {
		return loadSubmissionLog(m.ctx, m.baseBranch, m.headBranch)
	})
}

func (m submitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type != tea.KeyCtrlC && m.loadingMsg != "" {
			break
		}
		switch msg.Type {
		case tea.KeyEnter:
			switch m.state {
			case submitStateTo, submitStateVersion:
				m = m.setState(submitStateConfirm)
			case submitStateCoverLetter:
				if m.headBranch == "" {
					break
				}
				cmd := exec.Command("git", "branch", "--edit-description", m.headBranch)
				return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
					if err != nil {
						return err
					}
					coverLetter, err := loadGitBranchDescription(m.headBranch)
					if err != nil {
						return err
					} else {
						return coverLetterUpdated{coverLetter}
					}
				})
			case submitStateConfirm:
				if !m.canSubmit() {
					break
				}
				m.loadingMsg = "Submitting patches..."
				return m, func() tea.Msg {
					cfg := submissionConfig{
						to:            m.to.Value(),
						baseBranch:    m.baseBranch,
						rerollCount:   m.version.Value(),
						subjectPrefix: m.subjectPrefix,
					}
					return submitPatches(m.ctx, m.headBranch, &cfg, m.gitConfig, m.coverLetter != "", m.progress)
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
	case submissionLog:
		m.loadingMsg = ""
		m.commits = msg.commits
		m.sameAsPrevSubmission = msg.sameAsPrevSubmission
	case coverLetterUpdated:
		m.coverLetter = msg.coverLetter
	case submissionProgress:
		if msg.done {
			m.loadingMsg = ""
			m.done = true
			return m, tea.Quit
		} else {
			if msg.mailsSent == 0 && msg.mailsTotal == 1 {
				m.loadingMsg = fmt.Sprintf("Sending mail...")
			} else if msg.mailsSent < msg.mailsTotal {
				m.loadingMsg = fmt.Sprintf("Sending mail %v/%v...", msg.mailsSent+1, msg.mailsTotal)
			} else {
				m.loadingMsg = "Finishing up submission..."
			}
			return m, func() tea.Msg {
				return <-m.progress
			}
		}
	case error:
		m.loadingMsg = ""
		m.errMsg = msg.Error()
		return m, tea.Quit
	}

	var toCmd, versionCmd tea.Cmd
	m.to, toCmd = m.to.Update(msg)
	m.version, versionCmd = m.version.Update(msg)
	return m, tea.Batch(toCmd, versionCmd)
}

func (m submitModel) View() string {
	if m.loadingMsg != "" && len(m.commits) == 0 {
		return m.spinner.View() + m.loadingMsg + "\n"
	}

	var sb strings.Builder

	field := formField{Label: "Base", Text: m.baseBranch}
	sb.WriteString(field.View() + "\n")

	sb.WriteString(m.to.View() + "\n")
	sb.WriteString(m.version.View() + "\n")

	var coverLetter string
	if m.coverLetter != "" {
		coverLetter, _, _ = strings.Cut(m.coverLetter, "\n")
		coverLetter = truncate.StringWithTail(coverLetter, 72, "...")
	} else {
		coverLetter = "none"
	}
	field = formField{Label: "Cover letter", Text: coverLetter, Active: m.state == submitStateCoverLetter}
	sb.WriteString(field.View() + "\n")

	sb.WriteString("\n")

	if m.sameAsPrevSubmission {
		sb.WriteString(warningStyle.Render("⚠ This version has already been submitted") + "\n\n")
	}

	if m.loadingMsg != "" {
		sb.WriteString(m.spinner.View() + m.loadingMsg + "\n")
	} else if m.done {
		sb.WriteString(successStyle.Render("✓ Patches sent\n"))
	} else {
		btn := button{
			Label:    "Submit",
			Active:   m.state == submitStateConfirm,
			Disabled: !m.canSubmit(),
		}
		sb.WriteString(btn.View() + "\n")
	}

	sb.WriteString("\n")

	if len(m.commits) > 0 {
		sb.WriteString(pluralize("commit", len(m.commits)) + "\n")

		n := len(m.commits)
		if n > 10 {
			n = 10
		}
		for _, commit := range m.commits[:n] {
			hash := commit.Hash[:12]
			sb.WriteString(hashStyle.Render(hash) + " " + commit.Subject + "\n")
		}
	} else if m.errMsg == "" {
		sb.WriteString(warningStyle.Render("⚠ There are no changes\n"))
	}

	if m.errMsg != "" {
		sb.WriteString(errorStyle.Render("× " + m.errMsg + "\n"))
	}

	return lipgloss.NewStyle().Padding(1).Render(sb.String())
}

func (m submitModel) setState(state submitState) submitModel {
	m.to.Blur()
	m.to.PromptStyle = labelStyle
	m.to.TextStyle = textStyle

	m.version.Blur()
	m.version.PromptStyle = labelStyle
	m.version.TextStyle = textStyle

	m.state = state
	switch state {
	case submitStateTo:
		m.to.Focus()
		m.to.PromptStyle = activeLabelStyle
		m.to.TextStyle = activeTextStyle
	case submitStateVersion:
		m.version.Focus()
		m.version.PromptStyle = activeLabelStyle
		m.version.TextStyle = activeTextStyle
	}
	return m
}

func (m submitModel) canSubmit() bool {
	return len(m.commits) > 0 && checkAddress(m.to.Value()) && checkVersion(m.version.Value())
}

func loadSubmissionLog(ctx context.Context, baseBranch, headBranch string) tea.Msg {
	commits, err := loadGitLog(ctx, baseBranch+".."+headBranch)
	if err != nil {
		return err
	}

	sameAsPrevSubmission := false
	if len(commits) > 0 {
		last := getLastSentHash(headBranch)
		sameAsPrevSubmission = last != "" && last == commits[0].Hash
	}

	return submissionLog{commits: commits, sameAsPrevSubmission: sameAsPrevSubmission}
}

type mailSender interface {
	Close() error
	SendMail(ctx context.Context, from string, to []string, data io.Reader) error
}

func submitPatches(ctx context.Context, headBranch string, submission *submissionConfig, git *gitSendEmailConfig, coverLetter bool, ch chan<- submissionProgress) tea.Msg {
	if err := saveSubmissionConfig(headBranch, submission); err != nil {
		return err
	}
	if err := autosaveSendEmailTo(submission.to); err != nil {
		return err
	}

	from, err := loadGitSendEmailFrom()
	if err != nil {
		return err
	}
	_, fromHostname, _ := strings.Cut(from.Address, "@")

	envelopeSender, err := getGitConfig("sendemail.envelopeSender")
	if err != nil {
		return err
	} else if envelopeSender == "" {
		envelopeSender = from.Address
	}

	patches, err := formatGitPatches(ctx, submission.baseBranch, &gitFormatPatchOptions{
		RerollCount:   submission.rerollCount,
		CoverLetter:   coverLetter,
		SubjectPrefix: submission.subjectPrefix,
	})
	if err != nil {
		return err
	}

	var firstMsgID string
	for i := range patches {
		patch := &patches[i]
		patch.header.SetAddressList("From", []*mail.Address{from})
		patch.header.SetAddressList("To", []*mail.Address{{Address: submission.to}})
		if err := patch.header.GenerateMessageIDWithHostname(fromHostname); err != nil {
			return err
		}
		if firstMsgID == "" {
			firstMsgID, _ = patch.header.MessageID()
		} else {
			patch.header.SetMsgIDList("In-Reply-To", []string{firstMsgID})
		}
	}

	var sender mailSender
	if git.SMTP != nil {
		sender, err = git.SMTP.dialAndAuth(ctx)
	} else {
		sender = &sendmailCmd{git.Sendmail}
	}
	if err != nil {
		return err
	}
	defer sender.Close()

	progress := submissionProgress{mailsTotal: len(patches)}
	ch <- progress

	for _, patch := range patches {
		r := bytes.NewReader(patch.Bytes())
		err := sender.SendMail(ctx, envelopeSender, []string{submission.to}, r)
		if err != nil {
			return err
		}

		progress.mailsSent++
		ch <- progress
	}

	if err := saveLastSentHash(headBranch); err != nil {
		return err
	}

	progress.done = true
	return progress
}

func loadGitSendEmailFrom() (*mail.Address, error) {
	raw, err := getGitConfig("sendemail.from")
	if err != nil {
		return nil, err
	} else if raw != "" {
		addr, err := mail.ParseAddress(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid sendemail.from: %v", err)
		}
		return addr, nil
	}

	email, err := getGitConfig("user.email")
	if err != nil {
		return nil, err
	}
	name, err := getGitConfig("user.name")
	if err != nil {
		return nil, err
	}
	if email == "" {
		return nil, fmt.Errorf("user.email not set")
	}
	return &mail.Address{Name: name, Address: email}, nil
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

func loadB4ProjectDefaults(cfg *submissionConfig) error {
	toplevelDir, err := getGitToplevelDir()
	if err != nil {
		return err
	}

	b4ConfigPath := filepath.Join(toplevelDir, ".b4-config")
	if _, err := os.Stat(b4ConfigPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	var to, prefixes string
	values := map[string]*string{
		"send-series-to": &to,
		"send-prefixes":  &prefixes,
	}
	for k, ptr := range values {
		k = "b4." + k
		cmd := exec.Command("git", "config", "--file="+b4ConfigPath, "--default=", k)
		b, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to get b4 config %q: %v", k, err)
		}
		*ptr = strings.TrimSpace(string(b))
	}

	if cfg.to == "" && to != "" {
		addr, err := mail.ParseAddress(to)
		if err != nil {
			return fmt.Errorf("invalid b4.send-series-to: %v", err)
		}
		cfg.to = addr.Address
	}
	if cfg.subjectPrefix == "" && prefixes != "" && validateSubjectPrefix(prefixes) {
		cfg.subjectPrefix = "PATCH " + prefixes
	}

	return nil
}

func validateSubjectPrefix(s string) bool {
	if len(s) > 1024 {
		return false
	}
	for _, ch := range s {
		if unicode.IsControl(ch) || ch == '\n' || ch == '\r' {
			return false
		}
	}
	return true
}

func autosaveSendEmailTo(to string) error {
	cur, err := loadGitSendEmailTo()
	if err != nil {
		return err
	} else if cur != nil {
		return nil // Don't overwrite any previous setting
	}

	toplevelDir, err := getGitToplevelDir()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(toplevelDir, "MAINTAINERS")); err == nil {
		return nil // Probably multiple mailing lists involved
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check for MAINTAINERS: %v", err)
	}

	return setGitConfig("sendemail.to", to)
}

func loadGitBranchDescription(branch string) (string, error) {
	if branch == "" {
		return "", nil
	}
	return getGitConfig("branch." + branch + ".description")
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

func checkVersion(ver string) bool {
	for _, ch := range ver {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

type formField struct {
	Label, Text string
	Active      bool
}

func (f *formField) View() string {
	labelStyle, textStyle := labelStyle, textStyle
	if f.Active {
		labelStyle, textStyle = activeLabelStyle, activeTextStyle
	}
	return fmt.Sprintf("%v %v", labelStyle.Render(f.Label), textStyle.Render(f.Text))
}

type button struct {
	Label            string
	Active, Disabled bool
}

func (btn *button) View() string {
	style := buttonStyle
	if btn.Active {
		if btn.Disabled {
			style = buttonInactiveStyle
		} else {
			style = buttonActiveStyle
		}
	}
	return style.Render(btn.Label)
}
