# SPDX-License-Identifier: AGPL-3.0-only

# The module has no provider: nothing here reaches a server.

run "beside_a_kv_module_it_names_no_url" {
  command = plan

  variables {
    url_secret_id = null
  }

  assert {
    condition     = output.env == { MY_STREAM = "valkey" } && length(output.secret_env) == 0 && length(output.secret_files) == 0
    error_message = "the kv module names the URL, and a second name would fail the plan"
  }
}

run "alone_it_names_the_url" {
  command = plan

  variables {
    url_secret_id    = "valkey-url"
    tls_ca_secret_id = "valkey-ca"
  }

  assert {
    condition     = output.secret_env == tomap({ MY_VALKEY_URL = "valkey-url" }) && output.secret_files == tomap({ MY_VALKEY_TLS_CA_FILE = "valkey-ca" })
    error_message = "alone, the module must name the URL and the CA"
  }
}

run "a_ca_without_a_url_fails" {
  command = plan

  variables {
    url_secret_id    = null
    tls_ca_secret_id = "valkey-ca"
  }

  expect_failures = [var.tls_ca_secret_id]
}
