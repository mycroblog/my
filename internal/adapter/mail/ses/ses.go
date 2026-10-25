// SPDX-License-Identifier: AGPL-3.0-only

package ses

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/aws/smithy-go"

	"github.com/mycroblog/my/internal/adapter/mail/internal/message"
	"github.com/mycroblog/my/internal/app/kernel"
)

// DefaultTimeout bounds one Send, the retries of the SDK included.
const DefaultTimeout = 30 * time.Second

// Client is the part of *sesv2.Client that the Mailer uses.
type Client interface {
	SendEmail(ctx context.Context, in *sesv2.SendEmailInput, optFns ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error)
}

// Options configure a Mailer.
type Options struct {
	// From is the sender address, a bare address on an identity verified in SES.
	From string
	// ConfigurationSet names the configuration set that SES applies to each message (its event
	// destinations, such as the SNS topic of the bounces); empty sends without one.
	ConfigurationSet string
	// Timeout bounds one Send; zero means DefaultTimeout.
	Timeout time.Duration
}

// Mailer sends messages through SES. It is safe for concurrent use.
type Mailer struct {
	client Client
	o      Options
}

var _ kernel.Mailer = (*Mailer)(nil)

// New returns a Mailer that sends through client. It fails when From is not a bare address.
func New(client Client, o Options) (*Mailer, error) {
	if a, err := mail.ParseAddress(o.From); err != nil || a.Name != "" || a.Address != o.From {
		return nil, errors.New("ses: the sender must be a bare address")
	}
	from, err := message.ASCIIAddress(o.From)
	if err != nil {
		return nil, fmt.Errorf("ses: the sender: %w", err)
	}
	o.From = from
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	return &Mailer{client: client, o: o}, nil
}

// NewFromConfig returns a Mailer on a *sesv2.Client of cfg (its region, credentials, HTTP
// client, and endpoint). It fails when cfg has no region.
func NewFromConfig(cfg aws.Config, o Options) (*Mailer, error) {
	if cfg.Region == "" {
		return nil, errors.New("ses: no AWS region: set ses_region or AWS_REGION")
	}
	return New(sesv2.NewFromConfig(cfg), o)
}

// Send implements kernel.Mailer.
func (m *Mailer) Send(ctx context.Context, msg kernel.MailMessage) error {
	if err := kernel.ValidateMail(msg); err != nil {
		return err
	}
	raw, to, err := message.Compose(m.o.From, msg, time.Now())
	if err != nil {
		return err
	}
	in := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(m.o.From),
		Destination:      &types.Destination{ToAddresses: []string{to}},
		Content:          &types.EmailContent{Raw: &types.RawMessage{Data: raw}},
	}
	if m.o.ConfigurationSet != "" {
		in.ConfigurationSetName = aws.String(m.o.ConfigurationSet)
	}
	ctx, cancel := context.WithTimeout(ctx, m.o.Timeout)
	defer cancel()
	if _, err := m.client.SendEmail(ctx, in); err != nil {
		return classify(err)
	}
	return nil
}

// classify returns the error of a SendEmail that failed: kernel.ErrMailRejected for
// MessageRejected and a temporary failure otherwise, named by the error code of SES without
// its message.
func classify(err error) error {
	var api smithy.APIError
	if !errors.As(err, &api) {
		return fmt.Errorf("ses: SendEmail: %w", err)
	}
	var rejected *types.MessageRejected
	if errors.As(err, &rejected) || api.ErrorCode() == "MessageRejected" {
		return fmt.Errorf("ses: SendEmail: %s: %w", api.ErrorCode(), kernel.ErrMailRejected)
	}
	return fmt.Errorf("ses: SendEmail: %s", api.ErrorCode())
}
