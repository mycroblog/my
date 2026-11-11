# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider, without credentials. The provider still tries its dry run of a ruleset at plan time, which Cloudflare
# refuses as a warning, so these tests do not show that Cloudflare accepts the rules.

mock_provider "cloudflare" {
  mock_data "cloudflare_ip_ranges" {
    defaults = {
      ipv4_cidrs = ["173.245.48.0/20"]
      ipv6_cidrs = ["2400:cb00::/32"]
    }
  }
}

variables {
  zone_id     = "0123456789abcdef0123456789abcdef"
  host        = "social.example.org"
  origin_host = "ghs.googlehosted.com"

  origin_secret    = "0123456789abcdef0123456789abcdef"
  origin_secret_id = "my-origin-secret"
}

run "the_contract" {
  command = plan

  assert {
    condition     = output.trusted_proxies == tolist(["173.245.48.0/20", "2400:cb00::/32"]) && output.env["MY_TRUSTED_PROXY_HEADER"] == "x-forwarded-for"
    error_message = "the server must walk past Cloud Run's front end and Cloudflare"
  }

  assert {
    condition     = output.service_ingress == "all"
    error_message = "the service cannot be limited to Cloudflare"
  }

  assert {
    condition     = cloudflare_dns_record.host.proxied && cloudflare_dns_record.host.type == "CNAME" && cloudflare_dns_record.host.content == "ghs.googlehosted.com"
    error_message = "the record must be proxied to the origin"
  }

  assert {
    condition     = cloudflare_ruleset.cache.phase == "http_request_cache_settings" && !cloudflare_ruleset.cache.rules[0].action_parameters.cache
    error_message = "the cache must be bypassed for everything"
  }

  assert {
    condition     = cloudflare_ruleset.firewall == null && cloudflare_ruleset.origin == null
    error_message = "the module must add no challenge, and no origin rule unless asked"
  }

  assert {
    condition     = cloudflare_ruleset.config.rules[0].action_parameters.ssl == "strict" && !cloudflare_ruleset.config.rules[0].action_parameters.bic
    error_message = "the origin must be verified, and non-browsers let through"
  }
}

run "an_address_makes_an_a_record" {
  command = plan

  variables {
    origin_host = "203.0.113.7"
  }

  assert {
    condition     = cloudflare_dns_record.host.type == "A"
    error_message = "an address needs an A record"
  }
}

run "a_run_app_origin_without_a_rewrite_fails" {
  command = plan

  variables {
    origin_host = "my-abc-an.a.run.app"
  }

  expect_failures = [cloudflare_dns_record.host]
}

run "a_rewrite_is_an_origin_rule" {
  command = plan

  variables {
    origin_host         = "my-abc-an.a.run.app"
    rewrite_host_header = true
  }

  assert {
    condition     = cloudflare_ruleset.origin.rules[0].action == "route" && cloudflare_ruleset.origin.rules[0].action_parameters.host_header == "my-abc-an.a.run.app"
    error_message = "the origin rule must send the origin's Host"
  }
}

run "media_in_the_same_zone" {
  command = plan

  module {
    source = "../ingress-cloudflare-media"
  }

  variables {
    origin_secret    = null
    origin_secret_id = null
    host             = "media.example.org"
    manage_rulesets  = false
  }
}

run "shares_the_rulesets_with_media" {
  command = plan

  variables {
    extra_rules = {
      cache    = [{ description = "media", expression = "true", action = "set_cache_settings", action_parameters = { cache = true } }]
      firewall = [{ description = "media", expression = "true", action = "block" }]
    }
  }

  assert {
    condition     = length(cloudflare_ruleset.cache.rules) == 2 && cloudflare_ruleset.firewall.rules[0].action == "block"
    error_message = "the rules of the media host must join this zone's rulesets"
  }
}

run "the_origin_secret_goes_by_reference_and_by_rule" {
  command = plan

  assert {
    condition     = output.env["MY_ORIGIN_SECRET_HEADER"] == "x-my-origin-secret" && output.secret_env["MY_ORIGIN_SECRET"] == "my-origin-secret"
    error_message = "the server takes the header's name, and the secret by reference"
  }

  assert {
    condition     = !contains(values(output.env), "0123456789abcdef0123456789abcdef") && cloudflare_ruleset.transform.phase == "http_request_late_transform"
    error_message = "the value goes to the rule alone"
  }

  assert {
    condition     = nonsensitive(cloudflare_ruleset.transform.rules[0].expression) == "http.host eq \"social.example.org\"" && nonsensitive(cloudflare_ruleset.transform.rules[0].action_parameters.headers["x-my-origin-secret"].operation) == "set"
    error_message = "the rule must replace what a client sent, on the server's host alone"
  }
}

run "one_rule_covers_the_media_host_and_other_hosts" {
  command = plan

  variables {
    origin_secret_extra_hosts = ["example.org"]
    extra_rules               = { transform = ["http.host eq \"media.example.org\""] }
  }

  assert {
    condition     = nonsensitive(cloudflare_ruleset.transform.rules[0].expression) == "http.host eq \"social.example.org\" or http.host eq \"example.org\" or http.host eq \"media.example.org\""
    error_message = "a host that is not covered is locked out"
  }
}

run "a_short_secret_fails" {
  command = plan

  variables {
    origin_secret = "short"
  }

  expect_failures = [var.origin_secret]
}

run "a_header_a_cdn_sets_fails" {
  command = plan

  variables {
    origin_secret_header = "x-forwarded-secret"
  }

  expect_failures = [var.origin_secret_header]
}
