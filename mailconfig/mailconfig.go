package mailconfig

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("mailautoconfig: no mail server found")

type SMTP struct {
	Hostname string
	Port     string
	STARTTLS bool
}

type provider interface {
	DiscoverSMTP(ctx context.Context, address string) (*SMTP, error)
}

func DiscoverSMTP(ctx context.Context, address string) (*SMTP, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var (
		dns     dnsProvider
		mozilla mozillaProvider
	)

	providers := []provider{&dns, &mozilla}
	results := make([]*providerResult, len(providers))
	for i := range providers {
		p := providers[i]
		res := &providerResult{done: make(chan struct{})}
		results[i] = res

		go func() {
			defer close(res.done)
			res.cfg, res.err = p.DiscoverSMTP(ctx, address)
		}()
	}

	var err error
	for _, res := range results {
		<-res.done
		if res.cfg != nil {
			return res.cfg, nil
		}
		if res.err != nil && res.err != ErrNotFound && err == nil {
			err = res.err
		}
	}
	if err == nil {
		err = ErrNotFound
	}
	return nil, err
}

type providerResult struct {
	done chan struct{}
	err  error
	cfg  *SMTP
}
