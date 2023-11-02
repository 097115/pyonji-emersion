package mailconfig

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("mailautoconfig: no mail server found")

type SMTP struct {
	Hostname string
	Port     int
	STARTTLS bool
}

type provider interface {
	DiscoverSMTP(ctx context.Context, address string) (*SMTP, error)
}

func DiscoverSMTP(ctx context.Context, address string) (*SMTP, error) {
	var dns dnsProvider
	return dns.DiscoverSMTP(ctx, address)
}
