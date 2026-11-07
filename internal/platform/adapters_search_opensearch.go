// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_search_opensearch

package platform

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	"github.com/mycroblog/my/internal/adapter/search/opensearch"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { searchIndexAdapters["opensearch"] = newOpenSearchIndex }

// openSearchEnsureTimeout bounds the creation of the index at start.
const openSearchEnsureTimeout = 30 * time.Second

// openSearchHTTPTimeout bounds one request: the callers bound theirs, a job's call by seconds
// and my search clear by minutes, so the client's only stops a request that nothing bounds.
const openSearchHTTPTimeout = 15 * time.Minute

// newOpenSearchIndex returns the Index of search = opensearch, after creating its index or moving
// its alias to the index of this release's mapping.
func newOpenSearchIndex(ctx context.Context, cfg *config.Config, logger *slog.Logger) (search.Index, error) {
	o := opensearch.Options{
		URL: cfg.SearchOpenSearchURL, Index: cmp.Or(cfg.SearchOpenSearchIndex, opensearch.DefaultIndex),
		Username: cfg.SearchOpenSearchUsername, Password: cfg.SearchOpenSearchPassword,
		HTTPClient: searchHTTPClient(openSearchHTTPTimeout), Logger: logger,
	}
	if cfg.SearchOpenSearchAWSSigning == opensearch.ServiceManaged {
		// The credentials come from the SDK's default chain: the execution role, the
		// environment, the shared files. They sign the requests of the adapter's own client,
		// which reaches only the configured domain.
		var opts []func(*awsconfig.LoadOptions) error
		if cfg.SearchOpenSearchAWSRegion != "" {
			opts = append(opts, awsconfig.WithRegion(cfg.SearchOpenSearchAWSRegion))
		}
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("load the AWS configuration: %w", err)
		}
		o.AWS = &opensearch.AWSSigning{Credentials: awsCfg.Credentials, Region: awsCfg.Region, Service: opensearch.ServiceManaged}
	}
	if path := cfg.SearchOpenSearchIndexSettingsFile; path != "" {
		// The configuration's check read it already; its contents never reach a log or an error.
		b, err := os.ReadFile(path) //nolint:gosec // the operator chose the path
		if err != nil {
			return nil, fmt.Errorf("read search_opensearch_index_settings_file: %w", err)
		}
		o.Settings = b
	}
	index, err := opensearch.New(o)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, openSearchEnsureTimeout)
	defer cancel()
	if err := index.Ensure(ctx); err != nil {
		return nil, err
	}
	return index, nil
}
