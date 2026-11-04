// SPDX-License-Identifier: AGPL-3.0-only

package stripe

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stripe/stripe-go/v87"

	"github.com/mycroblog/my/internal/app/supporters"
)

func TestClassifyKeepsTheBodyOnlyOfARefusedKey(t *testing.T) {
	const msg = "Enabling Balance Transaction Sources Read would allow this request"
	refused := classify(context.Background(), &stripe.Error{HTTPStatusCode: http.StatusForbidden, Msg: msg})
	if !errors.Is(refused, supporters.ErrPaymentPermission) || !strings.Contains(refused.Error(), msg) {
		t.Errorf("refused key: %v", refused)
	}
	other := classify(context.Background(), &stripe.Error{HTTPStatusCode: http.StatusBadRequest, Msg: msg})
	if !errors.Is(other, supporters.ErrPaymentInvalidRequest) || strings.Contains(other.Error(), msg) {
		t.Errorf("invalid request: %v", other)
	}
}
