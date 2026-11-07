// SPDX-License-Identifier: AGPL-3.0-only

// Package supporters holds the use cases of the supporters (ADR-0072, FR-910, FR-911): paid
// plans that local users subscribe to from the web client, and the roles that follow the state
// of their payments.
//
// The boundary with the payment provider is the PaymentProvider port (ADR-0074 §1), whose
// adapters live in internal/adapter/payment/* and pass supporterstest.PaymentProviderContract. The
// provider's facts reach this module on one path: the webhook endpoint records each delivery once
// (Payments.ReceiveWebhook) and queues JobPaymentEvent, and the periodic JobPaymentReconcile
// reads the provider's lists; both re-read the provider's current state and hand it to the
// Syncer, the subscription core that derives our state from it (supporters.sync, ADR-0072 §7).
// The records of the deliveries and of the reconciliations are the PaymentRepository's, whose
// database adapters pass supporterstest.PaymentRepositoryContract.
package supporters
