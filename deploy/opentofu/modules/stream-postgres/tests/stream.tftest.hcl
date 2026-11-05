# SPDX-License-Identifier: AGPL-3.0-only

# The module has no provider: nothing here reaches a database.

run "on_the_url_of_the_database" {
  command = plan

  assert {
    condition     = output.env == { MY_STREAM = "postgres" } && length(output.secret_env) == 0
    error_message = "with no URL of its own, the stream must name only the adapter"
  }
}

run "on_a_direct_url" {
  command = plan

  variables {
    listen_url_secret_id = "direct-url"
  }

  assert {
    condition     = output.secret_env == tomap({ MY_STREAM_POSTGRES_URL = "direct-url" })
    error_message = "the direct URL must arrive by reference"
  }
}
