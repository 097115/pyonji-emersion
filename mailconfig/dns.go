package mailconfig

import (
	"context"
	"net"
	"strings"
)

func discoverTCP(ctx context.Context, service, name string) (string, int, error) {
	var resolver net.Resolver
	_, addrs, err := resolver.LookupSRV(ctx, service, "tcp", name)
	if dnsErr, ok := err.(*net.DNSError); ok {
		if dnsErr.IsTemporary {
			return "", 0, err
		}
	} else if err != nil {
		return "", 0, err
	}

	if len(addrs) == 0 {
		return "", 0, nil
	}
	addr := addrs[0]

	target := strings.TrimSuffix(addr.Target, ".")
	if target == "" {
		return "", 0, nil
	}

	return target, int(addr.Port), nil
}

type dnsProvider struct{}

var _ provider = dnsProvider{}

// DiscoverSMTP performs a DNS-based SMTP submission service discovery, as
// defined in RFC 6186 section 3.1. RFC 8314 section 5.1 adds a new service for
// SMTP submission with implicit TLS.
func (dnsProvider) DiscoverSMTP(ctx context.Context, address string) (*SMTP, error) {
	_, domain, _ := strings.Cut(address, "@")

	hostname, port, err := discoverTCP(ctx, "submissions", domain)
	if err != nil {
		return nil, err
	} else if hostname != "" {
		return &SMTP{Hostname: hostname, Port: port}, nil
	}

	hostname, port, err = discoverTCP(ctx, "submission", domain)
	if err != nil {
		return nil, err
	} else if hostname != "" {
		return &SMTP{Hostname: hostname, Port: port, STARTTLS: true}, nil
	}

	return nil, ErrNotFound
}
