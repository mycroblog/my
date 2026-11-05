# SPDX-License-Identifier: AGPL-3.0-only

# Every module that takes a network sharing one, on mocked providers: nothing here reaches Google Cloud. It shows that the blocks
# fit and that nothing is circular, not that they run. By plan alone: prevent_destroy refuses the destroy after an apply.

mock_provider "google" {
  mock_data "google_project" {
    defaults = {
      number = "123456789012"
    }
  }
  mock_resource "google_service_account" {
    defaults = {
      email  = "my-server@p.iam.gserviceaccount.com"
      member = "serviceAccount:my-server@p.iam.gserviceaccount.com"
    }
  }
  mock_resource "google_memorystore_instance" {
    defaults = {
      endpoints = [{
        connections = [{
          psc_auto_connection = [
            { connection_type = "CONNECTION_TYPE_PRIMARY", ip_address = "10.20.1.8", port = 6379, forwarding_rule = "", network = "", project_id = "", psc_connection_id = "", service_attachment = "" },
          ]
        }]
      }]
      managed_server_ca = [{ ca_certs = [{ certificates = ["CERT"] }] }]
    }
  }
}

mock_provider "random" {}

run "the_modules_share_the_network_of_the_module" {
  command = plan

  module {
    source = "../../tests/network-gcp"
  }

  assert {
    condition     = output.db_private_network == output.network && output.kv_private_network == output.network
    error_message = "the database and the cache must say the network they were given"
  }

  assert {
    condition     = output.app_subnetwork == output.subnetwork
    error_message = "the service and the jobs must attach to the subnet of the network"
  }

  assert {
    condition     = output.db_mounts == 0
    error_message = "a private database takes no /cloudsql mount"
  }
}

run "the_modules_share_an_existing_vpc_by_the_same_input" {
  command = plan

  module {
    source = "../../tests/network-gcp"
  }

  assert {
    condition     = output.existing_db_private_network == "projects/p/global/networks/mine" && output.existing_kv_private_network == "projects/p/global/networks/mine"
    error_message = "the object of the shape must work with no network module"
  }

  assert {
    condition     = output.existing_app_subnetwork == "projects/p/regions/asia-northeast1/subnetworks/mine-run"
    error_message = "the service must attach to the subnet of the object"
  }
}
