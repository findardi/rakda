package sender

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/findardi/rakda/server/internal/platform/config"
	"github.com/resend/resend-go/v3"
	"github.com/wneessen/go-mail"
)

func smtpCfg() config.MailConfig {
	return config.MailConfig{Provider: config.MailProviderSMTP, From: "Rakda <no-reply@rakda.id>",
		SMTP: config.SMTPConfig{Host: "smtp.example.com", Port: 587, TLS: config.SMTPTLSMandatory}}
}

func resendCfg() config.MailConfig {
	return config.MailConfig{Provider: config.MailProviderResend, From: "Rakda <no-reply@rakda.id>",
		Resend: config.ResendConfig{APIKey: "re_test"}}
}

func TestNewSelectsProvider(t *testing.T) {
	cases := []struct {
		name    string
		cfg     config.MailConfig
		want    string // "" = error
		wantErr bool
	}{
		{name: "smtp", cfg: smtpCfg(), want: "smtp"},
		{name: "resend", cfg: resendCfg(), want: "resend"},
		{name: "unknown", cfg: config.MailConfig{Provider: "sendgrid"}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := New(tc.cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			var got string
			switch s.(type) {
			case *smtpSender:
				got = "smtp"
			case *resendSender:
				got = "resend"
			}
			if got != tc.want {
				t.Fatalf("transport = %q, want %q", got, tc.want)
			}
		})
	}
}

// Opsi go-mail yang salah harus gagal saat boot, bukan pada kiriman pertama.
func TestNewSMTPRejectsInvalidPortAtBoot(t *testing.T) {
	cfg := smtpCfg()
	cfg.SMTP.Port = 0
	if _, err := New(cfg); err == nil {
		t.Fatal("expected boot-time error for port 0")
	}
}

// Reply-To kosong tidak boleh menggagalkan pesan — regresi dari smtp.go yang
// memanggil msg.ReplyTo("") tanpa syarat.
func TestSMTPMessageReplyTo(t *testing.T) {
	cases := []struct {
		name    string
		replyTo string
		want    []string // header Reply-To yang diharapkan; nil = tidak ada
	}{
		{name: "absent", replyTo: ""},
		{name: "set", replyTo: "Dukungan <halo@rakda.id>", want: []string{"\"Dukungan\" <halo@rakda.id>"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := newSMTP("Rakda <no-reply@rakda.id>", tc.replyTo, smtpCfg().SMTP)
			if err != nil {
				t.Fatal(err)
			}

			msg, err := s.message("budi@example.com", "Kode", "teks", "<p>html</p>")
			if err != nil {
				t.Fatalf("message: %v", err)
			}

			got := msg.GetAddrHeaderString(mail.HeaderReplyTo)
			if len(got) != len(tc.want) {
				t.Fatalf("Reply-To = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("Reply-To = %v, want %v", got, tc.want)
				}
			}
			if from := msg.GetFromString(); len(from) != 1 || !strings.Contains(from[0], "no-reply@rakda.id") {
				t.Fatalf("From = %v", from)
			}
		})
	}
}

func TestSMTPTLSPolicy(t *testing.T) {
	cases := []struct {
		in   string
		want mail.TLSPolicy
		err  bool
	}{
		{in: config.SMTPTLSMandatory, want: mail.TLSMandatory},
		{in: config.SMTPTLSOpportunistic, want: mail.TLSOpportunistic},
		{in: config.SMTPTLSNone, want: mail.NoTLS},
		{in: "starttls", err: true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := smtpTLSPolicy(tc.in)
			if tc.err {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
}

type capturedSend struct {
	From    string   `json:"from"`
	ReplyTo string   `json:"reply_to"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Html    string   `json:"html"`
	Text    string   `json:"text"`
}

func resendTestServer(t *testing.T, status int, body string, got *capturedSend, auth, path *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*auth = r.Header.Get("Authorization")
		*path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(got); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "2")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestResendSendMapsRequest(t *testing.T) {
	var got capturedSend
	var auth, path string
	srv := resendTestServer(t, http.StatusOK, `{"id":"em_123"}`, &got, &auth, &path)
	defer srv.Close()

	s := newResend("Rakda <no-reply@rakda.id>", "halo@rakda.id", config.ResendConfig{APIKey: "re_test"})
	s.client.BaseURL, _ = url.Parse(srv.URL + "/")

	if err := s.Send(context.Background(), "budi@example.com", "Kode", "teks", "<p>html</p>"); err != nil {
		t.Fatal(err)
	}

	if auth != "Bearer re_test" {
		t.Fatalf("authorization = %q", auth)
	}
	if !strings.HasSuffix(path, "/emails") {
		t.Fatalf("path = %q", path)
	}
	want := capturedSend{From: "Rakda <no-reply@rakda.id>", ReplyTo: "halo@rakda.id",
		To: []string{"budi@example.com"}, Subject: "Kode", Html: "<p>html</p>", Text: "teks"}
	if got.From != want.From || got.ReplyTo != want.ReplyTo || len(got.To) != 1 || got.To[0] != want.To[0] ||
		got.Subject != want.Subject || got.Text != want.Text || got.Html != want.Html {
		t.Fatalf("request = %+v, want %+v", got, want)
	}
}

// Kegagalan API tetap bisa diperiksa tipenya oleh pemanggil (dibungkus %w).
func TestResendSendWrapsRateLimit(t *testing.T) {
	var got capturedSend
	var auth, path string
	srv := resendTestServer(t, http.StatusTooManyRequests,
		`{"statusCode":429,"message":"rate limit","name":"rate_limit_exceeded"}`, &got, &auth, &path)
	defer srv.Close()

	s := newResend("no-reply@rakda.id", "", config.ResendConfig{APIKey: "re_test"})
	s.client.BaseURL, _ = url.Parse(srv.URL + "/")

	err := s.Send(context.Background(), "budi@example.com", "Kode", "teks", "<p>html</p>")
	var rl *resend.RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("err = %v, want *resend.RateLimitError in chain", err)
	}
}
