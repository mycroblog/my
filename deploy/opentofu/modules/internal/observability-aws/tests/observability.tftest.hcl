# SPDX-License-Identifier: AGPL-3.0-only

# The module has no resource, so a plan checks its outputs alone.

run "every_signal_to_cloudwatch" {
  command = plan

  assert {
    condition     = jsonencode(output.env) == jsonencode({ MY_METRICS_EXPORTER = "emf", MY_METRICS_EMF_NAMESPACE = "my", MY_TRACES_EXPORTER = "none", MY_ERRORS_EXPORTER = "none" })
    error_message = "the metrics must go to CloudWatch in EMF, and the error reports stay on their log line: ${jsonencode(output.env)}"
  }

  assert {
    condition     = length(output.permissions) == 0 && length(output.secret_env) == 0
    error_message = "the log driver needs no grant to the server"
  }
}

run "a_namespace_of_its_own" {
  command = plan

  variables {
    metrics_emf_namespace = "social"
  }

  assert {
    condition     = output.env["MY_METRICS_EMF_NAMESPACE"] == "social"
    error_message = "the namespace must reach the server"
  }
}

run "errors_left_to_the_settings" {
  command = plan

  variables {
    errors = "settings"
  }

  assert {
    condition     = !contains(keys(output.env), "MY_ERRORS_EXPORTER") && output.env["MY_METRICS_EXPORTER"] == "emf"
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
    condition     = length(output.env) == 0
    error_message = "nothing is set"
  }
}

run "traces_to_x_ray" {
  command = plan

  variables {
    traces = "x-ray"
  }

  expect_failures = [var.traces]
}

run "a_namespace_of_aws" {
  command = plan

  variables {
    metrics_emf_namespace = "AWS/ECS"
  }

  expect_failures = [var.metrics_emf_namespace]
}
