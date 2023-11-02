package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/emersion/go-mbox"
	"github.com/emersion/go-message/mail"
)

func getGitConfig(key string) (string, error) {
	cmd := exec.Command("git", "config", "--default=", key)
	b, err := cmd.Output()
	return strings.TrimSpace(string(b)), err
}

func setGitConfig(key, value string) error {
	cmd := exec.Command("git", "config", "--global", key, value)
	return cmd.Run()
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
		if err := setGitConfig("sendemail."+kv[0], kv[1]); err != nil {
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

func formatGitPatches(ctx context.Context, baseBranch string) ([][]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "format-patch", "--stdout", "--thread", "--base="+baseBranch, baseBranch+"..")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var patches [][]byte
	mr := mbox.NewReader(stdout)
	for {
		r, err := mr.NextMessage()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		b, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}

		patches = append(patches, b)
	}

	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return patches, nil
}
