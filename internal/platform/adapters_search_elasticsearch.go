// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_search_elasticsearch

package platform

import (
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/mycroblog/my/internal/adapter/search/elasticsearch"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { searchIndexAdapters["elasticsearch"] = newElasticsearchIndex }

// elasticsearchTimeout bounds one request to the cluster. The calls of the search module have
// shorter deadlines of their own; this one bounds the setup of the index at start and the clear of
// my search clear.
const elasticsearchTimeout = time.Minute

// newElasticsearchIndex returns the Index of search = elasticsearch, with the index created or
// upgraded to the mapping of this build.
func newElasticsearchIndex(ctx context.Context, cfg *config.Config, logger *slog.Logger, _ kernel.Meter) (search.Index, error) {
	client := searchHTTPClient(elasticsearchTimeout)
	if cfg.SearchElasticsearchCAFile != "" {
		pem, err := os.ReadFile(cfg.SearchElasticsearchCAFile)
		if err != nil {
			return nil, fmt.Errorf("search_elasticsearch_ca_file: %w", err)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("search_elasticsearch_ca_file: no PEM certificate in the file")
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
		client.Transport = transport
	}
	return elasticsearch.Open(ctx, elasticsearch.Options{
		URL: cfg.SearchElasticsearchURL, APIKey: cfg.SearchElasticsearchAPIKey,
		Username: cfg.SearchElasticsearchUsername, Password: cfg.SearchElasticsearchPassword,
		Index: cmp.Or(cfg.SearchElasticsearchIndex, elasticsearch.DefaultIndex), Settings: cfg.SearchElasticsearchIndexSettings,
		HTTPClient: client, Logger: logger,
	})
}
