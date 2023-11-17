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

func (provider subdomainGuessProvider) DiscoverSMTP(ctx context.Context, _, domain string) (*SMTP, error) {
	host := provider.subdomain + "." + domain

	port := "465"
	if provider.startTLS {
		port = "587"
	}

	network := "tcp"
	addr := host + ":" + port

	// If the hostname is valid but isn't an SMTP server, this is likely to
	// timeout
	dialCtx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()

	var (
		conn net.Conn
		err  error
	)
	if provider.startTLS {
		var dialer net.Dialer
		conn, err = dialer.DialContext(dialCtx, network, addr)
	} else {
		var dialer tls.Dialer
		conn, err = dialer.DialContext(dialCtx, network, addr)
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

	return &SMTP{Hostname: host, Port: port, StartTLS: provider.startTLS}, nil
}

type dnsMXGuessProvider struct{}

var _ provider = dnsMXGuessProvider{}

func (dnsMXGuessProvider) DiscoverSMTP(ctx context.Context, addr, domain string) (*SMTP, error) {
	var resolver net.Resolver
	records, err := resolver.LookupMX(ctx, domain)
	if err != nil {
		return nil, err
	} else if len(records) == 0 {
		return nil, ErrNotFound
	}

	mxHost := strings.TrimSuffix(records[0].Host, ".")
	if mxHost == "" {
		return nil, ErrNotFound
	}

	mxDomain, ok := stripSubdomain(mxHost)
	if !ok || mxDomain == domain {
		return nil, ErrNotFound
	}

	return discoverSMTP(ctx, addr, mxDomain, false)
}

func stripSubdomain(name string) (string, bool) {
	// TODO: use something like publicsuffix.org
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return "", false
	}
	i = strings.LastIndexByte(name[:i], '.')
	if i < 0 {
		return "", false
	}
	return name[i+1:], true
}
