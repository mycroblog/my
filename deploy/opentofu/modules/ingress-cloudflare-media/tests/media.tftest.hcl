# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider, without credentials. The provider still tries its dry run of a ruleset at plan time, which Cloudflare
# refuses as a warning, so these tests do not show that Cloudflare accepts the rules.

mock_provider "cloudflare" {}

variables {
  zone_id     = "0123456789abcdef0123456789abcdef"
  host        = "media.example.org"
  origin_host = "ghs.googlehosted.com"
}

run "follows_the_cdn_guide" {
  command = plan

  assert {
    condition     = tomap(output.env) == tomap({ MY_MEDIA_ORIGIN = "https://media.example.org" })
    error_message = "the module must return the media origin, and no trust"
  }

  assert {
    condition = (
      output.rules.cache[0].action_parameters.cache &&
      output.rules.cache[0].action_parameters.edge_ttl.mode == "respect_origin" &&
      output.rules.cache[0].action_parameters.browser_ttl.mode == "respect_origin" &&
      !can(output.rules.cache[0].action_parameters.edge_ttl.default)
    )
    error_message = "the origin's Cache-Control must decide how long"
  }

  assert {
    condition     = !can(output.rules.cache[0].action_parameters.cache_key)
    error_message = "the default cache key holds the query string, and sig with it: a custom key must not change that"
  }

  assert {
    condition     = strcontains(output.rules.cache[0].expression, "starts_with(http.request.uri.path, \"/media/\")")
    error_message = "the rule must cover /media/ and /media/proxy/"
  }

  assert {
    condition     = output.rules.firewall[0].action == "block" && strcontains(output.rules.firewall[0].expression, "not starts_with(http.request.uri.path, \"/media/\")")
    error_message = "every other path on this host must be refused"
  }

  assert {
    condition     = cloudflare_ruleset.cache != null && cloudflare_ruleset.firewall != null && cloudflare_tiered_cache.smart.value == "on"
    error_message = "alone, the module must create what it needs"
  }
}

run "beside_the_app_module_the_app_owns_the_rulesets" {
  command = plan

  variables {
    manage_rulesets = false
  }

  assert {
    condition     = cloudflare_ruleset.cache == null && cloudflare_ruleset.config == null && cloudflare_ruleset.firewall == null && length(output.rules.cache) == 1
    error_message = "a zone has one ruleset per phase"
  }
}

run "a_run_app_origin_without_a_rewrite_fails" {
  command = plan

  variables {
    origin_host = "my-abc-an.a.run.app"
  }

  expect_failures = [cloudflare_dns_record.host]
}

run "a_rewrite_routes_to_the_origin" {
  command = plan

  variables {
    origin_host         = "my-abc-an.a.run.app"
    rewrite_host_header = true
  }

  assert {
    condition     = output.rules.origin[0].action_parameters.host_header == "my-abc-an.a.run.app" && cloudflare_dns_record.host.type == "CNAME"
    error_message = "the rule must send the origin's Host"
  }
}

run "beside_the_app_module_it_names_nothing_of_the_secret" {
  command = plan

  variables {
    manage_rulesets = false
  }

  assert {
    condition     = one(output.rules.transform) == "http.host eq \"media.example.org\"" && cloudflare_ruleset.transform == null && length(output.secret_env) == 0 && !contains(keys(output.env), "MY_ORIGIN_SECRET_HEADER")
    error_message = "the app module holds the one secret and names the settings"
  }
}

run "alone_it_can_name_the_settings" {
  command = plan

  variables {
    origin_secret    = "0123456789abcdef0123456789abcdef"
    origin_secret_id = "my-origin-secret"
  }

  assert {
    condition     = output.secret_env["MY_ORIGIN_SECRET"] == "my-origin-secret" && output.env["MY_ORIGIN_SECRET_HEADER"] == "x-my-origin-secret" && cloudflare_ruleset.transform.phase == "http_request_late_transform"
    error_message = "the rule and the settings go together"
  }
}

run "beside_a_module_that_names_the_settings_it_only_sends_the_header" {
  command = plan

  variables {
    origin_secret = "0123456789abcdef0123456789abcdef"
  }

  assert {
    condition     = length(output.secret_env) == 0 && !contains(keys(output.env), "MY_ORIGIN_SECRET_HEADER") && cloudflare_ruleset.transform.phase == "http_request_late_transform"
    error_message = "two modules must not name the settings twice"
  }
}

run "a_second_secret_beside_the_app_module_fails" {
  command = plan

  variables {
    manage_rulesets = false
    origin_secret   = "0123456789abcdef0123456789abcdef"
  }

  expect_failures = [cloudflare_dns_record.host]
}

run "settings_without_the_header_fail" {
  command = plan

  variables {
    origin_secret_id = "my-origin-secret"
  }

  expect_failures = [cloudflare_dns_record.host]
}
