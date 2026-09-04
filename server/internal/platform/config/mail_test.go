package config

import (
	"strings"
	"testing"
)

func validSMTP() MailConfig {
	return MailConfig{Provider: MailProviderSMTP, From: "Rakda <no-reply@rakda.id>",
		SMTP: SMTPConfig{Host: "smtp.example.com", Port: 587, TLS: SMTPTLSMandatory}}
}

func validResend() MailConfig {
	return MailConfig{Provider: MailProviderResend, From: "Rakda <no-reply@rakda.id>",
		Resend: ResendConfig{APIKey: "re_123"}}
}

func TestMailConfigValidate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*MailConfig)
		base    MailConfig
		wantErr string // "" = valid
	}{
		{name: "smtp valid", base: validSMTP()},
		{name: "resend valid", base: validResend()},
		{name: "reply-to valid", base: validResend(), mutate: func(c *MailConfig) { c.ReplyTo = "Dukungan <halo@rakda.id>" }},
		{name: "smtp without host", base: validSMTP(), mutate: func(c *MailConfig) { c.SMTP.Host = "" }, wantErr: "SMTP_HOST"},
		{name: "smtp unknown tls", base: validSMTP(), mutate: func(c *MailConfig) { c.SMTP.TLS = "starttls" }, wantErr: "SMTP_TLS"},
		{name: "resend without key", base: validResend(), mutate: func(c *MailConfig) { c.Resend.APIKey = "" }, wantErr: "RESEND_API_KEY"},
		{name: "missing from", base: validResend(), mutate: func(c *MailConfig) { c.From = "" }, wantErr: "MAIL_FROM"},
		{name: "malformed from", base: validResend(), mutate: func(c *MailConfig) { c.From = "Rakda <no-reply@" }, wantErr: "MAIL_FROM"},
		{name: "malformed reply-to", base: validResend(), mutate: func(c *MailConfig) { c.ReplyTo = "bukan alamat" }, wantErr: "MAIL_REPLY_TO"},
		{name: "unknown provider", base: MailConfig{Provider: "sendgrid", From: "x@y.id"}, wantErr: "MAIL_PROVIDER"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.base
			if tc.mutate != nil {
				tc.mutate(&cfg)
			}

			err := cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want mention of %s", err, tc.wantErr)
			}
		})
	}
}

// Env lama (tanpa MAIL_PROVIDER, hanya SMTP_FROM) dan nilai kosong dari berkas
// sampel harus tetap memuat sebagai smtp dengan TLS wajib.
func TestLoadMailConfigDefaults(t *testing.T) {
	t.Setenv("MAIL_PROVIDER", "")
	t.Setenv("MAIL_FROM", "")
	t.Setenv("MAIL_REPLY_TO", "")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_FROM", "no-reply@rakda.id")
	t.Setenv("SMTP_TLS", "")
	t.Setenv("RESEND_API_KEY", "")

	cfg, err := LoadMailConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != MailProviderSMTP || cfg.From != "no-reply@rakda.id" ||
		cfg.ReplyTo != "" || cfg.SMTP.TLS != SMTPTLSMandatory {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestLoadMailConfigResend(t *testing.T) {
	t.Setenv("MAIL_PROVIDER", "Resend")
	t.Setenv("MAIL_FROM", "Rakda <no-reply@rakda.id>")
	t.Setenv("MAIL_REPLY_TO", "halo@rakda.id")
	t.Setenv("RESEND_API_KEY", " re_abc ")
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_PORT", "")
	t.Setenv("SMTP_TLS", "")

	cfg, err := LoadMailConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != MailProviderResend || cfg.Resend.APIKey != "re_abc" || cfg.ReplyTo != "halo@rakda.id" {
		t.Fatalf("cfg = %+v", cfg)
	}
}
