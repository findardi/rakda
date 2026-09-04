package sender

import (
	"context"
	"fmt"

	"github.com/findardi/rakda/server/internal/platform/config"
	"github.com/wneessen/go-mail"
)

type smtpSender struct {
	from    string
	replyTo string
	client  *mail.Client
}

func newSMTP(from, replyTo string, cfg config.SMTPConfig) (*smtpSender, error) {
	policy, err := smtpTLSPolicy(cfg.TLS)
	if err != nil {
		return nil, err
	}

	opts := []mail.Option{
		mail.WithPort(cfg.Port),
		mail.WithTLSPolicy(policy),
	}
	if cfg.User != "" {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(cfg.User),
			mail.WithPassword(cfg.Pass),
		)
	}

	client, err := mail.NewClient(cfg.Host, opts...)
	if err != nil {
		return nil, fmt.Errorf("sender: smtp: %w", err)
	}

	return &smtpSender{from: from, replyTo: replyTo, client: client}, nil
}

func (s *smtpSender) Send(ctx context.Context, to, subject, textBody, htmlBody string) error {
	msg, err := s.message(to, subject, textBody, htmlBody)
	if err != nil {
		return err
	}

	if err := s.client.DialAndSendWithContext(ctx, msg); err != nil {
		return fmt.Errorf("smtp: dial and send: %w", err)
	}

	return nil
}

func (s *smtpSender) message(to, subject, textBody, htmlBody string) (*mail.Msg, error) {
	msg := mail.NewMsg()
	if err := msg.From(s.from); err != nil {
		return nil, fmt.Errorf("smtp: from: %w", err)
	}
	if err := msg.To(to); err != nil {
		return nil, fmt.Errorf("smtp: to: %w", err)
	}
	if s.replyTo != "" {
		if err := msg.ReplyTo(s.replyTo); err != nil {
			return nil, fmt.Errorf("smtp: reply-to: %w", err)
		}
	}

	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, textBody)
	msg.AddAlternativeString(mail.TypeTextHTML, htmlBody)

	return msg, nil
}

func smtpTLSPolicy(v string) (mail.TLSPolicy, error) {
	switch v {
	case config.SMTPTLSMandatory:
		return mail.TLSMandatory, nil
	case config.SMTPTLSOpportunistic:
		return mail.TLSOpportunistic, nil
	case config.SMTPTLSNone:
		return mail.NoTLS, nil
	default:
		return 0, fmt.Errorf("sender: smtp: kebijakan TLS %q tidak dikenal", v)
	}
}
