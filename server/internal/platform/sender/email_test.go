package sender

import (
	"strings"
	"testing"
	"time"
)

// Setiap jenis pesan harus ter-render penuh tanpa sisa placeholder.
func TestBuildEmailsRenderFully(t *testing.T) {
	cases := []struct {
		name string
		em   Email
	}{
		{"verify", BuildVerifyEmail("123456")},
		{"reset", BuildResetPasswordEmail("654321")},
		{"invite_registered", BuildInviteEmail("https://app.rakda.id", "Budi", "Proyek A",
			"tok", true, time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))},
		{"invite_new_user", BuildInviteEmail("https://app.rakda.id", "", "",
			"tok", false, time.Time{})},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for part, s := range map[string]string{
				"subject": c.em.Subject,
				"text":    c.em.Text,
				"html":    c.em.HTML,
			} {
				if strings.TrimSpace(s) == "" {
					t.Fatalf("%s kosong", part)
				}
				if strings.Contains(s, "{{") {
					t.Fatalf("%s masih mengandung placeholder: %q", part, s)
				}
			}
			if !strings.Contains(c.em.HTML, "Rakda") || !strings.Contains(c.em.Text, "Rakda") {
				t.Fatal("brand tidak ada di HTML dan teks")
			}
		})
	}
}

func TestFormatDateID(t *testing.T) {
	got := FormatDateID(time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC))
	if got != "3 September 2026" {
		t.Fatalf("FormatDateID = %q", got)
	}
}
