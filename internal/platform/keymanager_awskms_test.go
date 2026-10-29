// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_awskms

package platform_test

import (
	"path/filepath"
	"testing"

	"github.com/mycroblog/my/internal/adapter/keymanager/awskms"
	"github.com/mycroblog/my/internal/adapter/keymanager/awskms/awskmstest"
	"github.com/mycroblog/my/internal/app/kernel"
)

const testAWSKMSKey = "arn:aws:kms:us-east-1:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab"

// useAWSKMSStandIn points the AWS SDK's default chain at a stand-in for AWS KMS with the key
// testAWSKMSKey and its alias alias/my-kek, with static credentials and without the shared files
// or the instance metadata of the host, so that nothing reaches AWS.
func useAWSKMSStandIn(t *testing.T) *awskmstest.Server {
	t.Helper()
	srv, _ := awskmstest.Start(t, testAWSKMSKey)
	srv.AddAlias("alias/my-kek", testAWSKMSKey)
	none := filepath.Join(t.TempDir(), "none")
	for k, v := range map[string]string{
		"AWS_ENDPOINT_URL_KMS": srv.URL, "AWS_REGION": awskmstest.Region,
		"AWS_ACCESS_KEY_ID": "test", "AWS_SECRET_ACCESS_KEY": "test", "AWS_SESSION_TOKEN": "",
		"AWS_PROFILE": "", "AWS_CONFIG_FILE": none, "AWS_SHARED_CREDENTIALS_FILE": none, "AWS_EC2_METADATA_DISABLED": "true",
	} {
		t.Setenv(k, v)
	}
	return srv
}

// TestMoveBetweenLocalAndAWSKMS checks the moves of the sealed values from the local KEK to AWS
// KMS and back with `my kek rewrap`, with key_manager = awskms built from the configuration on a
// stand-in for AWS KMS: during each move the key manager of the other kind only unwraps.
func TestMoveBetweenLocalAndAWSKMS(t *testing.T) {
	srv := useAWSKMSStandIn(t)
	db := newDatabase(t, true)
	oldKEK, newKEK := randomKEK(t), randomKEK(t)

	signed := proxyURL(t, openApp(t, db, map[string]string{"MY_KEK": oldKEK}, nil, nil))

	// To AWS KMS: kms_key (an alias, resolved to the key ARN) wraps, and the local KEK in
	// kek_previous only unwraps.
	toKMS := openApp(t, db, map[string]string{"MY_KEY_MANAGER": "awskms", "MY_KMS_KEY": "alias/my-kek", "MY_KEK_PREVIOUS": oldKEK}, nil, nil)
	if got, want := toKMS.KeyManager.CurrentKeyID(), awskms.KeyID(testAWSKMSKey); got != want {
		t.Errorf("CurrentKeyID() = %q, want %q", got, want)
	}
	if got := proxyURL(t, toKMS); got != signed {
		t.Errorf("URL while moving to AWS KMS = %q, want %q", got, signed)
	}
	if r := rewrapSecrets(t, toKMS); r.Rewrapped != 1 || r.Failed != 0 {
		t.Errorf("rewrap to AWS KMS = %+v, want 1 rewrapped", r)
	}
	requireAllUnder(t, toKMS, awskms.Kind)
	calls := srv.Calls()
	kmsOnly := openApp(t, db, map[string]string{"MY_KEY_MANAGER": "awskms", "MY_KMS_KEY": testAWSKMSKey}, nil, nil)
	if got := proxyURL(t, kmsOnly); got != signed {
		t.Errorf("URL on AWS KMS alone = %q, want %q", got, signed)
	}
	if srv.Calls() == calls {
		t.Error("the App on AWS KMS alone did not call AWS KMS")
	}

	// Back to the local KEK: kek wraps, and kms_key only unwraps.
	toLocal := openApp(t, db, map[string]string{"MY_KEK": newKEK, "MY_KMS_KEY": "alias/my-kek"}, nil, nil)
	if got := kernel.KeyIDKind(toLocal.KeyManager.CurrentKeyID()); got != "local" {
		t.Errorf("CurrentKeyID() = %q, want a local key ID", toLocal.KeyManager.CurrentKeyID())
	}
	if got := proxyURL(t, toLocal); got != signed {
		t.Errorf("URL while moving back to the local KEK = %q, want %q", got, signed)
	}
	if r := rewrapSecrets(t, toLocal); r.Rewrapped != 1 || r.Failed != 0 {
		t.Errorf("rewrap to the local KEK = %+v, want 1 rewrapped", r)
	}
	requireAllUnder(t, toLocal, "local")
	if got := proxyURL(t, openApp(t, db, map[string]string{"MY_KEK": newKEK}, nil, nil)); got != signed {
		t.Errorf("URL on the local KEK alone = %q, want %q", got, signed)
	}
}
