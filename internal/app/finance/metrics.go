// SPDX-License-Identifier: AGPL-3.0-only

package finance

import "github.com/mycroblog/my/internal/app/kernel"

// Label keys of the billing import metric (ADR-0055 §3).
const (
	// LabelBillingSource is the billing source of an import (billing_sources: aws, gcp).
	LabelBillingSource = "my.billing.source"
	LabelOutcome       = "my.outcome"
	// LabelErrorType is the ImportErrorKind of a failed import.
	LabelErrorType = kernel.AttrErrorType
)

// The outcomes of an import, the my.outcome label of my.billing.imports.
const (
	ImportOutcomeSuccess = "success"
	ImportOutcomeFailed  = "failed"
)

// BillingImports counts the attempts of the billing imports, the failures that are not retried
// included: they complete the job, so the job metrics never show them as dead.
var BillingImports = kernel.Instrument{
	Name: "my.billing.imports", Unit: "{import}", Description: "Attempts of the imports of the cloud usage charges, by source and outcome.",
	LabelKeys: []string{LabelBillingSource, LabelOutcome, LabelErrorType},
}
