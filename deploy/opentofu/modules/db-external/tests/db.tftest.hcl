# SPDX-License-Identifier: AGPL-3.0-only

variables {
  url_secret_id = "direct"
}

run "a_direct_url_alone_serves_everything" {
  command = plan

  assert {
    condition     = output.secret_env == tomap({ MY_DATABASE_URL = "direct" })
    error_message = "with no pooled URL the service uses the direct one"
  }

  assert {
    condition     = length(output.direct_secret_env) == 0
    error_message = "with no pooled URL nothing differs for one-off work"
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
