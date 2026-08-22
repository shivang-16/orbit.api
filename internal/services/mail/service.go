package mail

import (
	"context"
	"strings"

	"github.com/shivang-16/orbit.api/internal/infra/resend"
	"github.com/shivang-16/orbit.api/internal/logger"
)

type Service struct {
	client *resend.Client
}

func NewService(client *resend.Client) *Service {
	return &Service{client: client}
}

func (s *Service) SendWelcome(ctx context.Context, to, _firstName string) {
	to = strings.TrimSpace(to)
	ctx = logger.SetTag(ctx, logger.TagMail)
	ctx = logger.SetUser(ctx, logger.From(ctx).UserID, to)
	if to == "" || !strings.Contains(to, "@") {
		logger.Warn(ctx, "mail: skipping welcome email because recipient is invalid")
		return
	}
	if s == nil || s.client == nil || !s.client.Enabled() {
		logger.Warn(ctx, "mail: RESEND_API_KEY is not configured; skipping welcome email")
		return
	}

	logger.Info(ctx, "mail: sending welcome email", "from", s.client.From(), "subject", welcomeEmailSubject)

	id, err := s.client.Send(ctx, resend.SendParams{
		To:      to,
		Subject: welcomeEmailSubject,
		HTML:    welcomeEmailHTML(),
		Text:    welcomeEmailText(),
	})
	if err != nil {
		logger.Error(ctx, "mail: welcome email failed", "from", s.client.From(), "error", err)
		return
	}
	logger.Info(ctx, "mail: welcome email sent", "from", s.client.From(), "email_id", id)
}
