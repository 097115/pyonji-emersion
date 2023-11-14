package main

import (
	"context"
	"io"
	"net"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"git.sr.ht/~emersion/pyonji/mailconfig"
)

type smtpConfig struct {
	mailconfig.SMTP
	InsecureNoTLS bool
	Password      string
}

func (cfg *smtpConfig) check(ctx context.Context) error {
	c, err := cfg.dialAndAuth(ctx)
	if c != nil {
		c.Close()
	}
	return err
}

func (cfg *smtpConfig) dialAndAuth(ctx context.Context) (*smtpClient, error) {
	addr := net.JoinHostPort(cfg.Hostname, cfg.Port)

	var (
		c   *smtp.Client
		err error
	)
	if cfg.STARTTLS || cfg.InsecureNoTLS {
		c, err = smtp.Dial(addr)
		if err == nil && !cfg.InsecureNoTLS {
			err = c.StartTLS(nil)
		}
	} else {
		c, err = smtp.DialTLS(addr, nil)
	}
	if err != nil {
		return nil, err
	}

	if err := c.Auth(sasl.NewPlainClient("", cfg.Username, cfg.Password)); err != nil {
		c.Close()
		return nil, err
	}

	return &smtpClient{c}, err
}

type smtpClient struct {
	*smtp.Client
}

var _ mailSender = smtpClient{}

func (c smtpClient) SendMail(ctx context.Context, from string, to []string, data io.Reader) error {
	// TODO: pass the context somehow
	return c.Client.SendMail(from, to, data)
}
