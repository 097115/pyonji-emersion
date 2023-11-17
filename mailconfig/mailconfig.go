package mailconfig

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrNotFound = errors.New("mailautoconfig: no mail server found")

type SMTP struct {
	Hostname string
	Port     string
	StartTLS bool

	Username string
}

type provider interface {
	DiscoverSMTP(ctx context.Context, addr, domain string) (*SMTP, error)
}

func discoverSMTP(ctx context.Context, addr, domain string, withMXGuess bool) (*SMTP, error) {
	providers := []provider{
		dnsSRVProvider{},
		mozillaISPDBProvider{},
		mozillaSubdomainProvider{},
		subdomainGuessProvider{"mail", false},
		subdomainGuessProvider{"smtp", false},
		subdomainGuessProvider{"mail", true},
		subdomainGuessProvider{"smtp", true},
	}
	if withMXGuess {
		providers = append(providers, dnsMXGuessProvider{})
	}

	providerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]*providerResult, len(providers))
	for i := range providers {
		p := providers[i]
		res := &providerResult{done: make(chan struct{})}
		results[i] = res

		go func() {
			defer close(res.done)
			res.cfg, res.err = p.DiscoverSMTP(providerCtx, addr, domain)
		}()
	}

	var err error
	for _, res := range results {
		select {
		case <-res.done:
			// ok
		case <-ctx.Done():
			return nil, ErrNotFound
		}
		if res.cfg != nil {
			if res.cfg.Username == "" {
				res.cfg.Username = addr
			}
			return res.cfg, nil
		}
		if res.err != nil && res.err != ErrNotFound && !errors.Is(res.err, context.DeadlineExceeded) && err == nil {
			err = res.err
		}
	}
	if err == nil {
		err = ErrNotFound
	}
	return nil, err
}

func DiscoverSMTP(ctx context.Context, addr string) (*SMTP, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	_, domain, _ := strings.Cut(addr, "@")
	return discoverSMTP(ctx, addr, domain, true)
}

type providerResult struct {
	done chan struct{}
	err  error
	cfg  *SMTP
}
