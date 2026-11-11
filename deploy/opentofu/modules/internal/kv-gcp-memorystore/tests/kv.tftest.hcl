# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches Google Cloud.

mock_provider "google" {
  mock_resource "google_memorystore_instance" {
    defaults = {
      endpoints = [{
        connections = [{
          psc_auto_connection = [
            { connection_type = "CONNECTION_TYPE_READER", ip_address = "10.20.0.9", port = 6379, forwarding_rule = "", network = "", project_id = "", psc_connection_id = "", service_attachment = "" },
            { connection_type = "CONNECTION_TYPE_PRIMARY", ip_address = "10.20.0.8", port = 6379, forwarding_rule = "", network = "", project_id = "", psc_connection_id = "", service_attachment = "" },
          ]
        }]
      }]
      managed_server_ca = [{ ca_certs = [{ certificates = ["CERT-A", "CERT-B"] }] }]
    }
  }
}

variables {
  project_id = "p"
  region     = "asia-northeast1"
  network = {
    platform                      = "gcp"
    network                       = "projects/p/global/networks/n"
    subnetwork                    = "projects/p/regions/asia-northeast1/subnetworks/n-run"
    memorystore_connection_policy = true
  }
}

run "the_settings" {
  command = apply

  assert {
    condition     = output.env == { MY_KV = "valkey", MY_VALKEY_URL = "valkeys://10.20.0.8:6379" }
    error_message = "the URL must name the primary connection, over TLS and with no password"
  }

  assert {
    condition     = output.secret_files == { MY_VALKEY_TLS_CA_FILE = "my-valkey-ca" } && length(output.secret_env) == 0 && length(output.permissions) == 0
    error_message = "only the CA goes through the secret store"
  }

  assert {
    condition     = google_secret_manager_secret_version.ca.secret_data == "CERT-A\nCERT-B"
    error_message = "the secret must hold every certificate of the CA"
  }

  assert {
    condition     = output.private_network == "projects/p/global/networks/n"
    error_message = "the app must be told the network of the instance"
  }
}

run "the_instance_and_its_network" {
  command = plan

  assert {
    condition     = google_memorystore_instance.valkey.authorization_mode == "AUTH_DISABLED" && google_memorystore_instance.valkey.transit_encryption_mode == "SERVER_AUTHENTICATION"
    error_message = "the server cannot present an IAM token, and traffic must stay encrypted"
  }

  assert {
    condition     = google_memorystore_instance.valkey.mode == "CLUSTER_DISABLED" && google_memorystore_instance.valkey.shard_count == 1 && google_memorystore_instance.valkey.node_type == "SHARED_CORE_NANO" && google_memorystore_instance.valkey.replica_count == 0
    error_message = "the default must be the smallest instance"
  }

  assert {
    condition     = google_memorystore_instance.valkey.desired_auto_created_endpoints[0].network == "projects/p/global/networks/n"
    error_message = "the instance must be on the network it was given"
  }
}

run "a_network_with_no_policy_fails" {
  command = plan

  variables {
    network = { platform = "gcp", network = "projects/p/global/networks/n" }
  }

  expect_failures = [var.network]
}

run "a_network_of_another_platform_fails" {
  command = plan

  variables {
    network = { platform = "aws", vpc_id = "vpc-1" }
  }

  expect_failures = [var.network]
}
