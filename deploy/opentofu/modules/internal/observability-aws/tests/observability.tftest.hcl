# SPDX-License-Identifier: AGPL-3.0-only

# The module has no resource, so a plan checks its outputs alone.

run "metrics_and_errors_to_cloudwatch_and_traces_to_x_ray" {
  command = plan

  assert {
    condition = jsonencode(output.env) == jsonencode({
      MY_METRICS_EXPORTER                = "emf"
      MY_METRICS_EMF_NAMESPACE           = "my"
      MY_TRACES_EXPORTER                 = "otlp"
      OTEL_EXPORTER_OTLP_TRACES_ENDPOINT = "http://127.0.0.1:4318/v1/traces"
      MY_ERRORS_EXPORTER                 = "none"
    })
    error_message = "the metrics must go to CloudWatch in EMF, the spans to the collector, and the error reports stay on their log line: ${jsonencode(output.env)}"
  }

  assert {
    condition     = jsonencode(output.permissions) == jsonencode([{ permission = "xray:PutTraceSegments", resource = "*" }]) && length(output.secret_env) == 0
    error_message = "the collector writes the spans to X-Ray with the task role, and nothing else needs a grant"
  }

  assert {
    condition = (
      length(output.sidecars) == 1 && output.sidecars[0].name == "otel-collector" &&
      startswith(output.sidecars[0].image, "public.ecr.aws/aws-observability/aws-otel-collector:v") &&
      can(regex("@sha256:[0-9a-f]{64}$", output.sidecars[0].image))
    )
    error_message = "the collector runs beside the server, from an image pinned by version and digest"
  }

  assert {
    condition = (
      yamldecode(output.sidecars[0].env["AOT_CONFIG_CONTENT"]).receivers.otlp.protocols.http.endpoint == "127.0.0.1:4318" &&
      yamldecode(output.sidecars[0].env["AOT_CONFIG_CONTENT"]).service.pipelines.traces.exporters == ["awsxray"] &&
      !contains(keys(yamldecode(output.sidecars[0].env["AOT_CONFIG_CONTENT"]).service.pipelines), "metrics")
    )
    error_message = "the collector takes OTLP over HTTP on the loopback the server sends to, and sends the traces alone to X-Ray"
  }
}

run "traces_to_none" {
  command = plan

  variables {
    traces = "none"
  }

  assert {
    condition     = output.env["MY_TRACES_EXPORTER"] == "none" && length(output.sidecars) == 0 && length(output.permissions) == 0 && !contains(keys(output.env), "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
    error_message = "with no traces, no collector runs and nothing needs a grant"
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
    condition     = length(output.env) == 0 && length(output.sidecars) == 0 && length(output.permissions) == 0
    error_message = "nothing is set, and no collector runs"
  }
}

run "traces_to_cloud_trace" {
  command = plan

  variables {
    traces = "cloud-trace"
  }

  expect_failures = [var.traces]
}

run "a_collector_of_the_latest_image" {
  command = plan

  variables {
    collector_image = "public.ecr.aws/aws-observability/aws-otel-collector:latest"
  }

  expect_failures = [var.collector_image]
}

run "a_namespace_of_aws" {
  command = plan

  variables {
    metrics_emf_namespace = "AWS/ECS"
  }

  expect_failures = [var.metrics_emf_namespace]
}
