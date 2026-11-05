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
    condition     = length(cloudflare_ruleset.cache) == 1 && length(cloudflare_ruleset.firewall) == 1 && cloudflare_tiered_cache.smart[0].value == "on"
    error_message = "alone, the module must create what it needs"
  }
}

run "beside_the_app_module_the_app_owns_the_rulesets" {
  command = plan

  variables {
    manage_rulesets = false
  }

  assert {
    condition     = length(cloudflare_ruleset.cache) == 0 && length(cloudflare_ruleset.config) == 0 && length(cloudflare_ruleset.firewall) == 0 && length(output.rules.cache) == 1
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
