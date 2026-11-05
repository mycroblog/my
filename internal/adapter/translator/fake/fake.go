// SPDX-License-Identifier: AGPL-3.0-only

// Package fake is a translation.Translator for tests, which calls no provider: a translation
// is the fragment behind the target language, "[ja] Hello ⟦1⟧", with its markers untouched.
package fake

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/mycroblog/my/internal/app/translation"
)

// Options configures a Translator.
type Options struct {
	// Detected is the language it reports for a source of translation.UndeterminedLanguage.
	// Left empty, it reports translation.UndeterminedLanguage, as a provider that could not tell.
	Detected string
	// Unsupported lists the source and target languages it refuses with
	// translation.ErrUnsupportedLanguage.
	Unsupported []string
}

// Translator is the fake provider.
type Translator struct {
	opts Options

	mu    sync.Mutex
	fails []error
}

// New returns a Translator configured by opts.
func New(opts Options) *Translator {
	return &Translator{opts: opts}
}

// FailNext makes the next call of Translate return err, without translating. Several calls
// queue their errors in order.
func (f *Translator) FailNext(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fails = append(f.fails, err)
}

// Translate implements translation.Translator.
func (f *Translator) Translate(ctx context.Context, req translation.Request) (translation.Result, error) {
	if err := ctx.Err(); err != nil {
		return translation.Result{}, fmt.Errorf("%w: %w", translation.ErrTemporary, err)
	}
	if err := f.nextFailure(); err != nil {
		return translation.Result{}, err
	}
	for _, language := range []string{req.SourceLanguage, req.TargetLanguage} {
		if slices.Contains(f.opts.Unsupported, language) {
			return translation.Result{}, fmt.Errorf("%w: %s", translation.ErrUnsupportedLanguage, language)
		}
	}
	translations := make([]string, len(req.Fragments))
	for i, fragment := range req.Fragments {
		translations[i] = "[" + req.TargetLanguage + "] " + fragment
	}
	detected := req.SourceLanguage
	if detected == translation.UndeterminedLanguage && f.opts.Detected != "" {
		detected = f.opts.Detected
	}
	return translation.Result{Translations: translations, DetectedLanguage: detected}, nil
}

func (f *Translator) nextFailure() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.fails) == 0 {
		return nil
	}
	err := f.fails[0]
	f.fails = f.fails[1:]
	return err
}
