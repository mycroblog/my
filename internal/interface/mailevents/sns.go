// SPDX-License-Identifier: AGPL-3.0-only

package mailevents

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// The verification of Amazon SNS messages (ADR-0086 §5), after the AWS documentation of
// verifying the signatures of SNS messages (https://docs.aws.amazon.com/sns/latest/dg/sns-verify-signature-of-message.html):
// the signature covers a string of fixed fields of the message, each name and value on a line of
// their own, in the order of the names; SignatureVersion 2 signs it with SHA256withRSA.

// The types of SNS messages.
const (
	typeNotification             = "Notification"
	typeSubscriptionConfirmation = "SubscriptionConfirmation"
	typeUnsubscribeConfirmation  = "UnsubscribeConfirmation"
)

// signatureVersion is the only SignatureVersion that the endpoint accepts: SHA256withRSA.
const signatureVersion = "2"

// certNamespace is the KeyValueStore namespace of the signing certificates, keyed by their URL.
const certNamespace = "mailevents_sns_cert"

// certTTL is how long a signing certificate stays cached. SNS publishes a new certificate under a
// new URL, so the cache only bounds how long an unused one stays.
const certTTL = 24 * time.Hour

// errRejected reports a message that the endpoint refuses for good: it is not a valid message of
// the configured topic.
var errRejected = errors.New("mailevents: SNS message rejected")

// rejected returns an error wrapping errRejected with the reason.
func rejected(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errRejected, fmt.Sprintf(format, args...))
}

// snsMessage is the JSON document that SNS posts to an HTTPS subscription.
type snsMessage struct {
	Type             string  `json:"Type"`
	MessageID        string  `json:"MessageId"`
	Token            string  `json:"Token"`
	TopicArn         string  `json:"TopicArn"`
	Subject          *string `json:"Subject"`
	Message          string  `json:"Message"`
	Timestamp        string  `json:"Timestamp"`
	SignatureVersion string  `json:"SignatureVersion"`
	Signature        string  `json:"Signature"`
	SigningCertURL   string  `json:"SigningCertURL"`
	SubscribeURL     string  `json:"SubscribeURL"`
}

// stringToSign returns the string that the signature of m covers, and false for a type of
// message that has none.
func (m *snsMessage) stringToSign() (string, bool) {
	var b strings.Builder
	field := func(name, value string) {
		b.WriteString(name)
		b.WriteByte('\n')
		b.WriteString(value)
		b.WriteByte('\n')
	}
	switch m.Type {
	case typeNotification:
		field("Message", m.Message)
		field("MessageId", m.MessageID)
		if m.Subject != nil {
			field("Subject", *m.Subject)
		}
		field("Timestamp", m.Timestamp)
		field("TopicArn", m.TopicArn)
		field("Type", m.Type)
	case typeSubscriptionConfirmation, typeUnsubscribeConfirmation:
		field("Message", m.Message)
		field("MessageId", m.MessageID)
		field("SubscribeURL", m.SubscribeURL)
		field("Timestamp", m.Timestamp)
		field("Token", m.Token)
		field("TopicArn", m.TopicArn)
		field("Type", m.Type)
	default:
		return "", false
	}
	return b.String(), true
}

// TopicARN is the ARN of an SNS topic, arn:<partition>:sns:<region>:<account>:<name>.
type TopicARN struct {
	raw    string
	region string
}

// topicARNPattern matches the ARN of an SNS topic and captures its region.
var topicARNPattern = regexp.MustCompile(`^arn:aws[a-z-]*:sns:([a-z]{2}(?:-[a-z]+)+-[0-9]+):[0-9]{12}:[A-Za-z0-9_.-]{1,256}$`)

// ParseTopicARN parses the ARN of an SNS topic.
func ParseTopicARN(s string) (TopicARN, error) {
	m := topicARNPattern.FindStringSubmatch(s)
	if m == nil {
		return TopicARN{}, fmt.Errorf("not the ARN of an SNS topic arn:<partition>:sns:<region>:<account>:<topic>: %q", s)
	}
	return TopicARN{raw: s, region: m[1]}, nil
}

