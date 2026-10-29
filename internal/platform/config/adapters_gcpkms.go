// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_gcpkms

package config

import "github.com/mycroblog/my/internal/adapter/keymanager/gcpkms"

func checkGCPKMSKeyName(v string) error { return gcpkms.CheckKeyName(v) }
