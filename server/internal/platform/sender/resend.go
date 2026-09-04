package sender

import (
	"context"
	"fmt"

	"github.com/findardi/rakda/server/internal/platform/config"
	"github.com/resend/resend-go/v3"
)

type resendSender struct {
	from    string
	replyTo string
	client  *resend.Client
}

func newResend(from, replyTo string, cfg config.ResendConfig) *resendSender {
	return &resendSender{from: from, replyTo: replyTo, client: resend.NewClient(cfg.APIKey)}
}

func (r *resendSender) Send(ctx context.Context, to, subject, textBody, htmlBody string) error {
	if _, err := r.client.Emails.SendWithContext(ctx, &resend.SendEmailRequest{
		From:    r.from,
		ReplyTo: r.replyTo,
		To:      []string{to},
		Subject: subject,
		Text:    textBody,
		Html:    htmlBody,
	}); err != nil {
		return fmt.Errorf("resend: send: %w", err)
	}

	return nil
}
