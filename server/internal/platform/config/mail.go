package config

import (
	"fmt"
	netmail "net/mail"
	"strconv"
	"strings"
)

const (
	MailProviderSMTP   = "smtp"
	MailProviderResend = "resend"
)

const (
	SMTPTLSMandatory     = "mandatory"
	SMTPTLSOpportunistic = "opportunistic"
	SMTPTLSNone          = "none"
)

type MailConfig struct {
	Provider string
	From     string
	ReplyTo  string
	SMTP     SMTPConfig
	Resend   ResendConfig
}

type SMTPConfig struct {
	Host string
	Port int
	User string
	Pass string
	TLS  string
}

type ResendConfig struct {
	APIKey string
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(GetEnv(key, "")); v != "" {
		return v
	}
	return fallback
}

func LoadMailConfig() (MailConfig, error) {
	port, err := strconv.Atoi(envOr("SMTP_PORT", "587"))
	if err != nil {
		return MailConfig{}, fmt.Errorf("invalid SMTP_PORT: %w", err)
	}

	cfg := MailConfig{
		Provider: strings.ToLower(envOr("MAIL_PROVIDER", MailProviderSMTP)),
		From:     envOr("MAIL_FROM", envOr("SMTP_FROM", "")),
		ReplyTo:  envOr("MAIL_REPLY_TO", ""),
		SMTP: SMTPConfig{
			Host: strings.TrimSpace(GetEnv("SMTP_HOST", "")),
			Port: port,
			User: GetEnv("SMTP_USER", ""),
			Pass: GetEnv("SMTP_PASS", ""),
			TLS:  strings.ToLower(envOr("SMTP_TLS", SMTPTLSMandatory)),
		},
		Resend: ResendConfig{
			APIKey: strings.TrimSpace(GetEnv("RESEND_API_KEY", "")),
		},
	}

	if err := cfg.Validate(); err != nil {
		return MailConfig{}, err
	}

	return cfg, nil
}

func (c MailConfig) Validate() error {
	var missing []string

	if c.From == "" {
		missing = append(missing, "MAIL_FROM")
	} else if _, err := netmail.ParseAddress(c.From); err != nil {
		return fmt.Errorf("mail: MAIL_FROM %q bukan alamat yang sah: %w", c.From, err)
	}

	if c.ReplyTo != "" {
		if _, err := netmail.ParseAddress(c.ReplyTo); err != nil {
			return fmt.Errorf("mail: MAIL_REPLY_TO %q bukan alamat yang sah: %w", c.ReplyTo, err)
		}
	}

	switch c.Provider {
	case MailProviderSMTP:
		if c.SMTP.Host == "" {
			missing = append(missing, "SMTP_HOST")
		}
		switch c.SMTP.TLS {
		case SMTPTLSMandatory, SMTPTLSOpportunistic, SMTPTLSNone:
		default:
			return fmt.Errorf("mail: SMTP_TLS %q tidak dikenal (mandatory, opportunistic, none)", c.SMTP.TLS)
		}
	case MailProviderResend:
		if c.Resend.APIKey == "" {
			missing = append(missing, "RESEND_API_KEY")
		}
	default:
		return fmt.Errorf("mail: MAIL_PROVIDER %q tidak dikenal (smtp, resend)", c.Provider)
	}

	if len(missing) > 0 {
		return fmt.Errorf("mail: provider %s butuh %s", c.Provider, strings.Join(missing, ", "))
	}

	return nil
}
