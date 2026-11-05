# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches Neon. By plan alone: prevent_destroy refuses the destroy that follows an apply.

mock_provider "neon" {}

variables {
  org_id        = "org-1"
  region_id     = "aws-ap-southeast-1"
  url_secret_id = "direct"
}

run "the_project_takes_what_the_free_plan_takes" {
  command = plan

  assert {
    condition     = neon_project.db.org_id == "org-1"
    error_message = "the organization must be stated"
  }

  assert {
    condition     = neon_project.db.history_retention_seconds == 21600
    error_message = "the provider's default of a day is above what the Free plan allows, so the default here must be its limit"
  }

  # The attribute itself is computed, so the mock fills it in: only the input says what the module sends.
  assert {
    condition     = var.suspend_timeout_seconds == null
    error_message = "the Free plan does not take suspend_timeout_seconds, so the default must send nothing"
  }

  assert {
    condition     = neon_project.db.store_password == "yes"
    error_message = "the provider reads the password from Neon, which must have stored it"
  }
}

run "a_direct_url_alone_serves_everything" {
  command = plan

  assert {
    condition     = output.secret_env == tomap({ MY_DATABASE_URL = "direct" })
    error_message = "with no pooled secret the service uses the direct one"
  }

  assert {
    condition     = length(output.direct_secret_env) == 0
    error_message = "with no pooled secret nothing differs for one-off work"
  }
}

run "a_pooled_url_serves_the_service_and_the_direct_one_the_rest" {
  command = plan

  variables {
    pooled_url_secret_id = "pooled"
  }

  assert {
    condition     = output.secret_env == tomap({ MY_DATABASE_URL = "pooled", MY_STREAM_POSTGRES_URL = "direct" })
    error_message = "the service uses the pooled URL, and LISTEN the direct one"
  }

  assert {
    condition     = output.direct_secret_env == tomap({ MY_DATABASE_URL = "direct" })
    error_message = "one-off work uses the direct URL"
  }
}

run "a_paid_plan_can_set_the_suspend_timeout" {
  command = plan

  variables {
    suspend_timeout_seconds = 600
  }

  assert {
    condition     = neon_project.db.suspend_timeout_seconds == 600
    error_message = "the input must reach the project"
  }
}

run "no_organization_fails" {
  command = plan

  variables {
    org_id = ""
  }

  expect_failures = [var.org_id]
}
