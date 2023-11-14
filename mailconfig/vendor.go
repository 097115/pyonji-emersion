package mailconfig

import (
	"strings"
)

func GetVendorPasswordHint(addr, smtpHostname string) string {
	switch smtpHostname {
	case "smtp.gmail.com":
		return "Enable two-factor authentication, then obtain an app password here:\n" +
			"https://security.google.com/settings/security/apppasswords"
	case "smtp.fastmail.com":
		return "Obtain an app password by following these instructions:\n" +
			"https://www.fastmail.help/hc/en-us/articles/360058752854"
	}

	// TODO: this doesn't work for custom domains
	_, hostname, _ := strings.Cut(addr, "@")
	switch hostname {
	case "protonmail.com", "protonmail.ch", "proton.me", "pm.me":
		return "Setup ProtonMail Bridge:\n" +
			"https://proton.me/mail/bridge"
	}

	return ""
}
