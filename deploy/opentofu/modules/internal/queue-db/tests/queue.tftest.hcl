# SPDX-License-Identifier: AGPL-3.0-only

run "it_is_settings_only" {
  command = plan

  assert {
    condition     = output.env == { MY_QUEUE = "db" } && output.secret_env == {} && output.permissions == []
    error_message = "the queue is in the database: nothing to create, read, or grant"
  }
}

run "no_workers_needs_nothing" {
  command = plan

  assert {
    condition     = length(output.workers) == 0 && output.env == { MY_QUEUE = "db" }
    error_message = "the queue is settings only"
  }
}

run "workers_add_no_setting_and_no_queue" {
  command = plan

  variables {
    workers = {
      delivery = { job_types = ["federation.*"], concurrency = 16 }
    }
  }

  assert {
    condition     = output.env == { MY_QUEUE = "db" } && output.workers["delivery"] == tolist(["federation.*"])
    error_message = "the app sets worker_job_types, and the queue only says what it was given"
  }
}

run "a_split_fails_here_too" {
  command = plan

  variables {
    workers = {
      delivery = { job_types = ["federation.deliver"] }
    }
  }

  expect_failures = [output.workers]
}

run "the_rates_of_a_queue_that_is_not_there_fail" {
  command = plan

  variables {
    workers = {
      delivery = { job_types = ["federation.*"], max_dispatches_per_second = 5 }
    }
  }

  expect_failures = [output.workers]
}
