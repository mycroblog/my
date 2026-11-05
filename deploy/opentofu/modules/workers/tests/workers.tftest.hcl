# SPDX-License-Identifier: AGPL-3.0-only

# What one value of workers means. It makes nothing, so these run on no provider at all.

variables {
  fields = ["max_instances", "concurrency"]
}

run "empty_is_nothing" {
  command = plan

  assert {
    condition     = length(output.problems) == 0 && length(output.workers) == 0 && length(output.all_patterns) == 0
    error_message = "an empty value must say nothing"
  }
}

run "a_worker_reads_with_every_field" {
  command = plan

  variables {
    workers = {
      delivery = {
        job_types     = ["federation.fanout", "federation.deliver"]
        max_instances = 20
        concurrency   = 16
      }
    }
  }

  assert {
    condition     = length(output.problems) == 0
    error_message = "the example of the contract must pass: ${join(" ", output.problems)}"
  }

  assert {
    condition     = output.workers["delivery"].max_instances == 20 && output.workers["delivery"].cpu == null
    error_message = "a field that is not named must be null"
  }

  assert {
    condition     = output.all_patterns == tolist(["federation.deliver", "federation.fanout"])
    error_message = "the patterns must be sorted, so that a change of order changes no setting"
  }
}

run "a_prefix_covers_its_types_so_it_keeps_them_together" {
  command = plan

  variables {
    workers = {
      federation = { job_types = ["federation.*"] }
    }
  }

  assert {
    condition     = length(output.problems) == 0
    error_message = "federation.* covers both the fanout and the deliveries: ${join(" ", output.problems)}"
  }
}

run "types_that_must_stay_together_may_not_split" {
  command = plan

  variables {
    workers = {
      delivery = { job_types = ["federation.deliver"] }
    }
  }

  assert {
    condition     = length(output.problems) == 1 && strcontains(output.problems[0], "federation.fanout and federation.deliver must stay on one worker") && strcontains(output.problems[0], "federation.fanout on the default")
    error_message = "a split from the default must be refused, and say where each is"
  }
}

run "types_that_must_stay_together_may_not_split_across_workers" {
  command = plan

  variables {
    workers = {
      a = { job_types = ["voice.apply"] }
      b = { job_types = ["voice.apply_media_event"] }
    }
  }

  assert {
    condition     = length(output.problems) == 1 && strcontains(output.problems[0], "must stay on one worker")
    error_message = "a split between two workers must be refused"
  }
}

run "a_prefix_takes_both_of_a_pair" {
  command = plan

  variables {
    workers = {
      a = { job_types = ["supporters.*"] }
      b = { job_types = ["notification.push"] }
    }
  }

  assert {
    condition     = length(output.problems) == 0
    error_message = "the prefix takes both of the pair, so nothing splits: ${join(" ", output.problems)}"
  }
}

run "a_type_on_two_workers_fails" {
  command = plan

  variables {
    workers = {
      a = { job_types = ["federation.*"] }
      b = { job_types = ["federation.inbox"] }
    }
  }

  assert {
    condition     = length(output.problems) == 1 && strcontains(output.problems[0], "federation.inbox is on two workers")
    error_message = "a prefix over another worker's name must be refused"
  }
}

run "a_name_that_is_no_job_type_fails" {
  command = plan

  variables {
    workers = {
      a = { job_types = ["federation.nothing"] }
      b = { job_types = ["nothing.*"] }
    }
  }

  assert {
    condition     = length(output.problems) == 2 && alltrue([for p in output.problems : strcontains(p, "matches no job type")])
    error_message = "a name and a prefix that match nothing must be refused"
  }
}

run "no_job_types_fails" {
  command = plan

  variables {
    workers = {
      a = { max_instances = 2 }
    }
  }

  assert {
    condition     = length(output.problems) == 1 && strcontains(output.problems[0], "has no job_types")
    error_message = "a worker with nothing to do must be refused"
  }
}

run "a_field_nobody_reads_fails" {
  command = plan

  variables {
    workers = {
      a = { job_types = ["webhook.deliver"], max_instance = 2 }
    }
  }

  assert {
    condition     = length(output.problems) == 1 && strcontains(output.problems[0], "max_instance, which is no field of workers")
    error_message = "a typo must not vanish"
  }
}

run "a_field_of_another_platform_fails" {
  command = plan

  variables {
    workers = {
      a = { job_types = ["webhook.deliver"], instances = 2 }
    }
  }

  assert {
    condition     = length(output.problems) == 1 && strcontains(output.problems[0], "instances, which this platform and queue do not have")
    error_message = "a field that only another platform has must be refused"
  }
}

run "a_key_that_cannot_name_things_fails" {
  command = plan

  variables {
    workers = {
      Delivery = { job_types = ["webhook.deliver"] }
      migrate  = { job_types = ["notification.push"] }
    }
  }

  assert {
    condition     = length(output.problems) == 2 && alltrue([for p in output.problems : strcontains(p, "is not a usable key of workers")])
    error_message = "a key with upper case, and one taken by the migration, must be refused"
  }
}

run "a_concurrency_out_of_range_fails" {
  command = plan

  variables {
    workers = {
      a = { job_types = ["webhook.deliver"], concurrency = 0 }
      b = { job_types = ["notification.push"], concurrency = 129 }
      c = { job_types = ["notification.email"], concurrency = 1.5 }
      d = { job_types = ["notification.moderator_email"], concurrency = 128 }
    }
  }

  assert {
    condition     = length(output.problems) == 3 && alltrue([for p in output.problems : strcontains(p, "from 1 to 128")])
    error_message = "what the server would refuse at start must fail at plan, and 128 must pass"
  }
}
