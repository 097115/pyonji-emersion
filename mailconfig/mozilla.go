package mailconfig

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

const mozillaISPDB = "https://autoconfig.thunderbird.net/v1.1/"

// See:
// https://wiki.mozilla.org/Thunderbird:Autoconfiguration:ConfigFileFormat
type mozillaConfig struct {
	EmailProvider struct {
		OutgoingServer []struct {
			Type       string            `xml:"type,attr"`
			Hostname   string            `xml:"hostname"`
			Port       string            `xml:"port"`
			SocketType mozillaSocketType `xml:"socketType"`
			Username   string            `xml:"username"`
			Auth       mozillaAuth       `xml:"authentication"`
		} `xml:"outgoingServer"`
	} `xml:"emailProvider"`
}

type mozillaSocketType string

const (
	mozillaSocketSSL      mozillaSocketType = "SSL"
	mozillaSocketSTARTTLS mozillaSocketType = "STARTTLS"
)

type mozillaAuth string

const (
	mozillaAuthPasswordCleartext mozillaAuth = "password-cleartext"
)

type mozillaProvider struct{}

var _ provider = mozillaProvider{}

// DiscoverSMTP looks up the Mozilla ISPDB. See:
// https://wiki.mozilla.org/Thunderbird:Autoconfiguration
func (mozillaProvider) DiscoverSMTP(ctx context.Context, address string) (*SMTP, error) {
	_, domain, _ := strings.Cut(address, "@")

	url := mozillaISPDB + domain
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusOK:
		// go on
	default:
		return nil, fmt.Errorf("HTTP error: %v", resp.Status)
	}

	var data mozillaConfig
	if err := xml.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var startTLSCfg *SMTP
	for _, srv := range data.EmailProvider.OutgoingServer {
		if srv.Type != "smtp" || srv.Auth != mozillaAuthPasswordCleartext {
			continue
		}

		cfg := &SMTP{
			Hostname: srv.Hostname,
			Port:     srv.Port,
		}
		switch srv.SocketType {
		case mozillaSocketSSL:
			return cfg, nil
		case mozillaSocketSTARTTLS:
			cfg.STARTTLS = true
			startTLSCfg = cfg
		default:
			continue
		}
	}
	if startTLSCfg != nil {
		return startTLSCfg, nil
	}

	return nil, ErrNotFound
}
