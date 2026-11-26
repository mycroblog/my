# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches Google Cloud.

mock_provider "google" {
  mock_resource "google_cloud_run_domain_mapping" {
    defaults = {
      status = [{
        conditions          = []
        mapped_route_name   = "my"
        observed_generation = 1
        resource_records = [
          { name = "social", type = "CNAME", rrdata = "ghs.googlehosted.com." },
        ]
      }]
    }
  }
}

variables {
  project_id   = "my-project"
  region       = "us-central1"
  host         = "social.example.org"
  service_name = "my"
}

run "it_maps_the_host_to_the_service" {
  command = apply

  assert {
    condition     = google_cloud_run_domain_mapping.host.name == "social.example.org" && google_cloud_run_domain_mapping.host.location == "us-central1" && google_cloud_run_domain_mapping.host.spec[0].route_name == "my"
    error_message = "the mapping must route the host to the service, in its region"
  }

  assert {
    condition     = output.dns_records == [{ name = "social", type = "CNAME", rrdata = "ghs.googlehosted.com." }]
    error_message = "the records Cloud Run gives must reach the operator"
  }

  assert {
    condition     = output.service_ingress == "all" && output.trusted_proxies == []
    error_message = "the internet reaches the service directly, through Cloud Run's front end alone"
  }

  assert {
    condition     = output.platform == "gcp" && output.env == {} && output.secret_env == {} && output.permissions == []
    error_message = "a mapping sets nothing on the server"
  }
}