// String returns the ARN.
func (t TopicARN) String() string { return t.raw }

// snsHost returns the host of the SNS endpoint of the topic's region, the only host that the
// signing certificates and the subscription confirmations come from.
func (t TopicARN) snsHost() string { return "sns." + t.region + ".amazonaws.com" }

// snsURL parses raw and reports whether it is an https URL on the topic's SNS host, without
// credentials or a port.
func (t TopicARN) snsURL(raw string) (*url.URL, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host != t.snsHost() {
		return nil, false
	}
	return u, true
}

// verifier checks the signatures of the messages of one topic.
type verifier struct {
	topic  TopicARN
	client kernel.HTTPClient
	kv     kernel.KeyValueStore
	logger *slog.Logger
}

// verify checks that m is a message of the topic signed by SNS. It fails with an error wrapping
// errRejected for a message to refuse, and with another error when the signing certificate could
// not be fetched, for SNS to send the message again.
func (v *verifier) verify(ctx context.Context, m *snsMessage) error {
	if m.TopicArn != v.topic.String() {
		return rejected("topic %q is not the configured topic", m.TopicArn)
	}
	if m.SignatureVersion != signatureVersion {
		return rejected("SignatureVersion %q, want %s", m.SignatureVersion, signatureVersion)
	}
	signed, ok := m.stringToSign()
	if !ok {
		return rejected("unknown message type %q", m.Type)
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil || len(sig) == 0 {
		return rejected("the signature is not base64")
	}
	certURL, ok := v.topic.snsURL(m.SigningCertURL)
	if !ok || certURL.RawQuery != "" || !strings.HasSuffix(certURL.Path, ".pem") {
		return rejected("SigningCertURL is not a certificate on %s", v.topic.snsHost())
	}
	key, err := v.publicKey(ctx, certURL.String())
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(signed))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return rejected("the signature does not verify")
	}
	return nil
}

// publicKey returns the RSA public key of the certificate at certURL, from the cache or fetched.
func (v *verifier) publicKey(ctx context.Context, certURL string) (*rsa.PublicKey, error) {
	cached, err := v.kv.Get(ctx, certNamespace, certURL)
	if err == nil {
		if key, perr := parseCertificate(cached); perr == nil {
			return key, nil
		}
	} else if !errors.Is(err, kernel.ErrKeyNotFound) {
		v.logger.WarnContext(ctx, "SNS signing certificate cache not read", slog.Any("error", err))
	}
	pemBytes, err := v.fetch(ctx, certURL)
	if err != nil {
		return nil, err
	}
	key, err := parseCertificate(pemBytes)
	if err != nil {
		return nil, rejected("the signing certificate: %v", err)
	}
	if err := v.kv.Set(ctx, certNamespace, certURL, pemBytes, certTTL); err != nil {
		v.logger.WarnContext(ctx, "SNS signing certificate not cached", slog.Any("error", err))
	}
	return key, nil
}

// fetch GETs rawURL through the SSRF-guarded client and returns its body; a status other than
// 2xx is an error.
func (v *verifier) fetch(ctx context.Context, rawURL string) ([]byte, error) {
	resp, err := v.client.Do(ctx, &kernel.HTTPRequest{URL: rawURL, Purpose: kernel.HTTPPurposeJSON})
	if err != nil {
		return nil, fmt.Errorf("mailevents: GET %s: %w", rawURL, err)
	}
	body, err := resp.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("mailevents: read %s: %w", rawURL, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("mailevents: GET %s: status %d", rawURL, resp.StatusCode)
	}
	return body, nil
}

// parseCertificate returns the RSA public key of a PEM-encoded X.509 certificate.
func parseCertificate(pemBytes []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("not a PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("not an RSA key")
	}
	return key, nil
}
