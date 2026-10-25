// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io/fs"
	"log/slog"
	"net/netip"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

var (
	testKEK  = base64.StdEncoding.EncodeToString(make([]byte, kekSize))
	testKEK1 = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, kekSize))
	testKEK2 = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, kekSize))
)

// minimal is the smallest valid environment.
func minimal() map[string]string {
	return map[string]string{
		"MY_HOST":         "social.example.org",
		"MY_DATABASE_URL": "file:my.db",
		"MY_KEK":          testKEK,
	}
}

func with(base map[string]string, kv ...string) map[string]string {
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] == unset {
			delete(base, kv[i])
			continue
		}
		base[kv[i]] = kv[i+1]
	}
	return base
}

const unset = "\x00"

func source(env map[string]string, files map[string]string) Source {
	return Source{
		LookupEnv: func(k string) (string, bool) { v, ok := env[k]; return v, ok },
		ReadFile: func(p string) ([]byte, error) {
			if s, ok := files[p]; ok {
				return []byte(s), nil
			}
			return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
		},
	}
}

func TestLoadValid(t *testing.T) {
	kek := make([]byte, kekSize)
	tests := []struct {
		name  string
		env   map[string]string
		files map[string]string
		want  Config
	}{
		{
			name: "defaults",
			env:  minimal(),
			want: Config{
				Host: "social.example.org", AccountDomain: "social.example.org", SourceURL: DefaultSourceURL, WebOrigin: "https://social.example.org", Role: RoleAll, Listen: ":8080", MaintenanceRetryAfter: 10 * time.Minute,
				DB: "sqlite", DatabaseURL: "file:my.db", Queue: "db", Blob: "fs", Media: "native", KV: "db", ValkeyKeyPrefix: "my:", Stream: "hub",
				BlobFSDir: "blobs", MediaMaxImageBytes: 16 << 20, MediaMaxImageSide: 8192, MediaMaxImagePixels: 7680 * 4320, MediaUnattachedTTL: 24 * time.Hour,
				MediaMaxAVBytes: 100 << 20, MediaMaxVideoDuration: 5 * time.Minute, MediaMaxAudioDuration: 10 * time.Minute, CustomEmojiMaxBytes: 256 << 10, RemoteCustomEmojiMaxBytes: 1 << 20, RemoteMediaCacheMaxBytes: 1 << 30, RemoteStatusRetention: 90 * 24 * time.Hour, RelayStatusRetention: 7 * 24 * time.Hour, RelayIngestHourlyLimit: 1000, BillingAWSMaxRequestsPerDay: 4, BillingAWSMaxRequestsPerMonth: 120, BillingAWSMetric: "NetUnblendedCost", BillingAWSFilter: "none", BillingGCPMethod: "bigquery", BillingGCPMaxBytesBilled: 1 << 30, PaymentsProvider: "none", PaymentsReconcileInterval: 6 * time.Hour, PaymentsReconcileMaxRequests: 1000, DeadJobRetention: 30 * 24 * time.Hour, JobAttemptRetention: 30 * 24 * time.Hour, DynamicClientUnredeemedTTL: 24 * time.Hour, DynamicClientIdleTTL: 90 * 24 * time.Hour,
				KeyManager: "local", KEK: kek, Mail: "none", SMTPPort: 587, Captcha: "none", Profile: "production", TrustedProxyHeader: "x-forwarded-for",
				MetricsExporter: "none", MetricsListen: "127.0.0.1:9464", MetricsEMFNamespace: "my",
				TracesExporter: "none", TracesSampleRatio: 0.01, TracesFlush: "batch",
			},
		},
		{
			name: "file overrides defaults and env overrides file",
			env:  with(minimal(), "MY_CONFIG", "my.toml", "MY_QUEUE", "sqs"),
			files: map[string]string{"my.toml": `
account_domain = "example.org"
source_url = "https://git.example.org/fork/my"
web_origin = "http://localhost:5173"
role = "web"
queue = "db"
db = "postgres"
listen = "127.0.0.1:9000"
`},
			want: Config{
				Host: "social.example.org", AccountDomain: "example.org", SourceURL: "https://git.example.org/fork/my", WebOrigin: "http://localhost:5173", Role: RoleWeb, Listen: "127.0.0.1:9000", MaintenanceRetryAfter: 10 * time.Minute,
				DB: "postgres", DatabaseURL: "file:my.db", Queue: "sqs", Blob: "fs", Media: "native", KV: "db", ValkeyKeyPrefix: "my:", Stream: "hub",
				BlobFSDir: "blobs", MediaMaxImageBytes: 16 << 20, MediaMaxImageSide: 8192, MediaMaxImagePixels: 7680 * 4320, MediaUnattachedTTL: 24 * time.Hour,
				MediaMaxAVBytes: 100 << 20, MediaMaxVideoDuration: 5 * time.Minute, MediaMaxAudioDuration: 10 * time.Minute, CustomEmojiMaxBytes: 256 << 10, RemoteCustomEmojiMaxBytes: 1 << 20, RemoteMediaCacheMaxBytes: 1 << 30, RemoteStatusRetention: 90 * 24 * time.Hour, RelayStatusRetention: 7 * 24 * time.Hour, RelayIngestHourlyLimit: 1000, BillingAWSMaxRequestsPerDay: 4, BillingAWSMaxRequestsPerMonth: 120, BillingAWSMetric: "NetUnblendedCost", BillingAWSFilter: "none", BillingGCPMethod: "bigquery", BillingGCPMaxBytesBilled: 1 << 30, PaymentsProvider: "none", PaymentsReconcileInterval: 6 * time.Hour, PaymentsReconcileMaxRequests: 1000, DeadJobRetention: 30 * 24 * time.Hour, JobAttemptRetention: 30 * 24 * time.Hour, DynamicClientUnredeemedTTL: 24 * time.Hour, DynamicClientIdleTTL: 90 * 24 * time.Hour,
				KeyManager: "local", KEK: kek, Mail: "none", SMTPPort: 587, Captcha: "none", Profile: "production", TrustedProxyHeader: "x-forwarded-for",
				MetricsExporter: "none", MetricsListen: "127.0.0.1:9464", MetricsEMFNamespace: "my",
				TracesExporter: "none", TracesSampleRatio: 0.01, TracesFlush: "batch",
			},
		},
		{
			name:  "secrets from _FILE",
			env:   with(minimal(), "MY_DATABASE_URL", unset, "MY_DATABASE_URL_FILE", "/mnt/db-url", "MY_KEK", unset, "MY_KEK_FILE", "/mnt/kek"),
			files: map[string]string{"/mnt/db-url": "postgres://db/my\n", "/mnt/kek": testKEK + "\n"},
			want: Config{
				Host: "social.example.org", AccountDomain: "social.example.org", SourceURL: DefaultSourceURL, WebOrigin: "https://social.example.org", Role: RoleAll, Listen: ":8080", MaintenanceRetryAfter: 10 * time.Minute,
				DB: "sqlite", DatabaseURL: "postgres://db/my", Queue: "db", Blob: "fs", Media: "native", KV: "db", ValkeyKeyPrefix: "my:", Stream: "hub",
				BlobFSDir: "blobs", MediaMaxImageBytes: 16 << 20, MediaMaxImageSide: 8192, MediaMaxImagePixels: 7680 * 4320, MediaUnattachedTTL: 24 * time.Hour,
				MediaMaxAVBytes: 100 << 20, MediaMaxVideoDuration: 5 * time.Minute, MediaMaxAudioDuration: 10 * time.Minute, CustomEmojiMaxBytes: 256 << 10, RemoteCustomEmojiMaxBytes: 1 << 20, RemoteMediaCacheMaxBytes: 1 << 30, RemoteStatusRetention: 90 * 24 * time.Hour, RelayStatusRetention: 7 * 24 * time.Hour, RelayIngestHourlyLimit: 1000, BillingAWSMaxRequestsPerDay: 4, BillingAWSMaxRequestsPerMonth: 120, BillingAWSMetric: "NetUnblendedCost", BillingAWSFilter: "none", BillingGCPMethod: "bigquery", BillingGCPMaxBytesBilled: 1 << 30, PaymentsProvider: "none", PaymentsReconcileInterval: 6 * time.Hour, PaymentsReconcileMaxRequests: 1000, DeadJobRetention: 30 * 24 * time.Hour, JobAttemptRetention: 30 * 24 * time.Hour, DynamicClientUnredeemedTTL: 24 * time.Hour, DynamicClientIdleTTL: 90 * 24 * time.Hour,
				KeyManager: "local", KEK: kek, Mail: "none", SMTPPort: 587, Captcha: "none", Profile: "production", TrustedProxyHeader: "x-forwarded-for",
				MetricsExporter: "none", MetricsListen: "127.0.0.1:9464", MetricsEMFNamespace: "my",
				TracesExporter: "none", TracesSampleRatio: 0.01, TracesFlush: "batch",
			},
		},
		{
			name: "previous KEKs",
			env:  with(minimal(), "MY_KEK_PREVIOUS", testKEK1+", "+testKEK2),
			want: Config{
				Host: "social.example.org", AccountDomain: "social.example.org", SourceURL: DefaultSourceURL, WebOrigin: "https://social.example.org", Role: RoleAll, Listen: ":8080", MaintenanceRetryAfter: 10 * time.Minute,
				DB: "sqlite", DatabaseURL: "file:my.db", Queue: "db", Blob: "fs", Media: "native", KV: "db", ValkeyKeyPrefix: "my:", Stream: "hub",
				BlobFSDir: "blobs", MediaMaxImageBytes: 16 << 20, MediaMaxImageSide: 8192, MediaMaxImagePixels: 7680 * 4320, MediaUnattachedTTL: 24 * time.Hour,
				MediaMaxAVBytes: 100 << 20, MediaMaxVideoDuration: 5 * time.Minute, MediaMaxAudioDuration: 10 * time.Minute, CustomEmojiMaxBytes: 256 << 10, RemoteCustomEmojiMaxBytes: 1 << 20, RemoteMediaCacheMaxBytes: 1 << 30, RemoteStatusRetention: 90 * 24 * time.Hour, RelayStatusRetention: 7 * 24 * time.Hour, RelayIngestHourlyLimit: 1000, BillingAWSMaxRequestsPerDay: 4, BillingAWSMaxRequestsPerMonth: 120, BillingAWSMetric: "NetUnblendedCost", BillingAWSFilter: "none", BillingGCPMethod: "bigquery", BillingGCPMaxBytesBilled: 1 << 30, PaymentsProvider: "none", PaymentsReconcileInterval: 6 * time.Hour, PaymentsReconcileMaxRequests: 1000, DeadJobRetention: 30 * 24 * time.Hour, JobAttemptRetention: 30 * 24 * time.Hour, DynamicClientUnredeemedTTL: 24 * time.Hour, DynamicClientIdleTTL: 90 * 24 * time.Hour,
				KeyManager: "local", KEK: kek, Mail: "none", SMTPPort: 587, Captcha: "none", Profile: "production", TrustedProxyHeader: "x-forwarded-for",
				MetricsExporter: "none", MetricsListen: "127.0.0.1:9464", MetricsEMFNamespace: "my",
				TracesExporter: "none", TracesSampleRatio: 0.01, TracesFlush: "batch",
				PreviousKEKs: [][]byte{bytes.Repeat([]byte{1}, kekSize), bytes.Repeat([]byte{2}, kekSize)},
			},
		},
		{
			name: "smtp mailer",
			env: with(minimal(), "MY_MAIL", "smtp", "MY_MAIL_FROM", "noreply@social.example.org", "MY_SMTP_HOST", "smtp.example.org",
				"MY_SMTP_PORT", "2525", "MY_SMTP_USERNAME", "mailer", "MY_SMTP_PASSWORD", "hunter3"),
			want: Config{
				Host: "social.example.org", AccountDomain: "social.example.org", SourceURL: DefaultSourceURL, WebOrigin: "https://social.example.org", Role: RoleAll, Listen: ":8080", MaintenanceRetryAfter: 10 * time.Minute,
				DB: "sqlite", DatabaseURL: "file:my.db", Queue: "db", Blob: "fs", Media: "native", KV: "db", ValkeyKeyPrefix: "my:", Stream: "hub",
				BlobFSDir: "blobs", MediaMaxImageBytes: 16 << 20, MediaMaxImageSide: 8192, MediaMaxImagePixels: 7680 * 4320, MediaUnattachedTTL: 24 * time.Hour,
				MediaMaxAVBytes: 100 << 20, MediaMaxVideoDuration: 5 * time.Minute, MediaMaxAudioDuration: 10 * time.Minute, CustomEmojiMaxBytes: 256 << 10, RemoteCustomEmojiMaxBytes: 1 << 20, RemoteMediaCacheMaxBytes: 1 << 30, RemoteStatusRetention: 90 * 24 * time.Hour, RelayStatusRetention: 7 * 24 * time.Hour, RelayIngestHourlyLimit: 1000, BillingAWSMaxRequestsPerDay: 4, BillingAWSMaxRequestsPerMonth: 120, BillingAWSMetric: "NetUnblendedCost", BillingAWSFilter: "none", BillingGCPMethod: "bigquery", BillingGCPMaxBytesBilled: 1 << 30, PaymentsProvider: "none", PaymentsReconcileInterval: 6 * time.Hour, PaymentsReconcileMaxRequests: 1000, DeadJobRetention: 30 * 24 * time.Hour, JobAttemptRetention: 30 * 24 * time.Hour, DynamicClientUnredeemedTTL: 24 * time.Hour, DynamicClientIdleTTL: 90 * 24 * time.Hour,
				KeyManager: "local", KEK: kek, Mail: "smtp", MailFrom: "noreply@social.example.org", SMTPHost: "smtp.example.org",
				SMTPPort: 2525, SMTPUsername: "mailer", SMTPPassword: "hunter3", Captcha: "none", Profile: "production", TrustedProxyHeader: "x-forwarded-for",
				MetricsExporter: "none", MetricsListen: "127.0.0.1:9464", MetricsEMFNamespace: "my",
				TracesExporter: "none", TracesSampleRatio: 0.01, TracesFlush: "batch",
			},
		},
		{
			name:  "captcha with the secret key from _FILE",
			env:   with(minimal(), "MY_CAPTCHA", "turnstile", "MY_CAPTCHA_SITE_KEY", "0x4AAA-site", "MY_CAPTCHA_SECRET_KEY_FILE", "/mnt/captcha"),
			files: map[string]string{"/mnt/captcha": "0x4AAA-secret\n"},
			want: Config{
				Host: "social.example.org", AccountDomain: "social.example.org", SourceURL: DefaultSourceURL, WebOrigin: "https://social.example.org", Role: RoleAll, Listen: ":8080", MaintenanceRetryAfter: 10 * time.Minute,
				DB: "sqlite", DatabaseURL: "file:my.db", Queue: "db", Blob: "fs", Media: "native", KV: "db", ValkeyKeyPrefix: "my:", Stream: "hub",
				BlobFSDir: "blobs", MediaMaxImageBytes: 16 << 20, MediaMaxImageSide: 8192, MediaMaxImagePixels: 7680 * 4320, MediaUnattachedTTL: 24 * time.Hour,
				MediaMaxAVBytes: 100 << 20, MediaMaxVideoDuration: 5 * time.Minute, MediaMaxAudioDuration: 10 * time.Minute, CustomEmojiMaxBytes: 256 << 10, RemoteCustomEmojiMaxBytes: 1 << 20, RemoteMediaCacheMaxBytes: 1 << 30, RemoteStatusRetention: 90 * 24 * time.Hour, RelayStatusRetention: 7 * 24 * time.Hour, RelayIngestHourlyLimit: 1000, BillingAWSMaxRequestsPerDay: 4, BillingAWSMaxRequestsPerMonth: 120, BillingAWSMetric: "NetUnblendedCost", BillingAWSFilter: "none", BillingGCPMethod: "bigquery", BillingGCPMaxBytesBilled: 1 << 30, PaymentsProvider: "none", PaymentsReconcileInterval: 6 * time.Hour, PaymentsReconcileMaxRequests: 1000, DeadJobRetention: 30 * 24 * time.Hour, JobAttemptRetention: 30 * 24 * time.Hour, DynamicClientUnredeemedTTL: 24 * time.Hour, DynamicClientIdleTTL: 90 * 24 * time.Hour,
				KeyManager: "local", KEK: kek, Mail: "none", SMTPPort: 587,
				Captcha: "turnstile", CaptchaSiteKey: "0x4AAA-site", CaptchaSecretKey: "0x4AAA-secret", Profile: "production", TrustedProxyHeader: "x-forwarded-for",
				MetricsExporter: "none", MetricsListen: "127.0.0.1:9464", MetricsEMFNamespace: "my",
				TracesExporter: "none", TracesSampleRatio: 0.01, TracesFlush: "batch",
			},
		},
		{
			name: "host with port",
			env:  with(minimal(), "MY_HOST", "localhost:3000"),
			want: Config{
				Host: "localhost:3000", AccountDomain: "localhost:3000", SourceURL: DefaultSourceURL, WebOrigin: "https://localhost:3000", Role: RoleAll, Listen: ":8080", MaintenanceRetryAfter: 10 * time.Minute,
				DB: "sqlite", DatabaseURL: "file:my.db", Queue: "db", Blob: "fs", Media: "native", KV: "db", ValkeyKeyPrefix: "my:", Stream: "hub",
				BlobFSDir: "blobs", MediaMaxImageBytes: 16 << 20, MediaMaxImageSide: 8192, MediaMaxImagePixels: 7680 * 4320, MediaUnattachedTTL: 24 * time.Hour,
				MediaMaxAVBytes: 100 << 20, MediaMaxVideoDuration: 5 * time.Minute, MediaMaxAudioDuration: 10 * time.Minute, CustomEmojiMaxBytes: 256 << 10, RemoteCustomEmojiMaxBytes: 1 << 20, RemoteMediaCacheMaxBytes: 1 << 30, RemoteStatusRetention: 90 * 24 * time.Hour, RelayStatusRetention: 7 * 24 * time.Hour, RelayIngestHourlyLimit: 1000, BillingAWSMaxRequestsPerDay: 4, BillingAWSMaxRequestsPerMonth: 120, BillingAWSMetric: "NetUnblendedCost", BillingAWSFilter: "none", BillingGCPMethod: "bigquery", BillingGCPMaxBytesBilled: 1 << 30, PaymentsProvider: "none", PaymentsReconcileInterval: 6 * time.Hour, PaymentsReconcileMaxRequests: 1000, DeadJobRetention: 30 * 24 * time.Hour, JobAttemptRetention: 30 * 24 * time.Hour, DynamicClientUnredeemedTTL: 24 * time.Hour, DynamicClientIdleTTL: 90 * 24 * time.Hour,
				KeyManager: "local", KEK: kek, Mail: "none", SMTPPort: 587, Captcha: "none", Profile: "production", TrustedProxyHeader: "x-forwarded-for",
				MetricsExporter: "none", MetricsListen: "127.0.0.1:9464", MetricsEMFNamespace: "my",
				TracesExporter: "none", TracesSampleRatio: 0.01, TracesFlush: "batch",
			},
		},
		{
			name: "trusted proxies",
			env: with(minimal(), "MY_TRUSTED_PROXIES", "10.1.2.3/8, 2001:db8::/32,192.0.2.10, ::ffff:198.51.100.0/120, 2001:db8::1",
				"MY_TRUSTED_PROXY_HEADER", "forwarded"),
			want: Config{
				Host: "social.example.org", AccountDomain: "social.example.org", SourceURL: DefaultSourceURL, WebOrigin: "https://social.example.org", Role: RoleAll, Listen: ":8080", MaintenanceRetryAfter: 10 * time.Minute,
				DB: "sqlite", DatabaseURL: "file:my.db", Queue: "db", Blob: "fs", Media: "native", KV: "db", ValkeyKeyPrefix: "my:", Stream: "hub",
				BlobFSDir: "blobs", MediaMaxImageBytes: 16 << 20, MediaMaxImageSide: 8192, MediaMaxImagePixels: 7680 * 4320, MediaUnattachedTTL: 24 * time.Hour,
				MediaMaxAVBytes: 100 << 20, MediaMaxVideoDuration: 5 * time.Minute, MediaMaxAudioDuration: 10 * time.Minute, CustomEmojiMaxBytes: 256 << 10, RemoteCustomEmojiMaxBytes: 1 << 20, RemoteMediaCacheMaxBytes: 1 << 30, RemoteStatusRetention: 90 * 24 * time.Hour, RelayStatusRetention: 7 * 24 * time.Hour, RelayIngestHourlyLimit: 1000, BillingAWSMaxRequestsPerDay: 4, BillingAWSMaxRequestsPerMonth: 120, BillingAWSMetric: "NetUnblendedCost", BillingAWSFilter: "none", BillingGCPMethod: "bigquery", BillingGCPMaxBytesBilled: 1 << 30, PaymentsProvider: "none", PaymentsReconcileInterval: 6 * time.Hour, PaymentsReconcileMaxRequests: 1000, DeadJobRetention: 30 * 24 * time.Hour, JobAttemptRetention: 30 * 24 * time.Hour, DynamicClientUnredeemedTTL: 24 * time.Hour, DynamicClientIdleTTL: 90 * 24 * time.Hour,
				KeyManager: "local", KEK: kek, Mail: "none", SMTPPort: 587, Captcha: "none", Profile: "production",
				TrustedProxies: []netip.Prefix{
					netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("192.0.2.10/32"),
					netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("2001:db8::1/128"),
				},
				TrustedProxyHeader: "forwarded",
				MetricsExporter:    "none", MetricsListen: "127.0.0.1:9464", MetricsEMFNamespace: "my",
				TracesExporter: "none", TracesSampleRatio: 0.01, TracesFlush: "batch",
			},
		},
		{
			name: "webhook destinations",
			env:  with(minimal(), "MY_WEBHOOK_ALLOWED_DESTINATIONS", "n8n.internal, 10.0.0.0/8,192.0.2.10, chat.corp.example"),
			want: Config{
				Host: "social.example.org", AccountDomain: "social.example.org", SourceURL: DefaultSourceURL, WebOrigin: "https://social.example.org", Role: RoleAll, Listen: ":8080", MaintenanceRetryAfter: 10 * time.Minute,
				DB: "sqlite", DatabaseURL: "file:my.db", Queue: "db", Blob: "fs", Media: "native", KV: "db", ValkeyKeyPrefix: "my:", Stream: "hub",
				BlobFSDir: "blobs", MediaMaxImageBytes: 16 << 20, MediaMaxImageSide: 8192, MediaMaxImagePixels: 7680 * 4320, MediaUnattachedTTL: 24 * time.Hour,
				MediaMaxAVBytes: 100 << 20, MediaMaxVideoDuration: 5 * time.Minute, MediaMaxAudioDuration: 10 * time.Minute, CustomEmojiMaxBytes: 256 << 10, RemoteCustomEmojiMaxBytes: 1 << 20, RemoteMediaCacheMaxBytes: 1 << 30, RemoteStatusRetention: 90 * 24 * time.Hour, RelayStatusRetention: 7 * 24 * time.Hour, RelayIngestHourlyLimit: 1000, BillingAWSMaxRequestsPerDay: 4, BillingAWSMaxRequestsPerMonth: 120, BillingAWSMetric: "NetUnblendedCost", BillingAWSFilter: "none", BillingGCPMethod: "bigquery", BillingGCPMaxBytesBilled: 1 << 30, PaymentsProvider: "none", PaymentsReconcileInterval: 6 * time.Hour, PaymentsReconcileMaxRequests: 1000, DeadJobRetention: 30 * 24 * time.Hour, JobAttemptRetention: 30 * 24 * time.Hour, DynamicClientUnredeemedTTL: 24 * time.Hour, DynamicClientIdleTTL: 90 * 24 * time.Hour,
				KeyManager: "local", KEK: kek, Mail: "none", SMTPPort: 587, Captcha: "none", Profile: "production", TrustedProxyHeader: "x-forwarded-for",
				WebhookAllowedHosts:    []string{"n8n.internal", "chat.corp.example"},
				WebhookAllowedPrefixes: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("192.0.2.10/32")},
				MetricsExporter:        "none", MetricsListen: "127.0.0.1:9464", MetricsEMFNamespace: "my",
				TracesExporter: "none", TracesSampleRatio: 0.01, TracesFlush: "batch",
			},
		},
		{
			name: "billing imports",
			env: with(minimal(), "MY_BILLING_SOURCES", "gcp, aws", "MY_BILLING_AWS_MAX_REQUESTS_PER_DAY", "2",
				"MY_BILLING_AWS_MAX_REQUESTS_PER_MONTH", "40",
				"MY_BILLING_GCP_TABLE", " my-billing.billing_export.gcp_billing_export_v1_012345_6789AB_CDEF01 ",
				"MY_BILLING_GCP_QUERY_PROJECT", "my-queries", "MY_BILLING_GCP_PROJECTS", "my-social, my-media",
				"MY_BILLING_GCP_LABEL", "app=my", "MY_BILLING_GCP_MAX_BYTES_BILLED", "5368709120"),
			want: Config{
				Host: "social.example.org", AccountDomain: "social.example.org", SourceURL: DefaultSourceURL, WebOrigin: "https://social.example.org", Role: RoleAll, Listen: ":8080", MaintenanceRetryAfter: 10 * time.Minute,
				DB: "sqlite", DatabaseURL: "file:my.db", Queue: "db", Blob: "fs", Media: "native", KV: "db", ValkeyKeyPrefix: "my:", Stream: "hub",
				BlobFSDir: "blobs", MediaMaxImageBytes: 16 << 20, MediaMaxImageSide: 8192, MediaMaxImagePixels: 7680 * 4320, MediaUnattachedTTL: 24 * time.Hour,
				MediaMaxAVBytes: 100 << 20, MediaMaxVideoDuration: 5 * time.Minute, MediaMaxAudioDuration: 10 * time.Minute, CustomEmojiMaxBytes: 256 << 10, RemoteCustomEmojiMaxBytes: 1 << 20, RemoteMediaCacheMaxBytes: 1 << 30, RemoteStatusRetention: 90 * 24 * time.Hour, RelayStatusRetention: 7 * 24 * time.Hour, RelayIngestHourlyLimit: 1000, PaymentsProvider: "none", PaymentsReconcileInterval: 6 * time.Hour, PaymentsReconcileMaxRequests: 1000, DeadJobRetention: 30 * 24 * time.Hour, JobAttemptRetention: 30 * 24 * time.Hour, DynamicClientUnredeemedTTL: 24 * time.Hour, DynamicClientIdleTTL: 90 * 24 * time.Hour,
				BillingSources: []string{"gcp", "aws"}, BillingAWSMaxRequestsPerDay: 2, BillingAWSMaxRequestsPerMonth: 40, BillingAWSMetric: "NetUnblendedCost", BillingAWSFilter: "none",
				BillingGCPMethod: "bigquery", BillingGCPTable: "my-billing.billing_export.gcp_billing_export_v1_012345_6789AB_CDEF01",
				BillingGCPQueryProject: "my-queries", BillingGCPProjects: []string{"my-social", "my-media"}, BillingGCPLabel: "app=my",
				BillingGCPMaxBytesBilled: 5 << 30,
				KeyManager:               "local", KEK: kek, Mail: "none", SMTPPort: 587, Captcha: "none", Profile: "production", TrustedProxyHeader: "x-forwarded-for",
				MetricsExporter: "none", MetricsListen: "127.0.0.1:9464", MetricsEMFNamespace: "my",
				TracesExporter: "none", TracesSampleRatio: 0.01, TracesFlush: "batch",
			},
		},
		{
			name: "AWS billing import of linked accounts through a role",
			env: with(minimal(), "MY_BILLING_SOURCES", "aws", "MY_BILLING_AWS_METRIC", "UnblendedCost",
				"MY_BILLING_AWS_FILTER", "linked_account", "MY_BILLING_AWS_LINKED_ACCOUNTS", "111122223333, 444455556666",
				"MY_BILLING_AWS_TAG", "app = my", "MY_BILLING_AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/my-billing"),
			want: Config{
				Host: "social.example.org", AccountDomain: "social.example.org", SourceURL: DefaultSourceURL, WebOrigin: "https://social.example.org", Role: RoleAll, Listen: ":8080", MaintenanceRetryAfter: 10 * time.Minute,
				DB: "sqlite", DatabaseURL: "file:my.db", Queue: "db", Blob: "fs", Media: "native", KV: "db", ValkeyKeyPrefix: "my:", Stream: "hub",
				BlobFSDir: "blobs", MediaMaxImageBytes: 16 << 20, MediaMaxImageSide: 8192, MediaMaxImagePixels: 7680 * 4320, MediaUnattachedTTL: 24 * time.Hour,
				MediaMaxAVBytes: 100 << 20, MediaMaxVideoDuration: 5 * time.Minute, MediaMaxAudioDuration: 10 * time.Minute, CustomEmojiMaxBytes: 256 << 10, RemoteCustomEmojiMaxBytes: 1 << 20, RemoteMediaCacheMaxBytes: 1 << 30, RemoteStatusRetention: 90 * 24 * time.Hour, RelayStatusRetention: 7 * 24 * time.Hour, RelayIngestHourlyLimit: 1000, PaymentsProvider: "none", PaymentsReconcileInterval: 6 * time.Hour, PaymentsReconcileMaxRequests: 1000, DeadJobRetention: 30 * 24 * time.Hour, JobAttemptRetention: 30 * 24 * time.Hour, DynamicClientUnredeemedTTL: 24 * time.Hour, DynamicClientIdleTTL: 90 * 24 * time.Hour,
				BillingSources: []string{"aws"}, BillingAWSMaxRequestsPerDay: 4, BillingAWSMaxRequestsPerMonth: 120, BillingAWSMetric: "UnblendedCost",
				BillingAWSFilter: "linked_account", BillingGCPMethod: "bigquery", BillingGCPMaxBytesBilled: 1 << 30, BillingAWSLinkedAccounts: []string{"111122223333", "444455556666"}, BillingAWSTag: "app = my",
				BillingAWSRoleARN: "arn:aws:iam::123456789012:role/my-billing",
				KeyManager:        "local", KEK: kek, Mail: "none", SMTPPort: 587, Captcha: "none", Profile: "production", TrustedProxyHeader: "x-forwarded-for",
				MetricsExporter: "none", MetricsListen: "127.0.0.1:9464", MetricsEMFNamespace: "my",
				TracesExporter: "none", TracesSampleRatio: 0.01, TracesFlush: "batch",
			},
		},
		{
			name: "Google Cloud billing import of a budget",
			env: with(minimal(), "MY_BILLING_SOURCES", "gcp", "MY_BILLING_GCP_METHOD", "budget",
				"MY_BILLING_GCP_BUDGET_SUBSCRIPTION", " projects/my-billing/subscriptions/my-billing-budget "),
			want: Config{
				Host: "social.example.org", AccountDomain: "social.example.org", SourceURL: DefaultSourceURL, WebOrigin: "https://social.example.org", Role: RoleAll, Listen: ":8080", MaintenanceRetryAfter: 10 * time.Minute,
				DB: "sqlite", DatabaseURL: "file:my.db", Queue: "db", Blob: "fs", Media: "native", KV: "db", ValkeyKeyPrefix: "my:", Stream: "hub",
				BlobFSDir: "blobs", MediaMaxImageBytes: 16 << 20, MediaMaxImageSide: 8192, MediaMaxImagePixels: 7680 * 4320, MediaUnattachedTTL: 24 * time.Hour,
				MediaMaxAVBytes: 100 << 20, MediaMaxVideoDuration: 5 * time.Minute, MediaMaxAudioDuration: 10 * time.Minute, CustomEmojiMaxBytes: 256 << 10, RemoteCustomEmojiMaxBytes: 1 << 20, RemoteMediaCacheMaxBytes: 1 << 30, RemoteStatusRetention: 90 * 24 * time.Hour, RelayStatusRetention: 7 * 24 * time.Hour, RelayIngestHourlyLimit: 1000, PaymentsProvider: "none", PaymentsReconcileInterval: 6 * time.Hour, PaymentsReconcileMaxRequests: 1000, DeadJobRetention: 30 * 24 * time.Hour, JobAttemptRetention: 30 * 24 * time.Hour, DynamicClientUnredeemedTTL: 24 * time.Hour, DynamicClientIdleTTL: 90 * 24 * time.Hour,
				BillingSources: []string{"gcp"}, BillingAWSMaxRequestsPerDay: 4, BillingAWSMaxRequestsPerMonth: 120, BillingAWSMetric: "NetUnblendedCost", BillingAWSFilter: "none",
				BillingGCPMethod: "budget", BillingGCPMaxBytesBilled: 1 << 30, BillingGCPBudgetSubscription: "projects/my-billing/subscriptions/my-billing-budget",
				KeyManager: "local", KEK: kek, Mail: "none", SMTPPort: 587, Captcha: "none", Profile: "production", TrustedProxyHeader: "x-forwarded-for",
				MetricsExporter: "none", MetricsListen: "127.0.0.1:9464", MetricsEMFNamespace: "my",
				TracesExporter: "none", TracesSampleRatio: 0.01, TracesFlush: "batch",
			},
		},
		{
			name: "stream on SNS",
			env: with(minimal(), "MY_ROLE", "web", "MY_STREAM", "sns",
				"MY_STREAM_SNS_TOPIC_ARN", "arn:aws:sns:us-east-1:123456789012:my-stream", "MY_STREAM_SQS_QUEUE_PREFIX", "my-stream"),
			want: Config{
				Host: "social.example.org", AccountDomain: "social.example.org", SourceURL: DefaultSourceURL, WebOrigin: "https://social.example.org", Role: RoleWeb, Listen: ":8080", MaintenanceRetryAfter: 10 * time.Minute,
				DB: "sqlite", DatabaseURL: "file:my.db", Queue: "db", Blob: "fs", Media: "native", KV: "db", ValkeyKeyPrefix: "my:",
				Stream: "sns", StreamSNSTopicARN: "arn:aws:sns:us-east-1:123456789012:my-stream", StreamSQSQueuePrefix: "my-stream",
				BlobFSDir: "blobs", MediaMaxImageBytes: 16 << 20, MediaMaxImageSide: 8192, MediaMaxImagePixels: 7680 * 4320, MediaUnattachedTTL: 24 * time.Hour,
				MediaMaxAVBytes: 100 << 20, MediaMaxVideoDuration: 5 * time.Minute, MediaMaxAudioDuration: 10 * time.Minute, CustomEmojiMaxBytes: 256 << 10, RemoteCustomEmojiMaxBytes: 1 << 20, RemoteMediaCacheMaxBytes: 1 << 30, RemoteStatusRetention: 90 * 24 * time.Hour, RelayStatusRetention: 7 * 24 * time.Hour, RelayIngestHourlyLimit: 1000, BillingAWSMaxRequestsPerDay: 4, BillingAWSMaxRequestsPerMonth: 120, BillingAWSMetric: "NetUnblendedCost", BillingAWSFilter: "none", BillingGCPMethod: "bigquery", BillingGCPMaxBytesBilled: 1 << 30, PaymentsProvider: "none", PaymentsReconcileInterval: 6 * time.Hour, PaymentsReconcileMaxRequests: 1000, DeadJobRetention: 30 * 24 * time.Hour, JobAttemptRetention: 30 * 24 * time.Hour, DynamicClientUnredeemedTTL: 24 * time.Hour, DynamicClientIdleTTL: 90 * 24 * time.Hour,
				KeyManager: "local", KEK: kek, Mail: "none", SMTPPort: 587, Captcha: "none", Profile: "production", TrustedProxyHeader: "x-forwarded-for",
				MetricsExporter: "none", MetricsListen: "127.0.0.1:9464", MetricsEMFNamespace: "my",
				TracesExporter: "none", TracesSampleRatio: 0.01, TracesFlush: "batch",
			},
		},
		{
			name: "stream on Pub/Sub",
			env: with(minimal(), "MY_ROLE", "web", "MY_STREAM", "pubsub",
				"MY_STREAM_PUBSUB_TOPIC", "projects/my-project/topics/my-stream", "MY_STREAM_PUBSUB_SUBSCRIPTION_PREFIX", "my-stream"),
			want: Config{
				Host: "social.example.org", AccountDomain: "social.example.org", SourceURL: DefaultSourceURL, WebOrigin: "https://social.example.org", Role: RoleWeb, Listen: ":8080", MaintenanceRetryAfter: 10 * time.Minute,
				DB: "sqlite", DatabaseURL: "file:my.db", Queue: "db", Blob: "fs", Media: "native", KV: "db", ValkeyKeyPrefix: "my:",
				Stream: "pubsub", StreamPubSubTopic: "projects/my-project/topics/my-stream", StreamPubSubSubscriptionPrefix: "my-stream",
				BlobFSDir: "blobs", MediaMaxImageBytes: 16 << 20, MediaMaxImageSide: 8192, MediaMaxImagePixels: 7680 * 4320, MediaUnattachedTTL: 24 * time.Hour,
				MediaMaxAVBytes: 100 << 20, MediaMaxVideoDuration: 5 * time.Minute, MediaMaxAudioDuration: 10 * time.Minute, CustomEmojiMaxBytes: 256 << 10, RemoteCustomEmojiMaxBytes: 1 << 20, RemoteMediaCacheMaxBytes: 1 << 30, RemoteStatusRetention: 90 * 24 * time.Hour, RelayStatusRetention: 7 * 24 * time.Hour, RelayIngestHourlyLimit: 1000, BillingAWSMaxRequestsPerDay: 4, BillingAWSMaxRequestsPerMonth: 120, BillingAWSMetric: "NetUnblendedCost", BillingAWSFilter: "none", BillingGCPMethod: "bigquery", BillingGCPMaxBytesBilled: 1 << 30, PaymentsProvider: "none", PaymentsReconcileInterval: 6 * time.Hour, PaymentsReconcileMaxRequests: 1000, DeadJobRetention: 30 * 24 * time.Hour, JobAttemptRetention: 30 * 24 * time.Hour, DynamicClientUnredeemedTTL: 24 * time.Hour, DynamicClientIdleTTL: 90 * 24 * time.Hour,
				KeyManager: "local", KEK: kek, Mail: "none", SMTPPort: 587, Captcha: "none", Profile: "production", TrustedProxyHeader: "x-forwarded-for",
				MetricsExporter: "none", MetricsListen: "127.0.0.1:9464", MetricsEMFNamespace: "my",
				TracesExporter: "none", TracesSampleRatio: 0.01, TracesFlush: "batch",
			},
		},
		{
			name: "stream on PostgreSQL with a direct connection",
			env: with(minimal(), "MY_DB", "postgres", "MY_DATABASE_URL", "postgres://pooler:6543/my", "MY_STREAM", "postgres",
				"MY_STREAM_POSTGRES_URL", "postgres://db:5432/my"),
			want: Config{
				Host: "social.example.org", AccountDomain: "social.example.org", SourceURL: DefaultSourceURL, WebOrigin: "https://social.example.org", Role: RoleAll, Listen: ":8080", MaintenanceRetryAfter: 10 * time.Minute,
				DB: "postgres", DatabaseURL: "postgres://pooler:6543/my", Queue: "db", Blob: "fs", Media: "native", KV: "db", ValkeyKeyPrefix: "my:",
				Stream: "postgres", StreamPostgresURL: "postgres://db:5432/my",
				BlobFSDir: "blobs", MediaMaxImageBytes: 16 << 20, MediaMaxImageSide: 8192, MediaMaxImagePixels: 7680 * 4320, MediaUnattachedTTL: 24 * time.Hour,
				MediaMaxAVBytes: 100 << 20, MediaMaxVideoDuration: 5 * time.Minute, MediaMaxAudioDuration: 10 * time.Minute, CustomEmojiMaxBytes: 256 << 10, RemoteCustomEmojiMaxBytes: 1 << 20, RemoteMediaCacheMaxBytes: 1 << 30, RemoteStatusRetention: 90 * 24 * time.Hour, RelayStatusRetention: 7 * 24 * time.Hour, RelayIngestHourlyLimit: 1000, BillingAWSMaxRequestsPerDay: 4, BillingAWSMaxRequestsPerMonth: 120, BillingAWSMetric: "NetUnblendedCost", BillingAWSFilter: "none", BillingGCPMethod: "bigquery", BillingGCPMaxBytesBilled: 1 << 30, PaymentsProvider: "none", PaymentsReconcileInterval: 6 * time.Hour, PaymentsReconcileMaxRequests: 1000, DeadJobRetention: 30 * 24 * time.Hour, JobAttemptRetention: 30 * 24 * time.Hour, DynamicClientUnredeemedTTL: 24 * time.Hour, DynamicClientIdleTTL: 90 * 24 * time.Hour,
				KeyManager: "local", KEK: kek, Mail: "none", SMTPPort: 587, Captcha: "none", Profile: "production", TrustedProxyHeader: "x-forwarded-for",
				MetricsExporter: "none", MetricsListen: "127.0.0.1:9464", MetricsEMFNamespace: "my",
				TracesExporter: "none", TracesSampleRatio: 0.01, TracesFlush: "batch",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := Load(source(tt.env, tt.files))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("Load() =\n%+v\nwant\n%+v", *got, tt.want)
			}
		})
	}
}

