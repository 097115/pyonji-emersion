package mailconfig

import (
	"context"
	"crypto/tls"
	"net"
	"strings"
	"time"

	"github.com/emersion/go-smtp"
)

type subdomainGuessProvider struct {
	subdomain string
	startTLS  bool
}

var _ provider = subdomainGuessProvider{}

func (provider subdomainGuessProvider) DiscoverSMTP(ctx context.Context, address string) (*SMTP, error) {
	_, domain, _ := strings.Cut(address, "@")
	host := provider.subdomain + "." + domain

	port := "465"
	if provider.startTLS {
		port = "587"
	}

	network := "tcp"
	addr := host + ":" + port

	var (
		conn net.Conn
		err  error
	)
	if provider.startTLS {
		var dialer net.Dialer
		conn, err = dialer.DialContext(ctx, network, addr)
	} else {
		var dialer tls.Dialer
		conn, err = dialer.DialContext(ctx, network, addr)
	}
	if err != nil {
		return nil, ErrNotFound
	}
	defer conn.Close()

	// TODO: pass context somehow
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return nil, err
	}
	c.CommandTimeout = 5 * time.Second

	if provider.startTLS {
		if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return nil, err
		}
	}

	if ok, _ := c.Extension("AUTH"); !ok {
		return nil, ErrNotFound
	}

	return &SMTP{Hostname: host, Port: port, STARTTLS: provider.startTLS}, nil
}
