package main

import (
	"fmt"
	"os/exec"
	"strings"
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