func TestLoadInvalid(t *testing.T) {
	tests := []struct {
		name  string
		env   map[string]string
		files map[string]string
		file  string
		want  []string // every issue, in order
	}{
		{
			name: "all required missing",
			env:  map[string]string{},
			want: []string{
				"host (MY_HOST): required",
				"database_url (MY_DATABASE_URL): required",
				"kek (MY_KEK): required when key_manager is local",
			},
		},
		{
			name: "invalid values are all listed",
			env: with(minimal(),
				"MY_HOST", "https://Social.example.org/",
				"MY_ACCOUNT_DOMAIN", "10.0.0.1",
				"MY_SOURCE_URL", "http://example.org/my",
				"MY_WEB_ORIGIN", "http://localhost:5173/",
				"MY_ROLE", "both",
				"MY_LISTEN", "8080",
				"MY_DB", "mysql",
				"MY_KEK", "c2hvcnQ=",
			),
			want: []string{
				"host (MY_HOST): must be a domain name with an optional :port, without scheme or path",
				"account_domain (MY_ACCOUNT_DOMAIN): must be a domain name, not an IP address",
				"source_url (MY_SOURCE_URL): must be an absolute https URL without user info or fragment, or a mailto: URI with an email address",
				"web_origin (MY_WEB_ORIGIN): must be an origin: http or https, a host, and an optional port, without path or trailing slash",
				`role (MY_ROLE): unknown value "both"; want one of all, web, worker`,
				"listen (MY_LISTEN): must be host:port, for example :8080 or 127.0.0.1:8080",
				`db (MY_DB): unknown value "mysql"; want one of sqlite, libsql, postgres`,
				"kek (MY_KEK): must decode to 32 bytes, got 5",
			},
		},
		{
			name: "upper case host and bad port",
			env:  with(minimal(), "MY_HOST", "Social.example.org", "MY_ACCOUNT_DOMAIN", "example.org:99999"),
			want: []string{
				"host (MY_HOST): must be lower case",
				`account_domain (MY_ACCOUNT_DOMAIN): port "99999" must be a number from 1 to 65535`,
			},
		},
		{
			name: "adapters outside v0",
			env:  with(minimal(), "MY_BLOB", "gcs", "MY_KEY_MANAGER", "awskms", "MY_KEK", unset),
			want: []string{
				`blob (MY_BLOB): "gcs" is not available in v0; want one of fs, s3`,
				`key_manager (MY_KEY_MANAGER): "awskms" is not available in v0; want one of local`,
			},
		},
		{
			name: "kv valkey without a URL",
			env:  with(minimal(), "MY_KV", "valkey"),
			want: []string{"valkey_url (MY_VALKEY_URL): required when kv is valkey"},
		},
		{
			name: "stream valkey without a URL",
			env:  with(minimal(), "MY_STREAM", "valkey"),
			want: []string{"valkey_url (MY_VALKEY_URL): required when stream is valkey"},
		},
		{
			name: "stream postgres without db postgres",
			env:  with(minimal(), "MY_STREAM", "postgres"),
			want: []string{"stream (MY_STREAM): postgres requires db = postgres, not sqlite"},
		},
		{
			name: "stream sns without its topic and queues",
			env:  with(minimal(), "MY_STREAM", "sns"),
			want: []string{
				"stream_sns_topic_arn (MY_STREAM_SNS_TOPIC_ARN): required when stream is sns",
				"stream_sqs_queue_prefix (MY_STREAM_SQS_QUEUE_PREFIX): required when stream is sns",
			},
		},
		{
			name: "malformed stream sns settings",
			env: with(minimal(), "MY_STREAM", "sns",
				"MY_STREAM_SNS_TOPIC_ARN", "arn:aws:sqs:us-east-1:123456789012:my-stream", "MY_STREAM_SQS_QUEUE_PREFIX", "my.stream"),
			want: []string{
				`stream_sns_topic_arn (MY_STREAM_SNS_TOPIC_ARN): not the ARN of an SNS topic`,
				`stream_sqs_queue_prefix (MY_STREAM_SQS_QUEUE_PREFIX): not 1 to 40 letters`,
			},
		},
		{
			name: "stream pubsub without its topic and subscriptions",
			env:  with(minimal(), "MY_STREAM", "pubsub"),
			want: []string{
				"stream_pubsub_topic (MY_STREAM_PUBSUB_TOPIC): required when stream is pubsub",
				"stream_pubsub_subscription_prefix (MY_STREAM_PUBSUB_SUBSCRIPTION_PREFIX): required when stream is pubsub",
			},
		},
		{
			name: "malformed stream pubsub settings",
			env: with(minimal(), "MY_STREAM", "pubsub",
				"MY_STREAM_PUBSUB_TOPIC", "my-stream", "MY_STREAM_PUBSUB_SUBSCRIPTION_PREFIX", "goog-stream"),
			want: []string{
				"stream_pubsub_topic (MY_STREAM_PUBSUB_TOPIC): not a Pub/Sub topic name",
				"stream_pubsub_subscription_prefix (MY_STREAM_PUBSUB_SUBSCRIPTION_PREFIX): not the start of a Pub/Sub subscription ID",
			},
		},
		{
			name: "unknown stream broker",
			env:  with(minimal(), "MY_STREAM", "kafka"),
			want: []string{`stream (MY_STREAM): unknown value "kafka"`},
		},
		{
			name: "valkey URLs",
			env:  with(minimal(), "MY_KV", "valkey", "MY_VALKEY_URL", "http://cache:6379"),
			want: []string{"valkey_url (MY_VALKEY_URL): valkey: invalid URL scheme: http"},
		},
		{
			name: "valkey URL that skips TLS verification",
			env:  with(minimal(), "MY_KV", "valkey", "MY_VALKEY_URL", "valkeys://cache:6379?skip_verify=true"),
			want: []string{"valkey_url (MY_VALKEY_URL): valkey: skip_verify is refused"},
		},
		{
			name: "valkey CA file without TLS",
			env:  with(minimal(), "MY_KV", "valkey", "MY_VALKEY_URL", "valkey://cache:6379/2", "MY_VALKEY_TLS_CA_FILE", "/etc/ca.pem"),
			want: []string{"valkey_tls_ca_file (MY_VALKEY_TLS_CA_FILE): needs a valkeys:// or rediss:// valkey_url"},
		},
		{
			name: "media limits",
			env:  with(minimal(), "MY_MEDIA_MAX_IMAGE_BYTES", "0", "MY_MEDIA_MAX_IMAGE_SIDE", "big", "MY_MEDIA_UNATTACHED_TTL", "1 day"),
			want: []string{
				"media_max_image_bytes (MY_MEDIA_MAX_IMAGE_BYTES): must be a whole number from 1 to 2147483647",
				"media_max_image_side (MY_MEDIA_MAX_IMAGE_SIDE): must be a whole number from 1 to 2147483647",
				"media_unattached_ttl (MY_MEDIA_UNATTACHED_TTL): must be a positive Go duration, for example 24h or 90m",
			},
		},
		{
			name: "malformed previous KEK",
			env:  with(minimal(), "MY_KEK_PREVIOUS", testKEK1+",c2hvcnQ="),
			want: []string{"kek_previous (MY_KEK_PREVIOUS): key #2 must decode to 32 bytes, got 5"},
		},
		{
			name: "previous KEK repeats the current one",
			env:  with(minimal(), "MY_KEK_PREVIOUS", testKEK1+","+testKEK),
			want: []string{"kek_previous (MY_KEK_PREVIOUS): must not repeat a key or contain the current kek"},
		},
		{
			name: "previous KEK given twice",
			env:  with(minimal(), "MY_KEK_PREVIOUS", testKEK1+", "+testKEK1),
			want: []string{"kek_previous (MY_KEK_PREVIOUS): must not repeat a key or contain the current kek"},
		},
		{
			name: "malformed trusted proxies",
			env:  with(minimal(), "MY_TRUSTED_PROXIES", "10.0.0.0/8, 10.0.0.0/33", "MY_TRUSTED_PROXY_HEADER", "x-real-ip"),
			want: []string{
				`trusted_proxies (MY_TRUSTED_PROXIES): entry #2 "10.0.0.0/33" must be a CIDR such as 10.0.0.0/8 or an IP address`,
				`trusted_proxy_header (MY_TRUSTED_PROXY_HEADER): unknown value "x-real-ip"; want one of x-forwarded-for, forwarded`,
			},
		},
		{
			name: "unknown billing source",
			env:  with(minimal(), "MY_BILLING_SOURCES", "aws, azure"),
			want: []string{`billing_sources (MY_BILLING_SOURCES): "azure" is not a billing source; use aws or gcp, comma-separated`},
		},
		{
			name: "repeated billing source",
			env:  with(minimal(), "MY_BILLING_SOURCES", "aws,aws"),
			want: []string{`billing_sources (MY_BILLING_SOURCES): "aws" is listed twice`},
		},
		{
			name: "Google Cloud billing import without its table",
			env:  with(minimal(), "MY_BILLING_SOURCES", "gcp"),
			want: []string{`billing_gcp_table (MY_BILLING_GCP_TABLE): required when billing_sources includes gcp and billing_gcp_method is bigquery`},
		},
		{
			name: "Google Cloud billing import of a budget without its subscription",
			env:  with(minimal(), "MY_BILLING_SOURCES", "gcp", "MY_BILLING_GCP_METHOD", "budget"),
			want: []string{`billing_gcp_budget_subscription (MY_BILLING_GCP_BUDGET_SUBSCRIPTION): required when billing_sources includes gcp and billing_gcp_method is budget`},
		},
		{
			name: "malformed Google Cloud billing settings",
			env: with(minimal(), "MY_BILLING_GCP_METHOD", "pubsub", "MY_BILLING_GCP_TABLE", "billing_export.table",
				"MY_BILLING_GCP_QUERY_PROJECT", "My_Project", "MY_BILLING_GCP_PROJECTS", "my-social,my-social",
				"MY_BILLING_GCP_LABEL", "app", "MY_BILLING_GCP_MAX_BYTES_BILLED", "0", "MY_BILLING_GCP_BUDGET_SUBSCRIPTION", "my-billing-budget"),
			want: []string{
				`billing_gcp_method (MY_BILLING_GCP_METHOD): unknown value "pubsub"; want one of bigquery, budget`,
				`billing_gcp_table (MY_BILLING_GCP_TABLE): "billing_export.table" is not project.dataset.table, such as my-project.billing_export.gcp_billing_export_v1_012345_6789AB_CDEF01`,
				`billing_gcp_query_project (MY_BILLING_GCP_QUERY_PROJECT): "My_Project" is not a Google Cloud project ID, such as my-project`,
				`billing_gcp_projects (MY_BILLING_GCP_PROJECTS): "my-social" is listed twice`,
				`billing_gcp_label (MY_BILLING_GCP_LABEL): "app" is not key=value, such as app=my`,
				`billing_gcp_max_bytes_billed (MY_BILLING_GCP_MAX_BYTES_BILLED): must be a whole number from 1 to 9223372036854775807`,
				`billing_gcp_budget_subscription (MY_BILLING_GCP_BUDGET_SUBSCRIPTION): "my-billing-budget" is not the full name of a Pub/Sub subscription, such as projects/my-project/subscriptions/my-billing-budget`,
			},
		},
		{
			name: "AWS billing filter without its values",
			env:  with(minimal(), "MY_BILLING_AWS_FILTER", "tag"),
			want: []string{`billing_aws_tag (MY_BILLING_AWS_TAG): required when billing_aws_filter is tag`},
		},
		{
			name: "AWS billing linked accounts missing",
			env:  with(minimal(), "MY_BILLING_AWS_FILTER", "linked_account"),
			want: []string{`billing_aws_linked_accounts (MY_BILLING_AWS_LINKED_ACCOUNTS): required when billing_aws_filter is linked_account`},
		},
		{
			name: "malformed AWS billing settings",
			env: with(minimal(), "MY_BILLING_AWS_METRIC", "BlendedCost", "MY_BILLING_AWS_LINKED_ACCOUNTS", "1234",
				"MY_BILLING_AWS_TAG", "app", "MY_BILLING_AWS_ROLE_ARN", "arn:aws:iam::123456789012:user/me"),
			want: []string{
				`billing_aws_metric (MY_BILLING_AWS_METRIC): unknown value "BlendedCost"; want one of NetUnblendedCost, UnblendedCost`,
				`billing_aws_tag (MY_BILLING_AWS_TAG): "app" is not key=value, such as app=my`,
				`billing_aws_linked_accounts (MY_BILLING_AWS_LINKED_ACCOUNTS): "1234" is not a 12-digit AWS account ID`,
				`billing_aws_role_arn (MY_BILLING_AWS_ROLE_ARN): "arn:aws:iam::123456789012:user/me" is not the ARN of an IAM role, such as arn:aws:iam::123456789012:role/my-billing`,
			},
		},
		{
			name: "malformed webhook destinations",
			env:  with(minimal(), "MY_WEBHOOK_ALLOWED_DESTINATIONS", "n8n.internal, http://n8n.internal:5678"),
			want: []string{`webhook_allowed_destinations (MY_WEBHOOK_ALLOWED_DESTINATIONS): entry #2 "http://n8n.internal:5678" must be a lower-case host name, an IP address, or a CIDR such as 10.0.0.0/8`},
		},
		{
			name: "trusted proxy with a zone",
			env:  with(minimal(), "MY_TRUSTED_PROXIES", "fe80::1%eth0"),
			want: []string{`trusted_proxies (MY_TRUSTED_PROXIES): entry #1 "fe80::1%eth0" must be a CIDR such as 10.0.0.0/8 or an IP address`},
		},
		{
			name: "smtp without a sender or a host",
			env:  with(minimal(), "MY_MAIL", "smtp"),
			want: []string{
				"mail_from (MY_MAIL_FROM): required when mail is not none",
				"smtp_host (MY_SMTP_HOST): required when mail is smtp",
			},
		},
		{
			name: "malformed mail settings",
			env:  with(minimal(), "MY_MAIL", "smtp", "MY_MAIL_FROM", "My <noreply@example.org>", "MY_SMTP_HOST", "smtp.example.org:587", "MY_SMTP_PORT", "0"),
			want: []string{
				"mail_from (MY_MAIL_FROM): must be a bare email address such as noreply@example.com",
				"smtp_host (MY_SMTP_HOST): must be a host name or an IP address, without scheme or port",
				`smtp_port (MY_SMTP_PORT): port "0" must be a number from 1 to 65535`,
			},
		},
		{
			name: "captcha without keys",
			env:  with(minimal(), "MY_CAPTCHA", "recaptcha"),
			want: []string{
				"captcha_site_key (MY_CAPTCHA_SITE_KEY): required when captcha is not none",
				"captcha_secret_key (MY_CAPTCHA_SECRET_KEY): required when captcha is not none",
			},
		},
		{
			name: "stripe without keys",
			env:  with(minimal(), "MY_PAYMENTS_PROVIDER", "stripe"),
			want: []string{
				"payments_stripe_secret_key (MY_PAYMENTS_STRIPE_SECRET_KEY): required when payments_provider is stripe",
				"payments_stripe_webhook_secret (MY_PAYMENTS_STRIPE_WEBHOOK_SECRET): required when payments_provider is stripe",
			},
		},
		{
			name: "stripe with a publishable key and a bare secret",
			env: with(with(with(minimal(), "MY_PAYMENTS_PROVIDER", "stripe"),
				"MY_PAYMENTS_STRIPE_SECRET_KEY", "pk_live_abc"), "MY_PAYMENTS_STRIPE_WEBHOOK_SECRET", "whsec_"),
			want: []string{
				"payments_stripe_secret_key (MY_PAYMENTS_STRIPE_SECRET_KEY): must be a Stripe secret key",
				"payments_stripe_webhook_secret (MY_PAYMENTS_STRIPE_WEBHOOK_SECRET): must be a Stripe webhook signing secret",
			},
		},
		{
			name: "vapid private key that is not a P-256 key",
			env:  with(minimal(), "MY_VAPID_PRIVATE_KEY", "c2hvcnQ"),
			want: []string{"vapid_private_key (MY_VAPID_PRIVATE_KEY): "},
		},
		{
			name:  "captcha secret key in the config file",
			env:   with(minimal(), "MY_CONFIG", "my.toml"),
			files: map[string]string{"my.toml": "captcha = \"hcaptcha\"\ncaptcha_site_key = \"site\"\ncaptcha_secret_key = \"0x5eCrE7\"\n"},
			want: []string{
				"captcha_secret_key (MY_CAPTCHA_SECRET_KEY): is a secret; set MY_CAPTCHA_SECRET_KEY or MY_CAPTCHA_SECRET_KEY_FILE instead of the config file",
			},
		},
		{
			name: "unknown captcha service",
			env:  with(minimal(), "MY_CAPTCHA", "recaptcha-v3"),
			want: []string{`captcha (MY_CAPTCHA): unknown value "recaptcha-v3"`},
		},
		{
			name: "inline queue with a split role",
			env:  with(minimal(), "MY_QUEUE", "inline", "MY_ROLE", "worker"),
			want: []string{"queue (MY_QUEUE): inline runs jobs in the enqueuing process and requires role = all, not worker"},
		},
		{
			name: "metrics on the public listener",
			env:  with(minimal(), "MY_METRICS_EXPORTER", "prometheus", "MY_METRICS_LISTEN", ":8080"),
			want: []string{"metrics_listen (MY_METRICS_LISTEN): must differ from listen: the metrics are never served on the public listener"},
		},
		{
			name: "bad emf dimensions",
			env:  with(minimal(), "MY_METRICS_EMF_DIMENSIONS", "my.surface, Account"),
			want: []string{"metrics_emf_dimensions (MY_METRICS_EMF_DIMENSIONS): must be comma-separated label keys of lower-case letters, digits, dots, and underscores, such as my.surface"},
		},
		{
			name: "trace sample ratio above 1",
			env:  with(minimal(), "MY_TRACES_SAMPLE_RATIO", "1.5"),
			want: []string{"traces_sample_ratio (MY_TRACES_SAMPLE_RATIO): must be a number from 0 to 1, for example 0.01"},
		},
		{
			name: "trace sample ratio not a number",
			env:  with(minimal(), "MY_TRACES_SAMPLE_RATIO", "NaN"),
			want: []string{"traces_sample_ratio (MY_TRACES_SAMPLE_RATIO): must be a number from 0 to 1, for example 0.01"},
		},
		{
			name: "private networks in the production profile",
			env:  with(minimal(), "MY_HTTP_ALLOW_PRIVATE_NETWORKS", "true"),
			want: []string{"http_allow_private_networks (MY_HTTP_ALLOW_PRIVATE_NETWORKS): true is refused in the production profile; set profile = development"},
		},
		{
			name: "secret sources",
			env:  with(minimal(), "MY_DATABASE_URL_FILE", "/x", "MY_KEK", unset, "MY_KEK_FILE", "/missing"),
			want: []string{
				"database_url (MY_DATABASE_URL): set only one of MY_DATABASE_URL and MY_DATABASE_URL_FILE",
				"kek (MY_KEK): MY_KEK_FILE: open /missing: file does not exist",
			},
		},
		{
			name: "file problems",
			env:  minimal(),
			file: "my.toml",
			files: map[string]string{"my.toml": `
hots = "typo.example"
role = 1
kek = "nope"
`},
			want: []string{
				`config file my.toml: unknown key "hots"`,
				"role (MY_ROLE): must be a string in the config file, got int64",
				"kek (MY_KEK): is a secret; set MY_KEK or MY_KEK_FILE instead of the config file",
			},
		},
		{
			name: "file not found",
			env:  with(minimal(), "MY_CONFIG", "nope.toml"),
			want: []string{"config file: open nope.toml: file does not exist"},
		},
		{
			name:  "file syntax error",
			env:   minimal(),
			file:  "bad.toml",
			files: map[string]string{"bad.toml": "role = "},
			want:  []string{"config file bad.toml: toml: "},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := source(tt.env, tt.files)
			src.File = tt.file
			cfg, _, err := Load(src)
			if cfg != nil {
				t.Errorf("Load() config = %+v, want nil", cfg)
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("Load() error = %v, want *ValidationError", err)
			}
			got := make([]string, len(ve.Issues))
			for i, is := range ve.Issues {
				got[i] = is.String()
			}
			if len(got) != len(tt.want) {
				t.Fatalf("issues =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
			for i := range got {
				if !strings.HasPrefix(got[i], tt.want[i]) {
					t.Errorf("issue %d = %q, want prefix %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestLoadSettingsRedactSecrets(t *testing.T) {
	_, settings, err := Load(source(with(minimal(), "MY_KEK_PREVIOUS", testKEK1, "MY_SMTP_PASSWORD", "hunter3",
		"MY_CAPTCHA", "turnstile", "MY_CAPTCHA_SITE_KEY", "site", "MY_CAPTCHA_SECRET_KEY", "0x5eCrE7",
		"MY_VAPID_PRIVATE_KEY", "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw",
		"MY_KV", "valkey", "MY_VALKEY_URL", "valkey://user:pass@cache:6379/0",
		"MY_STREAM_POSTGRES_URL", "postgres://user:pass@db/my",
		"MY_PAYMENTS_PROVIDER", "stripe", "MY_PAYMENTS_STRIPE_SECRET_KEY", "rk_test_abc", "MY_PAYMENTS_STRIPE_WEBHOOK_SECRET", "whsec_abc"), nil))
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, len(settings))
	for i, s := range settings {
		keys[i] = s.Key
		if s.Secret && s.Value != "(redacted)" {
			t.Errorf("%s = %q, want redacted", s.Key, s.Value)
		}
	}
	for i := range schema {
		if !slices.Contains(keys, schema[i].key) {
			t.Errorf("settings miss %s", schema[i].key)
		}
	}
}

func TestSchemaWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for i := range schema {
		f := &schema[i]
		if seen[f.key] {
			t.Errorf("duplicate key %s", f.key)
		}
		seen[f.key] = true
		if f.doc == "" || f.set == nil {
			t.Errorf("%s: doc and set are required", f.key)
		}
		if f.def != "" {
			if err := validate(f, f.def); err != nil {
				t.Errorf("%s: default %q is invalid: %v", f.key, f.def, err)
			}
		}
		if f.secret && f.def != "" {
			t.Errorf("%s: secrets have no default", f.key)
		}
	}
}

// TestServerOrigin checks that the server is reached over plain HTTP only when web_origin says so
// on host, and over HTTPS in every other setup.
func TestServerOrigin(t *testing.T) {
	for _, c := range []struct{ host, webOrigin, want string }{
		{"social.example.org", unset, "https://social.example.org"},
		{"social.example.org", "https://social.example.org", "https://social.example.org"},
		{"localhost:8080", "http://localhost:8080", "http://localhost:8080"},
		// A web client served elsewhere (a development server) says nothing about the server's scheme.
		{"localhost:8080", "http://localhost:5173", "https://localhost:8080"},
		{"social.example.org", "http://localhost:5173", "https://social.example.org"},
	} {
		cfg, _, err := Load(source(with(minimal(), "MY_HOST", c.host, "MY_WEB_ORIGIN", c.webOrigin), nil))
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.ServerOrigin(); got != c.want {
			t.Errorf("host %q, web origin %q: ServerOrigin() = %q, want %q", c.host, c.webOrigin, got, c.want)
		}
	}
}

// TestStatusPageURL checks that status_page_url is empty by default and takes only an absolute https URL
// (ADR-0085 §6).
func TestStatusPageURL(t *testing.T) {
	cfg, _, err := Load(source(minimal(), nil))
	if err != nil || cfg.StatusPageURL != "" {
		t.Fatalf("default: StatusPageURL = %q, err = %v", cfg.StatusPageURL, err)
	}
	for _, v := range []string{"https://status.example.org", "https://example.org/status?page=1"} {
		cfg, _, err := Load(source(with(minimal(), "MY_STATUS_PAGE_URL", v), nil))
		if err != nil {
			t.Errorf("%q: %v", v, err)
			continue
		}
		if cfg.StatusPageURL != v {
			t.Errorf("%q: StatusPageURL = %q", v, cfg.StatusPageURL)
		}
	}
	for _, v := range []string{
		"http://status.example.org",
		"https:///no-host",
		"https://user@status.example.org",
		"https://status.example.org#now",
		"mailto:status@example.org",
		"/status",
		"status.example.org",
	} {
		if _, _, err := Load(source(with(minimal(), "MY_STATUS_PAGE_URL", v), nil)); err == nil || !strings.Contains(err.Error(), "status_page_url (MY_STATUS_PAGE_URL)") {
			t.Errorf("%q: err = %v, want a status_page_url error", v, err)
		}
	}
}

// TestSourceURL checks that source_url takes an https URL or a mailto: address (AGPL-3.0 section 13) and that the
// server refuses to start with anything else.
func TestSourceURL(t *testing.T) {
	for _, v := range []string{
		"https://github.com/mycroblog/my",
		"https://git.example.org/fork/my/-/tree/v1.2.3",
		"mailto:source@example.org",
		"mailto:source@example.org?subject=Source%20code",
	} {
		cfg, _, err := Load(source(with(minimal(), "MY_SOURCE_URL", v), nil))
		if err != nil {
			t.Errorf("%q: %v", v, err)
			continue
		}
		if cfg.SourceURL != v {
			t.Errorf("%q: SourceURL = %q", v, cfg.SourceURL)
		}
	}
	for _, v := range []string{
		"http://github.com/mycroblog/my",
		"https:///no-host",
		"https://user@git.example.org/my",
		"https://git.example.org/my#readme",
		"ftp://git.example.org/my",
		"ssh://example.org/mycroblog/my",
		"mailto:",
		"mailto:no-address",
		"mailto://source@example.org",
		"/relative/path",
	} {
		if _, _, err := Load(source(with(minimal(), "MY_SOURCE_URL", v), nil)); err == nil || !strings.Contains(err.Error(), "source_url (MY_SOURCE_URL)") {
			t.Errorf("%q: err = %v, want a source_url error", v, err)
		}
	}
}

func TestConfigLogValueRedactsSecrets(t *testing.T) {
	cfg, _, err := Load(source(with(minimal(), "MY_DATABASE_URL", "postgres://user:hunter2@db/my", "MY_KEK_PREVIOUS", testKEK1, "MY_SMTP_PASSWORD", "hunter3",
		"MY_CAPTCHA", "hcaptcha", "MY_CAPTCHA_SITE_KEY", "site", "MY_CAPTCHA_SECRET_KEY", "0x5eCrE7",
		"MY_KV", "valkey", "MY_VALKEY_URL", "valkey://user:pass@cache:6379/0",
		"MY_STREAM_POSTGRES_URL", "postgres://user:pass@db/my"), nil))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("config", "config", cfg)
	out := buf.String()
	for _, secret := range []string{"hunter2", "hunter3", "0x5eCrE7", string(cfg.KEK), string(cfg.PreviousKEKs[0]), testKEK, testKEK1} {
		if strings.Contains(out, secret) {
			t.Errorf("log output %q contains secret %q", out, secret)
		}
	}
	for _, want := range []string{"config.host=social.example.org", "config.database_url=(redacted)", "config.kek=(redacted)", "config.kek_previous=1", "config.captcha_secret_key=(redacted)"} {
		if !strings.Contains(out, want) {
			t.Errorf("log output %q does not contain %q", out, want)
		}
	}
}

// TestMediaOrigin checks that media_origin is empty by default, which serves the media from the
// server's own origin, and takes an https origin, or an http one on a loopback host.
func TestMediaOrigin(t *testing.T) {
	cfg, _, err := Load(source(minimal(), nil))
	if err != nil || cfg.MediaOrigin != "" || cfg.MediaBaseURL() != cfg.ServerOrigin() {
		t.Fatalf("default: MediaOrigin = %q, MediaBaseURL() = %q, err = %v", cfg.MediaOrigin, cfg.MediaBaseURL(), err)
	}
	for _, v := range []string{"https://media.example", "https://cdn.example.org:8443", "http://localhost:9000", "http://127.0.0.1:9000", "http://[::1]:9000"} {
		cfg, _, err := Load(source(with(minimal(), "MY_MEDIA_ORIGIN", v), nil))
		if err != nil {
			t.Errorf("%q: %v", v, err)
			continue
		}
		if cfg.MediaOrigin != v || cfg.MediaBaseURL() != v {
			t.Errorf("%q: MediaOrigin = %q, MediaBaseURL() = %q", v, cfg.MediaOrigin, cfg.MediaBaseURL())
		}
	}
	for _, v := range []string{
		"https://media.example/media",
		"https://media.example/",
		"https://media.example?x=1",
		"https://media.example#x",
		"http://media.example",
		"http://192.0.2.1:9000",
		"ftp://media.example",
		"media.example",
		"https://user@media.example",
		"https://Media.example",
		"/media",
	} {
		if _, _, err := Load(source(with(minimal(), "MY_MEDIA_ORIGIN", v), nil)); err == nil || !strings.Contains(err.Error(), "media_origin (MY_MEDIA_ORIGIN)") {
			t.Errorf("%q: err = %v, want a media_origin error", v, err)
		}
	}
}
