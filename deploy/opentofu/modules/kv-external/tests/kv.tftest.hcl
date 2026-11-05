# SPDX-License-Identifier: AGPL-3.0-only

# The module has no provider: nothing here reaches a server.

variables {
  url_secret_id = "valkey-url"
}

run "the_url_is_a_reference" {
  command = plan

  assert {
    condition     = output.env == { MY_KV = "valkey" } && output.secret_env == { MY_VALKEY_URL = "valkey-url" } && length(output.secret_files) == 0
    error_message = "with no CA and no prefix, the module names only the adapter and the URL"
  }
}

run "the_ca_is_a_file_and_the_prefix_a_setting" {
  command = plan

  variables {
    tls_ca_secret_id = "valkey-ca"
    key_prefix       = "social:"
  }

  assert {
    condition     = output.secret_files == tomap({ MY_VALKEY_TLS_CA_FILE = "valkey-ca" }) && output.env["MY_VALKEY_KEY_PREFIX"] == "social:"
    error_message = "the CA must be mounted and the prefix must reach the settings"
  }
}

run "a_blank_prefix_fails" {
  command = plan

  variables {
    key_prefix = " "
  }

  expect_failures = [var.key_prefix]
}
