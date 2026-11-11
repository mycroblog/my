# SPDX-License-Identifier: AGPL-3.0-only

# What does not fit together fails before the plan, on the input whose message names the others.

mock_provider "google" {
  mock_data "google_client_config" {
    defaults = {
      project = "my-project"
      region  = "asia-northeast1"
    }
  }
  mock_data "google_project" {
    defaults = {
      number = "123456789012"
    }
  }
}

mock_provider "random" {}

mock_provider "cloudflare" {}

variables {
  host  = "social.example.org"
  image = "ghcr.io/mycroblog/my:0.1.0"
  media = { bucket_name = "my-project-my-media" }
  mail  = { from = "noreply@social.example.org", smtp = { host = "smtp.example.org", username = "my-smtp-user" } }
}

run "a_name_that_cannot_name_things" {
  command = plan

  variables {
    name = "My_Instance"
  }

  expect_failures = [var.name]
}

run "an_unknown_kind_of_ingress" {
  command = plan

  variables {
    ingress = { kind = "nginx" }
  }

  expect_failures = [var.ingress]
}

run "an_unknown_kind_of_database" {
  command = plan

  variables {
    database = { kind = "neon" }
  }

  expect_failures = [var.database]
}

run "an_unknown_kind_of_queue" {
  command = plan

  variables {
    queue = { kind = "sqs" }
  }

  expect_failures = [var.queue]
}

run "an_unknown_kind_of_kv" {
  command = plan

  variables {
    kv = { kind = "redis" }
  }

  expect_failures = [var.kv]
}

run "an_unknown_kind_of_stream" {
  command = plan

  variables {
    stream = { kind = "hub" }
  }

  expect_failures = [var.stream]
}

run "an_unknown_kind_of_kek" {
  command = plan

  variables {
    kek = { kind = "local" }
  }

  expect_failures = [var.kek]
}

run "an_unknown_kind_of_network" {
  command = plan

  variables {
    network = { kind = "default" }
  }

  expect_failures = [var.network]
}

run "an_unknown_kind_of_media" {
  command = plan

  variables {
    media = { kind = "local", bucket_name = "b" }
  }

  expect_failures = [var.media]
}

run "an_existing_network_without_its_names" {
  command = plan

  variables {
    network = { kind = "existing" }
  }

  expect_failures = [var.network]
}

run "nat_without_a_vpc_of_this_module" {
  command = plan

  variables {
    network = { nat = true }
  }

  expect_failures = [var.network]
}

run "cloudflare_ingress_without_cloudflare" {
  command = plan

  variables {
    ingress = { kind = "cloudflare" }
  }

  expect_failures = [var.ingress]
}

run "a_media_host_without_cloudflare" {
  command = plan

  variables {
    ingress = { media_host = "media.example.org" }
  }

  expect_failures = [var.ingress]
}

run "a_media_host_that_is_the_host" {
  command = plan

  variables {
    ingress    = { media_host = "social.example.org" }
    cloudflare = { zone_id = "0123456789abcdef0123456789abcdef" }
  }

  expect_failures = [var.ingress]
}

run "a_pooled_cloud_sql" {
  command = plan

  variables {
    database = { kind = "cloud-sql", pooled = true }
  }

  expect_failures = [var.database]
}

run "queue_db_on_a_service_that_scales_to_zero" {
  command = plan

  variables {
    queue = { kind = "db" }
  }

  expect_failures = [var.queue]
}

run "queue_db_without_cpu_always_allocated" {
  command = plan

  variables {
    queue   = { kind = "db" }
    service = { min_instances = 1 }
  }

  expect_failures = [var.queue]
}

run "s3_without_an_endpoint" {
  command = plan

  variables {
    media = { kind = "s3", bucket_name = "my-media" }
  }

  expect_failures = [var.media]
}

run "r2_without_cloudflare" {
  command = plan

  variables {
    media = { kind = "r2", bucket_name = "my-media" }
  }

  expect_failures = [var.media]
}

run "memorystore_without_a_vpc" {
  command = plan

  variables {
    kv = { kind = "memorystore" }
  }

  expect_failures = [var.kv]
}

run "the_fields_of_an_external_valkey_on_another_kind" {
  command = plan

  variables {
    kv = { tls_ca = true }
  }

  expect_failures = [var.kv]
}

run "the_valkey_stream_without_a_valkey" {
  command = plan

  variables {
    stream = { kind = "valkey" }
  }

  expect_failures = [var.stream]
}
