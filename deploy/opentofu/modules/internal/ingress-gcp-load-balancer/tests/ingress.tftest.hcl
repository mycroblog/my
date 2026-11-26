# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches Google Cloud. Nothing here is protected from destroy, so the runs apply.

mock_provider "google" {
  mock_resource "google_compute_global_address" {
    defaults = {
      address = "203.0.113.10"
    }
  }
}

variables {
  project_id   = "my-project"
  region       = "us-central1"
  host         = "social.example.org"
  service_name = "my"
}

run "it_serves_https_for_the_host_and_redirects_http" {
  command = apply

  assert {
    condition     = google_compute_managed_ssl_certificate.host.managed[0].domains == tolist(["social.example.org"]) && google_compute_global_forwarding_rule.https.port_range == "443"
    error_message = "the load balancer must serve TLS for the host on 443"
  }

  assert {
    condition     = google_compute_url_map.redirect.default_url_redirect[0].https_redirect && google_compute_global_forwarding_rule.redirect.port_range == "80"
    error_message = "port 80 only redirects to HTTPS"
  }

  assert {
    condition     = google_compute_region_network_endpoint_group.service.network_endpoint_type == "SERVERLESS" && google_compute_region_network_endpoint_group.service.cloud_run[0].service == "my" && google_compute_region_network_endpoint_group.service.region == "us-central1"
    error_message = "the load balancer must reach the Cloud Run service, in its region"
  }

  assert {
    condition     = google_compute_global_forwarding_rule.https.name == "my-lb-https" && google_compute_global_forwarding_rule.redirect.name == "my-lb-redirect"
    error_message = "the names must start with the name given"
  }
}

run "it_writes_what_the_server_trusts_and_what_dns_needs" {
  command = apply

  assert {
    condition     = output.trusted_proxies == ["203.0.113.10"]
    error_message = "the server must look past the address of the load balancer"
  }

  assert {
    condition     = output.ip_address == "203.0.113.10" && output.dns_records == [{ name = "social.example.org", type = "A", rrdata = "203.0.113.10" }]
    error_message = "the host must point at the address of the load balancer"
  }

  assert {
    condition     = output.service_ingress == "internal-and-load-balancer"
    error_message = "the internet must not go around the load balancer through the run.app URL"
  }

  assert {
    condition     = output.platform == "gcp" && output.env == {} && output.secret_env == {} && output.permissions == []
    error_message = "a load balancer sets nothing on the server"
  }
}
