# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches Google Cloud.

mock_provider "google" {}

variables {
  project_id = "my-project"
}

run "every_signal_to_google_cloud" {
  command = plan

  assert {
    condition     = output.env == { MY_METRICS_EXPORTER = "none", MY_TRACES_EXPORTER = "none", MY_ERRORS_EXPORTER = "none" }
    error_message = "the server exports nothing itself: Cloud Run delivers to the destinations"
  }

  assert {
    condition = (
      toset([for s in google_project_service.destination : s.service]) == toset(["monitoring.googleapis.com", "cloudtrace.googleapis.com", "clouderrorreporting.googleapis.com"]) &&
      alltrue([for s in google_project_service.destination : s.project == "my-project" && !s.disable_on_destroy])
    )
    error_message = "the API of each destination must be enabled in the project, and stay enabled when the module goes"
  }

  assert {
    condition     = length(output.permissions) == 0 && length(output.secret_env) == 0
    error_message = "the destinations need no grant to the server"
  }
}

run "errors_left_to_the_settings" {
  command = plan

  variables {
    errors = "settings"
  }

  assert {
    condition     = !contains(keys(output.env), "MY_ERRORS_EXPORTER") && !contains(keys(google_project_service.destination), "errors")
    error_message = "a signal left to the settings is set nowhere here"
  }
}

run "every_signal_left_to_the_settings" {
  command = plan

  variables {
    metrics = "settings"
    traces  = "settings"
    errors  = "settings"
  }

  assert {
    condition     = output.env == {} && length(google_project_service.destination) == 0
    error_message = "nothing is enabled or set"
  }
}

run "an_unknown_destination" {
  command = plan

  variables {
    traces = "otlp"
  }

  expect_failures = [var.traces]
}
