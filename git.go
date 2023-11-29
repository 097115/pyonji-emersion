package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/emersion/go-mbox"
	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
	"github.com/emersion/go-message/textproto"
)

func getGitConfig(key string) (string, error) {
	cmd := exec.Command("git", "config", "--default=", key)
	b, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Git config %q: %v", key, err)
	}
	return strings.TrimSpace(string(b)), nil
}

func getAllGitConfig(key string) ([]string, error) {
	// --get-all does not support --default
	first, err := getGitConfig(key)
	if err != nil {
		return nil, err
	} else if first == "" {
		return nil, nil
	}

	cmd := exec.Command("git", "config", "--get-all", key)
	b, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get Git config %q: %v", key, err)
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return nil, nil
	}
	return strings.Split(s, "\n"), nil
}

func setGitConfig(key, value string) error {
	cmd := exec.Command("git", "config", key, value)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set Git config %q: %v", key, err)
	}
	return nil
}

func setGitGlobalConfig(key, value string) error {
	cmd := exec.Command("git", "config", "--global", key, value)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set Git global config %q: %v", key, err)
	}
	return nil
}

func saveGitSendEmailConfig(cfg *smtpConfig) error {
	enc := "ssl"
	if cfg.StartTLS {
		enc = "tls"
	}

	kvs := []struct{ k, v string }{
		{"smtpServer", cfg.Hostname},
		{"smtpServerPort", cfg.Port},
		{"smtpEncryption", enc},
		{"smtpUser", cfg.Username},
		{"smtpPass", cfg.Password}, // TODO: do not store as plaintext
	}
	for _, kv := range kvs {
		if err := setGitGlobalConfig("sendemail."+kv.k, kv.v); err != nil {
			return err
		}
	}
	return nil
}

type gitSendEmailConfig struct {
	SMTP     *smtpConfig
	Sendmail *sendmailConfig
}

func loadGitSendEmailConfig() (*gitSendEmailConfig, error) {
	var server, port, enc, user, pass, sendmailCmd string
	entries := map[string]*string{
		"smtpServer":     &server,
		"smtpServerPort": &port,
		"smtpEncryption": &enc,
		"smtpUser":       &user,
		"smtpPass":       &pass,
		"sendmailCmd":    &sendmailCmd,
	}
	for k, ptr := range entries {
		v, err := getGitConfig("sendemail." + k)
		if err != nil {
			return nil, err
		}
		*ptr = v
	}

	// git-send-email allows specifying a sendmail command in smtpServer
	if filepath.IsAbs(server) {
		server, sendmailCmd = "", server
	}

	if server == "" && sendmailCmd == "" {
		return nil, nil
	}

	var cfg gitSendEmailConfig
	if server != "" {
		// git-send-email supports supplying the port in smtpServer
		if serverHost, serverPort, err := net.SplitHostPort(server); err == nil {
			if port != "" && port != serverPort {
				return nil, fmt.Errorf("conflicting Git sendemail options: smtpServer = %q, smtpServerPort = %q", server, port)
			}
			server, port = serverHost, serverPort
		}

		cfg.SMTP = new(smtpConfig)
		cfg.SMTP.Hostname = server
		switch enc {
		case "", "ssl":
			// direct TLS
		case "tls":
			cfg.SMTP.StartTLS = true
		case "none":
			cfg.SMTP.InsecureNoTLS = true
		default:
			return nil, fmt.Errorf("invalid sendemail.smtpEncryption %q", enc)
		}
		switch port {
		case "":
			if cfg.SMTP.StartTLS {
				cfg.SMTP.Port = "submission"
			} else {
				cfg.SMTP.Port = "submissions"
			}
		default:
			cfg.SMTP.Port = port
		}
		cfg.SMTP.Username = user
		cfg.SMTP.Password = pass
	} else {
		cfg.Sendmail = new(sendmailConfig)
		cfg.Sendmail.Cmd = sendmailCmd

		opts, err := getAllGitConfig("sendemail.smtpServerOption")
		if err != nil {
			return nil, err
		}
		cfg.Sendmail.Options = opts
	}
	return &cfg, nil
}

func loadGitSendEmailTo() (*mail.Address, error) {
	v, err := getGitConfig("sendemail.to")
	if err != nil {
		return nil, err
	} else if v == "" {
		return nil, nil
	}
	addr, err := mail.ParseAddress(v)
	if err != nil {
		return nil, fmt.Errorf("invalid sendemail.to: %v", err)
	}
	return addr, nil
}

type logCommit struct {
	Hash    string
	Subject string
}

func loadGitLog(ctx context.Context, revRange string) ([]logCommit, error) {
	cmd := exec.CommandContext(ctx, "git", "log", "--pretty=format:%H %s", revRange)
	b, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to load git log: %v", err)
	}

	var log []logCommit
	for _, l := range strings.Split(string(b), "\n") {
		if l == "" {
			continue
		}
		hash, subject, _ := strings.Cut(l, " ")
		log = append(log, logCommit{hash, subject})
	}
	return log, nil
}

type patch struct {
	header mail.Header
	body   []byte
}

func (p *patch) Bytes() []byte {
	var buf bytes.Buffer
	textproto.WriteHeader(&buf, p.header.Header.Header)
	buf.Write(p.body)
	return buf.Bytes()
}

type gitFormatPatchOptions struct {
	RerollCount   string
	CoverLetter   bool
	SubjectPrefix string
}

func formatGitPatches(ctx context.Context, baseBranch string, options *gitFormatPatchOptions) ([]patch, error) {
	baseCommit, err := getGitMergeBase(baseBranch, "HEAD")
	if err != nil {
		return nil, err
	}

	args := []string{"format-patch", "--stdout", "--encode-email-headers"}
	if options.RerollCount != "" {
		args = append(args, "--reroll-count="+options.RerollCount)
	}
	if options.CoverLetter {
		args = append(args, "--cover-letter", "--cover-from-description=subject")
	}
	if options.SubjectPrefix != "" {
		args = append(args, "--subject-prefix="+options.SubjectPrefix)
	}
	args = append(args, "--base="+baseCommit, baseBranch+"..")

	cmd := exec.CommandContext(ctx, "git", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to format Git patches: %v", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to format Git patches: %v", err)
	}

	var patches []patch
	mr := mbox.NewReader(stdout)
	for {
		r, err := mr.NextMessage()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("failed to parse Git patches mbox: %v", err)
		}

		br := bufio.NewReader(r)
		header, err := textproto.ReadHeader(br)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Git patch header: %v", err)
		}

		b, err := io.ReadAll(br)
		if err != nil {
			return nil, fmt.Errorf("failed to read Git patch body: %v", err)
		}

		patches = append(patches, patch{
			header: mail.Header{message.Header{header}},
			body:   b,
		})
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("failed to format Git patches: %v", err)
	}

	return patches, nil
}

func getGitMergeBase(a, b string) (string, error) {
	cmd := exec.Command("git", "merge-base", a, b)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to find merge base of %q and %q: %v", a, b, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func getGitCurrentCommit() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to fetch current commit: %v", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func findGitCurrentBranch() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	b, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func findGitDefaultBranch() string {
	// TODO: find a better way to get the default branch

	for _, name := range []string{"main", "master"} {
		if checkGitBranch(name) {
			return name
		}
	}

	return ""
}

func checkGitBranch(name string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", name)
	return cmd.Run() == nil
}

func getGitToplevelDir() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Git toplevel directory: %v", err)
	}
	return strings.TrimSpace(string(out)), nil
}
