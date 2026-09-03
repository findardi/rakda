package sender

import (
	"context"

	"github.com/findardi/rakda/server/internal/platform/config"
	"github.com/wneessen/go-mail"
)

type Mailer struct {
	cfg config.MailConfig
}

func New(cfg config.MailConfig) *Mailer {
	return &Mailer{
		cfg: cfg,
	}
}

func (m *Mailer) Send(ctx context.Context, to, subject, textBody, htmlBody string) error {
	client, err := mail.NewClient(m.cfg.Host,
		mail.WithPort(m.cfg.Port),
		mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(m.cfg.User),
		mail.WithPassword(m.cfg.Pass),
		mail.WithTLSPolicy(mail.TLSMandatory),
	)

	if err != nil {
		return err
	}

	msg := mail.NewMsg()
	if err := msg.From(m.cfg.From); err != nil {
		return err
	}
	if err := msg.To(to); err != nil {
		return err
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, textBody)
	msg.AddAlternativeString(mail.TypeTextHTML, htmlBody)

	return client.DialAndSendWithContext(ctx, msg)

}
