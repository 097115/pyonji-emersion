package main

import (
	"fmt"
	"os/exec"
	"strings"

	"git.sr.ht/~emersion/pyonji/mailconfig"
)

func getGitConfig(key string) (string, error) {
	cmd := exec.Command("git", "config", "--global", "--default=", key)
	b, err := cmd.Output()
	return strings.TrimSpace(string(b)), err
}

func setGitConfig(key, value string) error {
	cmd := exec.Command("git", "config", "--global", key, value)
	return cmd.Run()
}

func saveGitSendEmailConfig(cfg *mailconfig.SMTP, username, password string) error {
	enc := "ssl"
	if cfg.STARTTLS {
		enc = "tls"
	}

	kvs := [][2]string{
		{"smtpServer", cfg.Hostname},
		{"smtpServerPort", fmt.Sprintf("%v", cfg.Port)},
		{"smtpEncryption", enc},
		{"smtpUser", username},
		{"smtpPass", password}, // TODO: do not store as plaintext
	}
	for _, kv := range kvs {
		if err := setGitConfig("sendemail."+kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}
