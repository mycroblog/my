# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches Google Cloud.

mock_provider "google" {
  mock_data "google_project" {
    defaults = {
      number = "123456789012"
    }
  }
}

variables {
  project_id = "p"
  region     = "asia-northeast1"
}

run "jobs_call_the_services_own_url" {
  command = plan

  assert {
    condition     = output.env["MY_QUEUE_CLOUDTASKS_URL"] == "https://my-123456789012.asia-northeast1.run.app"
    error_message = "tasks call the default URL, which Cloud Run counts as internal"
  }

  assert {
    condition     = google_cloud_scheduler_job.tick.http_target[0].uri == "https://my-123456789012.asia-northeast1.run.app/_internal/tick"
    error_message = "the tick calls the same URL"
  }

  assert {
    condition     = google_cloud_scheduler_job.tick.http_target[0].oidc_token[0].audience == null
    error_message = "no audience: it defaults to the URI, which is what the server requires while queue_cloudtasks_audience is unset"
  }
}

run "a_name_that_leaves_no_deterministic_url_fails" {
  command = plan

  variables {
    service_name = "a-service-name-long-enough-to-leave-no-deterministic-url-at-all"
  }

  expect_failures = [google_cloud_tasks_queue.jobs]
}
