package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
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
	if cfg.STARTTLS {
		enc = "tls"
	}

	kvs := [][2]string{
		{"smtpServer", cfg.Hostname},
		{"smtpServerPort", cfg.Port},
		{"smtpEncryption", enc},
		{"smtpUser", cfg.Username},
		{"smtpPass", cfg.Password}, // TODO: do not store as plaintext
	}
	for _, kv := range kvs {
		if err := setGitGlobalConfig("sendemail."+kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}

func loadGitSendEmailConfig() (*smtpConfig, error) {
	var server, port, enc, user, pass string
	entries := map[string]*string{
		"smtpServer":     &server,
		"smtpServerPort": &port,
		"smtpEncryption": &enc,
		"smtpUser":       &user,
		"smtpPass":       &pass,
	}
	for k, ptr := range entries {
		v, err := getGitConfig("sendemail." + k)
		if err != nil {
			return nil, err
		}
		*ptr = v
	}

	if server == "" {
		return nil, nil
	}

	var cfg smtpConfig
	cfg.Hostname = server
	switch enc {
	case "", "ssl":
		// direct TLS
	case "tls":
		cfg.STARTTLS = true
	case "none":
		cfg.InsecureNoTLS = true
	default:
		return nil, fmt.Errorf("invalid sendemail.smtpEncryption %q", enc)
	}
	switch port {
	case "":
		if cfg.STARTTLS {
			cfg.Port = "submission"
		} else {
			cfg.Port = "submissions"
		}
	default:
		cfg.Port = port
	}
	cfg.Username = user
	cfg.Password = pass
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
	cmd := exec.CommandContext(ctx, "git", "log", "--pretty=format:%h %s", revRange)
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

func formatGitPatches(ctx context.Context, baseBranch string) ([]patch, error) {
	cmd := exec.CommandContext(ctx, "git", "format-patch", "--stdout", "--base="+baseBranch, baseBranch+"..")
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
