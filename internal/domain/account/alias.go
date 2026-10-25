// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"errors"
	"time"
)

// MaxAliases is how many aliases a local account has at most (ADR-0089).
const MaxAliases = 5

var (
	// ErrTooManyAliases reports an alias beyond MaxAliases.
	ErrTooManyAliases = errors.New("account: the account has too many aliases")
	// ErrSelfAlias reports an alias that is the account itself.
	ErrSelfAlias = errors.New("account: an account cannot be its own alias")
)

// Alias is an account that a local account declares as its own, as its actor's alsoKnownAs
// (account_aliases, ADR-0089): the account it moves from, which may then move to it. An alias
// alone changes nothing.
type Alias struct {
	AccountID string
	// URI is the ActivityPub id of the alias's actor.
	URI     string
	AddedAt time.Time
}

// Validate checks the invariants of a new alias. uri is the account's own ActivityPub id.
func (a Alias) Validate(uri string) error {
	if a.AccountID == "" || a.URI == "" {
		return errors.New("account: an alias needs an account and a URI")
	}
	if a.URI == uri {
		return ErrSelfAlias
	}
	return nil
}
