// The integration tests of pkg/mybot (ADR-0094 §10): a module of its own, never published, that joins the server
// module, pkg/mykit, and pkg/mybot through the replace directives below, so that neither the root module's go.mod
// nor the libraries' take in the others.
module github.com/mycroblog/my/pkg/mybot/integration

go 1.27.3

require (
	github.com/mycroblog/my v0.0.0
	github.com/mycroblog/my/pkg/mybot v0.0.0
	github.com/mycroblog/my/pkg/mykit v0.0.0
)

require (
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth v0.24.1-0.20261001053825-dbc26066f70a // indirect
	cloud.google.com/go/auth/oauth2adapt v0.3.0 // indirect
	cloud.google.com/go/compute/metadata v0.10.0 // indirect
	cloud.google.com/go/iam v1.14.0 // indirect
	cloud.google.com/go/pubsub/v2 v2.8.0 // indirect
	code.superseriousbusiness.org/httpsig v1.5.0 // indirect
	github.com/agentable/go-intl v0.2.17 // indirect
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/aws/aws-sdk-go-v2 v1.47.4 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.33.10 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.20.10 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.20.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.7 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.7 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/costexplorer v1.74.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/sesv2 v1.79.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.10.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/sns v1.47.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/sqs v1.52.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.38.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.43.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.51.5 // indirect
	github.com/aws/smithy-go v1.28.6 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cayleygraph/quad v1.3.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cockroachdb/apd/v3 v3.2.3 // indirect
	github.com/dadrus/httpsig v0.9.0 // indirect
	github.com/deepteams/webp v1.2.8 // indirect
	github.com/dunglas/httpsfv v1.1.2 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.4 // indirect
	github.com/getkin/kin-openapi v0.152.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v1.0.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/go-webauthn/webauthn v0.18.2 // indirect
	github.com/go-webauthn/x v0.3.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/google/s2a-go v0.1.11 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.23 // indirect
	github.com/googleapis/gax-go/v2 v2.27.0 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.31.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.11.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/kaptinlin/messageformat-go/mf1 v0.8.6 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/microcosm-cc/bluemonday v1.0.27 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/oapi-codegen/runtime v1.8.0 // indirect
	github.com/oasdiff/yaml v0.1.1 // indirect
	github.com/oasdiff/yaml3 v0.0.14 // indirect
	github.com/pelletier/go-toml/v2 v2.4.3 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/piprate/json-gold v0.9.1 // indirect
	github.com/pquerna/cachecontrol v0.2.0 // indirect
	github.com/prometheus/client_golang v1.25.0 // indirect
	github.com/prometheus/client_model v0.6.3 // indirect
	github.com/prometheus/common v0.72.0 // indirect
	github.com/prometheus/otlptranslator v1.0.0 // indirect
	github.com/prometheus/procfs v0.22.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/stripe/stripe-go/v87 v87.0.0 // indirect
	github.com/tinylib/msgp v1.6.4 // indirect
	github.com/valkey-io/valkey-go v1.0.79 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/yuin/goldmark v1.8.6 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.71.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.71.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/runtime v0.72.0 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.23.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.47.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.47.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.47.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.69.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/sdk v1.47.0 // indirect
	go.opentelemetry.io/otel/sdk/log v1.47.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	go.opentelemetry.io/proto/otlp v1.11.1 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/image v0.47.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/oauth2 v0.37.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	google.golang.org/api v0.304.0 // indirect
	google.golang.org/genproto v0.0.0-20261012220534-4341edf3f941 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20261012220534-4341edf3f941 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261012220534-4341edf3f941 // indirect
	google.golang.org/grpc v1.84.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	modernc.org/libc v1.79.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.61.0 // indirect
	rsc.io/qr v0.2.0 // indirect
)

replace (
	github.com/mycroblog/my => ../../..
	github.com/mycroblog/my/pkg/mybot => ..
	github.com/mycroblog/my/pkg/mykit => ../../mykit
)
