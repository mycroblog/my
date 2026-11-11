# SPDX-License-Identifier: AGPL-3.0-only

# The checks of the settings and secrets inputs. The module makes nothing, so these run on no provider at all.

variables {
  name     = "my"
  managed  = ["database_url", "queue", "blob_gcs_bucket"]
  excluded = { listen = "the port is fixed", blob_fs_dir = "no disk" }
}

run "nothing_is_nothing" {
  command = plan

  assert {
    condition     = length(output.env) == 0 && length(output.secrets) == 0
    error_message = "empty inputs must return nothing"
  }
}

run "valid_input_returns_the_environment_and_the_secrets" {
  command = plan

  variables {
    settings = {
      translator_deepl_url = "https://api.deepl.com"
      account_domain       = "example.com"
    }
    secrets = ["translator_deepl_api_key", "errors_sentry_dsn"]
  }

  assert {
    condition = output.env == tomap({
      MY_TRANSLATOR_DEEPL_URL = "https://api.deepl.com"
      MY_ACCOUNT_DOMAIN       = "example.com"
    })
    error_message = "env must map each MY_ name to its value: ${jsonencode(output.env)}"
  }

  assert {
    condition = jsonencode(output.secrets) == jsonencode({
      errors_sentry_dsn        = { env = "MY_ERRORS_SENTRY_DSN", secret = "my-errors-sentry-dsn" }
      translator_deepl_api_key = { env = "MY_TRANSLATOR_DEEPL_API_KEY", secret = "my-translator-deepl-api-key" }
    })
    error_message = "secrets must name each MY_ variable and its secret <name>-<key>: ${jsonencode(output.secrets)}"
  }
}

run "settings_refuses_an_unknown_key" {
  command = plan

  variables {
    settings = { translater_deepl_url = "https://api.deepl.com" }
  }

  expect_failures = [var.settings]
}

run "settings_refuses_a_secret" {
  command = plan

  variables {
    settings = { translator_deepl_api_key = "plain" }
  }

  expect_failures = [var.settings]
}

run "settings_refuses_what_a_section_sets" {
  command = plan

  variables {
    settings = { queue = "db" }
  }

  expect_failures = [var.settings]
}

run "settings_refuses_what_the_module_does_not_take" {
  command = plan

  variables {
    settings = { listen = ":9090" }
  }

  expect_failures = [var.settings]
}

run "secrets_refuses_an_unknown_key" {
  command = plan

  variables {
    secrets = ["translator_deepl_apikey"]
  }

  expect_failures = [var.secrets]
}

run "secrets_refuses_a_setting_that_is_not_secret" {
  command = plan

  variables {
    secrets = ["translator_deepl_url"]
  }

  expect_failures = [var.secrets]
}

run "secrets_refuses_what_a_section_sets" {
  command = plan

  variables {
    secrets = ["database_url"]
  }

  expect_failures = [var.secrets]
}

run "secrets_refuses_what_the_module_does_not_take" {
  command = plan

  variables {
    secrets  = ["kek_previous"]
    excluded = { kek_previous = "rotated by hand" }
  }

  expect_failures = [var.secrets]
}
