# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches Google Cloud. By plan alone: prevent_destroy refuses the destroy that follows
# an apply.

mock_provider "google" {}

variables {
  project_id = "my-project"
  location   = "us-central1"
}

run "it_creates_a_key_for_encryption_in_its_ring" {
  command = plan

  assert {
    condition     = google_kms_key_ring.this.name == "my" && google_kms_key_ring.this.location == "us-central1" && google_kms_crypto_key.kek.name == "my-kek" && google_kms_crypto_key.kek.purpose == "ENCRYPT_DECRYPT"
    error_message = "the module must create the key, for encryption, in its ring"
  }

  assert {
    condition     = output.env == { MY_KEY_MANAGER = "gcpkms", MY_KMS_KEY = "projects/my-project/locations/us-central1/keyRings/my/cryptoKeys/my-kek" }
    error_message = "the server must take the key manager and the resource name of the key, known at plan time"
  }

  assert {
    condition     = output.permissions == [{ permission = "roles/cloudkms.cryptoKeyEncrypterDecrypter", scope = "crypto_key", resource = "projects/my-project/locations/us-central1/keyRings/my/cryptoKeys/my-kek" }]
    error_message = "the service account must encrypt and decrypt with this key alone"
  }

  assert {
    condition     = output.platform == "gcp" && output.secret_env == {}
    error_message = "the server authenticates with its service account"
  }
}

run "the_names_come_from_the_variables" {
  command = plan

  variables {
    key_ring = "ring"
    key      = "kek"
  }

  assert {
    condition     = output.env["MY_KMS_KEY"] == "projects/my-project/locations/us-central1/keyRings/ring/cryptoKeys/kek"
    error_message = "the key must take the names given"
  }
}

run "a_key_made_elsewhere_creates_nothing" {
  command = plan

  variables {
    crypto_key_id = "projects/other/locations/global/keyRings/theirs/cryptoKeys/kek"
  }

  assert {
    condition     = google_kms_key_ring.this == null && google_kms_crypto_key.kek == null && output.env["MY_KMS_KEY"] == var.crypto_key_id && output.permissions[0].resource == var.crypto_key_id
    error_message = "a key made elsewhere must be used as it is"
  }
}

run "a_key_with_a_version_fails" {
  command = plan

  variables {
    crypto_key_id = "projects/other/locations/global/keyRings/theirs/cryptoKeys/kek/cryptoKeyVersions/1"
  }

  expect_failures = [var.crypto_key_id]
}
