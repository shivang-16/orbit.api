package mail

import (
	"context"
	"log"
	"strings"

	"github.com/shivang-16/orbit.api/internal/infra/resend"
)

type Service struct {
	client *resend.Client
}

func NewService(client *resend.Client) *Service {
	return &Service{client: client}
}

func (s *Service) SendWelcome(ctx context.Context, to, _firstName string) {
	to = strings.TrimSpace(to)
	if to == "" || !strings.Contains(to, "@") {
		log.Printf("mail: skipping welcome email because recipient is invalid")
		return
	}
	if s == nil || s.client == nil || !s.client.Enabled() {
		log.Printf("mail: RESEND_API_KEY is not configured; skipping welcome email")
		return
	}

	log.Printf("mail: sending welcome email to=%s from=%s subject=%q", to, s.client.From(), welcomeEmailSubject)

	id, err := s.client.Send(ctx, resend.SendParams{
		To:      to,
		Subject: welcomeEmailSubject,
		HTML:    welcomeEmailHTML(),
		Text:    welcomeEmailText(),
	})
	if err != nil {
		log.Printf("mail: welcome email failed to=%s from=%s: %v", to, s.client.From(), err)
		return
	}
	log.Printf("mail: welcome email sent to=%s from=%s email_id=%s", to, s.client.From(), id)
}
