# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches Google Cloud.

mock_provider "google" {}

variables {
  project_id = "p"
  region     = "asia-northeast1"
}

run "what_it_hands_over" {
  command = plan

  assert {
    condition     = output.platform == "gcp" && output.network == "projects/p/global/networks/my" && output.subnetwork == "projects/p/regions/asia-northeast1/subnetworks/my-run"
    error_message = "the shape must be known at plan time, built from the inputs"
  }

  assert {
    condition     = output.private_services_range == "my-private-services" && output.memorystore_connection_policy && !output.nat
    error_message = "a network has private services access and the policy, and no NAT by default"
  }
}

run "private_services_and_memorystore_are_on_their_own_ranges" {
  command = plan

  assert {
    condition     = google_compute_global_address.private_services.purpose == "VPC_PEERING" && google_compute_global_address.private_services.address == "10.21.0.0" && google_compute_global_address.private_services.prefix_length == 20
    error_message = "private services access takes a range of the purpose VPC_PEERING"
  }

  assert {
    condition     = google_service_networking_connection.private_services.service == "servicenetworking.googleapis.com" && google_service_networking_connection.private_services.reserved_peering_ranges == tolist(["my-private-services"])
    error_message = "the connection must use that range"
  }

  assert {
    condition     = google_network_connectivity_service_connection_policy.memorystore.service_class == "gcp-memorystore" && google_network_connectivity_service_connection_policy.memorystore.psc_config[0].limit == null
    error_message = "one policy of Memorystore's class, with no limit, as the number of instances is not known here"
  }

  assert {
    condition     = google_compute_subnetwork.run.ip_cidr_range != google_compute_subnetwork.service_connect.ip_cidr_range && google_compute_subnetwork.run.private_ip_google_access
    error_message = "Direct VPC egress and Private Service Connect must not share a subnet"
  }
}

run "no_nat_by_default" {
  command = plan

  assert {
    condition     = google_compute_router.nat == null && google_compute_router_nat.nat == null
    error_message = "NAT must be off unless asked for"
  }
}

run "nat_covers_the_subnet_of_the_service" {
  command = plan

  variables {
    nat = true
  }

  assert {
    condition     = google_compute_router_nat.nat.source_subnetwork_ip_ranges_to_nat == "LIST_OF_SUBNETWORKS" && output.nat
    error_message = "NAT must cover the subnet of the service and say so"
  }
}

run "a_subnet_too_small_for_direct_vpc_fails" {
  command = plan

  variables {
    subnet_cidr = "10.20.0.0/27"
  }

  expect_failures = [var.subnet_cidr]
}

run "a_public_range_fails" {
  command = plan

  variables {
    private_services_cidr = "8.8.0.0/20"
  }

  expect_failures = [var.private_services_cidr]
}

run "a_range_too_small_for_cloud_sql_fails" {
  command = plan

  variables {
    private_services_cidr = "10.21.0.0/25"
  }

  expect_failures = [var.private_services_cidr]
}
