// SPDX-License-Identifier: AGPL-3.0-only

package fakeplugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"connectrpc.com/connect/v2"

	keymanagerv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/key_manager/v1"
	keymanagerv1connect "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/key_manager/v1/key_managerv1connect"
	"github.com/mycroblog/my/internal/app/kernel"
)

// errKeyStoreFailed is what the fake answers when its KeyManager fails with an error that is not a result of the port.
var errKeyStoreFailed = connect.NewError(connect.CodeUnavailable, "the key store failed")

type keyManagerServer struct {
	keymanagerv1connect.UnimplementedKeyManagerServiceHandler

	o Options
}

// misbehave says what the options of the fake make of a call: an empty answer, an error, or neither to answer it.
func (s *keyManagerServer) misbehave(ctx context.Context) (empty bool, err error) {
	switch {
	case s.o.KeyManager == nil:
		return false, connect.NewError(connect.CodeUnimplemented, "this fake has no key manager")
	case s.o.Block:
		<-ctx.Done()
		return false, ctx.Err()
	}
	return s.o.Broken, nil
}

// Describe implements keymanagerv1connect.KeyManagerServiceHandler.
func (s *keyManagerServer) Describe(ctx context.Context, _ *keymanagerv1.DescribeRequest) (*keymanagerv1.DescribeResponse, error) {
	if empty, err := s.misbehave(ctx); err != nil || empty {
		return new(keymanagerv1.DescribeResponse), err
	}
	return &keymanagerv1.DescribeResponse{Outcome: keymanagerv1.Outcome_OUTCOME_OK, KeyId: s.o.KeyManager.CurrentKeyID()}, nil
}

// WrapKey implements keymanagerv1connect.KeyManagerServiceHandler.
func (s *keyManagerServer) WrapKey(ctx context.Context, req *keymanagerv1.WrapKeyRequest) (*keymanagerv1.WrapKeyResponse, error) {
	if empty, err := s.misbehave(ctx); err != nil || empty {
		return new(keymanagerv1.WrapKeyResponse), err
	}
	id, wrapped, err := s.o.KeyManager.WrapKey(ctx, req.GetDek(), req.GetAad())
	if err != nil {
		return nil, errKeyStoreFailed
	}
	return &keymanagerv1.WrapKeyResponse{Outcome: keymanagerv1.Outcome_OUTCOME_OK, KeyId: id, Wrapped: wrapped}, nil
}

// UnwrapKey implements keymanagerv1connect.KeyManagerServiceHandler.
func (s *keyManagerServer) UnwrapKey(ctx context.Context, req *keymanagerv1.UnwrapKeyRequest) (*keymanagerv1.UnwrapKeyResponse, error) {
	if empty, err := s.misbehave(ctx); err != nil || empty {
		return new(keymanagerv1.UnwrapKeyResponse), err
	}
	return answerUnwrap(ctx, s.o.KeyManager, req)
}

// answerUnwrap answers a call from a port's KeyManager, as a plugin that holds the KEK does.
func answerUnwrap(ctx context.Context, km kernel.KeyManager, req *keymanagerv1.UnwrapKeyRequest) (*keymanagerv1.UnwrapKeyResponse, error) {
	dek, err := km.UnwrapKey(ctx, req.GetKeyId(), req.GetWrapped(), req.GetAad())
	switch {
	case err == nil:
		return &keymanagerv1.UnwrapKeyResponse{Outcome: keymanagerv1.Outcome_OUTCOME_OK, Dek: dek}, nil
	case errors.Is(err, kernel.ErrUnknownKey):
		return &keymanagerv1.UnwrapKeyResponse{Outcome: keymanagerv1.Outcome_OUTCOME_UNKNOWN_KEY}, nil
	case errors.Is(err, kernel.ErrDecrypt):
		return &keymanagerv1.UnwrapKeyResponse{Outcome: keymanagerv1.Outcome_OUTCOME_DECRYPT}, nil
	default:
		return nil, errKeyStoreFailed
	}
}

// keyManagerJSON is every message of the kind in the shape of proto3's JSON mapping: camelCase names, base64 for bytes (which
// encoding/json does for a []byte), a name for an enum, and fields at their zero value left out.
type keyManagerJSON struct {
	Outcome string `json:"outcome,omitempty"`
	KeyID   string `json:"keyId,omitempty"`
	DEK     []byte `json:"dek,omitempty"`
	AAD     []byte `json:"aad,omitempty"`
	Wrapped []byte `json:"wrapped,omitempty"`
	Extra   string `json:"extra,omitempty"`
}

// serveKeyManagerJSON adds the key manager kind of the fake of net/http and encoding/json alone.
func serveKeyManagerJSON(mux *http.ServeMux, o Options) {
	if o.KeyManager == nil {
		return
	}
	handle := func(procedure string, answer func(context.Context, keyManagerJSON) (keyManagerJSON, error)) {
		mux.HandleFunc("POST "+procedure, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/json" {
				http.Error(w, "send JSON", http.StatusUnsupportedMediaType)
				return
			}
			var req keyManagerJSON
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad JSON", http.StatusBadRequest)
				return
			}
			if o.Block {
				<-r.Context().Done()
				return
			}
			if o.Broken {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte("{}"))
				return
			}
			res, err := answer(r.Context(), req)
			if err != nil {
				http.Error(w, "the key store failed", http.StatusServiceUnavailable)
				return
			}
			res.Extra = "a field the server does not know, which it ignores"
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(res)
		})
	}
	ok := keymanagerv1.Outcome_OUTCOME_OK.String()
	handle(keymanagerv1connect.KeyManagerServiceDescribeProcedure, func(context.Context, keyManagerJSON) (keyManagerJSON, error) {
		return keyManagerJSON{Outcome: ok, KeyID: o.KeyManager.CurrentKeyID()}, nil
	})
	handle(keymanagerv1connect.KeyManagerServiceWrapKeyProcedure, func(ctx context.Context, req keyManagerJSON) (keyManagerJSON, error) {
		id, wrapped, err := o.KeyManager.WrapKey(ctx, req.DEK, req.AAD)
		return keyManagerJSON{Outcome: ok, KeyID: id, Wrapped: wrapped}, err
	})
	handle(keymanagerv1connect.KeyManagerServiceUnwrapKeyProcedure, func(ctx context.Context, req keyManagerJSON) (keyManagerJSON, error) {
		res, err := answerUnwrap(ctx, o.KeyManager, &keymanagerv1.UnwrapKeyRequest{KeyId: req.KeyID, Wrapped: req.Wrapped, Aad: req.AAD})
		if err != nil {
			return keyManagerJSON{}, err
		}
		return keyManagerJSON{Outcome: res.GetOutcome().String(), DEK: res.GetDek()}, nil
	})
}
