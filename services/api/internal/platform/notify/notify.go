// Package notify provides the SMS and email sender abstractions with clearly
// labelled local adapters. The fake SMS adapter never transmits anything; the
// Mailpit email adapter only delivers to a local capture server.
package notify

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/smtp"
)

// SMSSender sends (or pretends to send, locally) an SMS.
type SMSSender interface {
	Send(ctx context.Context, phone, body string) error
	Name() string
}

// FakeSMS logs the message and records it as simulated. It NEVER claims a
// real SMS was delivered and never touches a network.
type FakeSMS struct{ Log *slog.Logger }

func (f *FakeSMS) Name() string { return "fake (simulated, no messages sent)" }

func (f *FakeSMS) Send(ctx context.Context, phone, body string) error {
	f.Log.InfoContext(ctx, "SIMULATED SMS (not sent)",
		slog.String("to", phone), slog.String("body", body))
	return nil
}

// EmailSender sends a transactional email.
type EmailSender interface {
	Send(ctx context.Context, to, subject, textBody string) error
	Name() string
}

// MailpitEmail delivers to the local Mailpit capture server (dev only).
type MailpitEmail struct {
	Addr string // host:port of Mailpit SMTP
	From string
}

func (m *MailpitEmail) Name() string { return "mailpit (local capture)" }

func (m *MailpitEmail) Send(ctx context.Context, to, subject, textBody string) error {
	addr := m.Addr
	from := m.From
	if from == "" {
		from = "no-reply@biteorbit.local"
	}
	msg := bytes.Join([][]byte{
		[]byte(fmt.Sprintf("From: BiteOrbit <%s>\r\n", from)),
		[]byte(fmt.Sprintf("To: %s\r\n", to)),
		[]byte(fmt.Sprintf("Subject: %s\r\n", subject)),
		[]byte("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n"),
		[]byte(textBody),
	}, nil)
	// Mailpit accepts unauthenticated SMTP locally.
	if err := smtp.SendMail(addr, nil, from, []string{to}, msg); err != nil {
		return fmt.Errorf("mailpit send: %w", err)
	}
	return nil
}

// ConsoleEmail prints to the log (used in tests and when Mailpit is absent).
type ConsoleEmail struct{ Log *slog.Logger }

func (c *ConsoleEmail) Name() string { return "console (logged only)" }

func (c *ConsoleEmail) Send(ctx context.Context, to, subject, textBody string) error {
	c.Log.InfoContext(ctx, "SIMULATED EMAIL (logged only)",
		slog.String("to", to), slog.String("subject", subject), slog.String("body", textBody))
	return nil
}
