// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_awskms

package config

import "github.com/mycroblog/my/internal/adapter/keymanager/awskms"

func checkAWSKMSKeyName(v string) error { return awskms.CheckKeyName(v) }
