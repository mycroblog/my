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

run "no_workers_is_one_queue" {
  command = plan

  assert {
    condition     = length(google_cloud_tasks_queue.worker) == 0 && !contains(keys(output.env), "MY_QUEUE_CLOUDTASKS_ROUTES") && !contains(keys(output.env), "MY_QUEUE_CLOUDTASKS_AUDIENCE") && length(output.permissions) == 2 && length(output.workers) == 0
    error_message = "no workers must leave the queue as it is"
  }
}

run "a_worker_is_a_queue_and_routes_from_its_key" {
  command = plan

  variables {
    workers = {
      delivery = { job_types = ["federation.fanout", "federation.deliver"], max_concurrent_dispatches = 50, max_dispatches_per_second = 100 }
    }
  }

  assert {
    condition     = google_cloud_tasks_queue.worker["delivery"].name == "my-jobs-delivery" && google_cloud_tasks_queue.worker["delivery"].rate_limits[0].max_concurrent_dispatches == 50 && google_cloud_tasks_queue.worker["delivery"].rate_limits[0].max_dispatches_per_second == 100
    error_message = "the queue must take its name from the key and its rates from the worker"
  }

  assert {
    condition     = output.env["MY_QUEUE_CLOUDTASKS_ROUTES"] == "federation.deliver=projects/p/locations/asia-northeast1/queues/my-jobs-delivery@https://my-delivery-123456789012.asia-northeast1.run.app,federation.fanout=projects/p/locations/asia-northeast1/queues/my-jobs-delivery@https://my-delivery-123456789012.asia-northeast1.run.app"
    error_message = "each job type must route to the worker's queue and the deterministic URL of its service"
  }

  assert {
    condition     = output.env["MY_QUEUE_CLOUDTASKS_AUDIENCE"] == "https://my-123456789012.asia-northeast1.run.app" && google_cloud_scheduler_job.tick.http_target[0].oidc_token[0].audience == output.env["MY_QUEUE_CLOUDTASKS_AUDIENCE"]
    error_message = "every process and the tick must use one audience"
  }

  assert {
    condition     = output.env["MY_QUEUE_CLOUDTASKS_QUEUE"] == "projects/p/locations/asia-northeast1/queues/my-jobs" && output.env["MY_QUEUE_CLOUDTASKS_URL"] == "https://my-123456789012.asia-northeast1.run.app"
    error_message = "what no route names must stay on the default"
  }

  assert {
    condition     = length(output.permissions) == 3 && output.permissions[2].resource == "projects/p/locations/asia-northeast1/queues/my-jobs-delivery"
    error_message = "the server must be able to enqueue on the worker's queue"
  }

  assert {
    condition     = output.workers["delivery"] == tolist(["federation.deliver", "federation.fanout"])
    error_message = "the app must be able to compare what the queue was given"
  }
}

run "a_split_fails_here_too" {
  command = plan

  variables {
    workers = {
      delivery = { job_types = ["federation.deliver"] }
    }
  }

  expect_failures = [google_cloud_tasks_queue.jobs]
}

run "a_field_only_a_service_with_tasks_has_fails" {
  command = plan

  variables {
    workers = {
      delivery = { job_types = ["federation.*"], instances = 2 }
    }
  }

  expect_failures = [google_cloud_tasks_queue.jobs]
}

run "a_worker_whose_name_leaves_no_deterministic_url_fails" {
  command = plan

  variables {
    service_name = "a-service-name-long-enough-to-leave-no-url"
    workers = {
      delivery = { job_types = ["federation.*"] }
    }
  }

  expect_failures = [google_cloud_tasks_queue.worker]
}
