package main

import (
	"context"
	"fmt"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"git.sr.ht/~emersion/pyonji/mailconfig"
)

type smtpConfig struct {
	mailconfig.SMTP
	Username string
	Password string
}

func (cfg *smtpConfig) check(ctx context.Context) error {
	addr := fmt.Sprintf("%v:%v", cfg.Hostname, cfg.Port)

	var (
		c   *smtp.Client
		err error
	)
	if cfg.STARTTLS {
		c, err = smtp.Dial(addr)
		if err == nil {
			err = c.StartTLS(nil)
		}
	} else {
		c, err = smtp.DialTLS(addr, nil)
	}
	if err != nil {
		return err
	}
	defer c.Close()

	return c.Auth(sasl.NewPlainClient("", cfg.Username, cfg.Password))
}
