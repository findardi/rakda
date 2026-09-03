package sender

import (
	"bytes"
	"embed"
	"fmt"
	html "html/template"
	text "text/template"
	"time"
)

const (
	ColorBg      = "#F7FBFC"
	ColorCard    = "#FFFFFF"
	ColorInk     = "#111C23"
	ColorPrimary = "#006B82"
	ColorBorder  = "#E3EAEC"
	ColorMuted   = "#5E6F76"
)

//go:embed templates
var templateFiles embed.FS

type Email struct {
	Subject string
	Text    string
	HTML    string
}

type OtpMail struct {
	Headline string
	Lead     string
	Code     string
	Closing  string
	Reason   string
}

type InviteMail struct {
	InvitedBy     string
	WorkspaceName string
	Link          string
	Registered    bool
	ExpiresLabel  string
	Reason        string
}

type emailLayoutData struct {
	Subject   string
	Preheader string
	Body      html.HTML
}

func parseHTML(path string) *html.Template {
	return html.Must(html.ParseFS(templateFiles, path))
}

func parseText(path string) *text.Template {
	return text.Must(text.ParseFS(templateFiles, path))
}

var (
	layoutHTML  = parseHTML("templates/layout.html")
	htmlContent = map[string]*html.Template{
		"verify": parseHTML("templates/verify.html"),
		"reset":  parseHTML("templates/reset.html"),
		"invite": parseHTML("templates/invite.html"),
	}
	textContent = map[string]*text.Template{
		"verify": parseText("templates/verify.txt"),
		"reset":  parseText("templates/reset.txt"),
		"invite": parseText("templates/invite.txt"),
	}
)

func execHTML(t *html.Template, data any) html.HTML {
	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		panic(fmt.Sprintf("sender: execute html template: %v", err))
	}
	return html.HTML(b.String())
}

func execText(t *text.Template, data any) string {
	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		panic(fmt.Sprintf("sender: execute text template: %v", err))
	}
	return b.String()
}

func buildEmail(kind, subject, preheader string, data any) Email {
	body := execHTML(htmlContent[kind], data)
	page := execHTML(layoutHTML, emailLayoutData{
		Subject:   subject,
		Preheader: preheader,
		Body:      body,
	})
	return Email{
		Subject: subject,
		Text:    execText(textContent[kind], data),
		HTML:    string(page),
	}
}

func BuildVerifyEmail(code string) Email {
	return buildEmail("verify", "Kode verifikasi Rakda Anda",
		"Gunakan kode di bawah untuk memverifikasi email Anda. Berlaku 5 menit.",
		OtpMail{
			Headline: "Verifikasi email Anda",
			Lead:     "Terima kasih sudah mendaftar di Rakda. Masukkan kode berikut untuk mengaktifkan akun Anda.",
			Code:     code,
			Reason:   "aktivasi akun Rakda Anda",
		})
}

func BuildResetPasswordEmail(code string) Email {
	return buildEmail("reset", "Atur ulang kata sandi Anda",
		"Kode untuk mengatur ulang kata sandi akun Rakda Anda. Berlaku 5 menit.",
		OtpMail{
			Headline: "Atur ulang kata sandi",
			Lead:     "Kami menerima permintaan atur ulang kata sandi untuk akun Rakda Anda. Masukkan kode berikut untuk melanjutkan.",
			Code:     code,
			Closing:  "Tidak merasa meminta ini? Abaikan email ini — kata sandi Anda tidak berubah.",
			Reason:   "permintaan atur ulang kata sandi",
		})
}

func BuildInviteEmail(webURL, invitedBy, workspaceName, token string, registered bool, expiresAt time.Time) Email {
	if invitedBy == "" {
		invitedBy = "Pengelola ruang"
	}

	subject := "Anda diundang ke ruang data"
	if workspaceName != "" {
		subject = "Anda diundang ke " + workspaceName
	}

	link := webURL + "/invitation"
	if !registered {
		link = webURL + "/invitations/accept?token=" + token
	}

	label := ""
	if !expiresAt.IsZero() {
		label = FormatDateID(expiresAt)
	}

	return buildEmail("invite", subject,
		"Buka undangan Anda untuk menerima atau menolak.",
		InviteMail{
			InvitedBy:     invitedBy,
			WorkspaceName: workspaceName,
			Link:          link,
			Registered:    registered,
			ExpiresLabel:  label,
			Reason:        "undangan ke ruang data di Rakda",
		})
}

func FormatDateID(t time.Time) string {
	months := [...]string{
		"Januari", "Februari", "Maret", "April", "Mei", "Juni",
		"Juli", "Agustus", "September", "Oktober", "November", "Desember",
	}
	return fmt.Sprintf("%d %s %d", t.Day(), months[int(t.Month())-1], t.Year())
}
