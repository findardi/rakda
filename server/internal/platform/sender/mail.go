package sender

import (
	"context"
	"fmt"

	"github.com/findardi/rakda/server/internal/platform/config"
)

type Sender interface {
	Send(ctx context.Context, to, subject, textBody, htmlBody string) error
}

func New(cfg config.MailConfig) (Sender, error) {
	switch cfg.Provider {
	case config.MailProviderSMTP:
		return newSMTP(cfg.From, cfg.ReplyTo, cfg.SMTP)
	case config.MailProviderResend:
		return newResend(cfg.From, cfg.ReplyTo, cfg.Resend), nil
	default:
		return nil, fmt.Errorf("sender: MAIL_PROVIDER %q tidak dikenal", cfg.Provider)
	}
}
