# SPDX-License-Identifier: AGPL-3.0-only

run "it_is_settings_only" {
  command = plan

  assert {
    condition     = output.env == { MY_QUEUE = "db" } && output.secret_env == {} && output.permissions == []
    error_message = "the queue is in the database: nothing to create, read, or grant"
  }
}
