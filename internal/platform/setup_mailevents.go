// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"fmt"

	"github.com/mycroblog/my/internal/interface/mailevents"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpMailEvents registers POST /mail/events/ses, which records the bounces and complaints that
// SES publishes to the SNS topic ses_sns_topic_arn (ADR-0086 §5). Without the topic nothing is
// registered and the path answers 404 like any unknown path. Its requests count under the oauth
// surface, with the other /mail endpoints of the server.
func (a *App) setUpMailEvents(cfg *config.Config) error {
	if cfg.SESSNSTopicARN == "" {
		return nil
	}
	topic, err := mailevents.ParseTopicARN(cfg.SESSNSTopicARN)
	if err != nil {
		return fmt.Errorf("ses_sns_topic_arn: %w", err)
	}
	mailevents.New(mailevents.Options{
		Topic: topic, Recorder: a.Identity, HTTPClient: a.HTTPClient, KV: a.KV, Clock: a.Clock,
		Problem: a.writeStatusProblem, Logger: a.Logger,
	}).Register(surfaceRouter{a, surfaceOAuth})
	return nil
}
