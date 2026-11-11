# SPDX-License-Identifier: AGPL-3.0-only

# On mocked providers: nothing here reaches Google Cloud. scripts/tofu-db-state-test.sh applies this module against a local stub,
# which is what shows that the password reaches neither the plan nor the state. By plan alone here: prevent_destroy refuses the
# destroy that follows an apply.

mock_provider "google" {}

mock_provider "random" {}

variables {
  project_id = "p"
  region     = "asia-northeast1"
}

run "backups_and_protection_are_on_by_default" {
  command = plan

  assert {
    condition     = google_sql_database_instance.db.settings[0].backup_configuration[0].enabled && google_sql_database_instance.db.settings[0].backup_configuration[0].point_in_time_recovery_enabled
    error_message = "backups and point-in-time recovery must be on by default"
  }

  assert {
    condition     = google_sql_database_instance.db.deletion_protection && google_sql_database_instance.db.settings[0].deletion_protection_enabled
    error_message = "the instance must refuse deletion by default"
  }
}

run "nothing_reaches_it_but_the_connector" {
  command = plan

  assert {
    condition     = length(google_sql_database_instance.db.settings[0].ip_configuration[0].authorized_networks) == 0
    error_message = "no network may reach the public address directly"
  }

  assert {
    condition     = google_sql_database_instance.db.settings[0].ip_configuration[0].ssl_mode == "ENCRYPTED_ONLY"
    error_message = "connections must be encrypted"
  }
}

run "the_password_goes_through_the_write_only_attributes_at_one_version" {
  command = plan

  variables {
    password_version = 3
  }

  assert {
    condition     = google_sql_user.server.password == null && google_sql_user.server.password_wo_version == 3
    error_message = "the user's password must go through the write-only attribute"
  }

  assert {
    condition     = google_secret_manager_secret_version.url.secret_data == null && google_secret_manager_secret_version.url.secret_data_wo_version == 3
    error_message = "the URL must go through the write-only attribute, at the user's version"
  }
}

run "the_contract" {
  command = plan

  assert {
    condition     = output.env["MY_DATABASE"] == "postgres"
    error_message = "env must name the database"
  }

  assert {
    condition     = output.secret_env["MY_DATABASE_URL"] == "my-database-url"
    error_message = "secret_env must name the URL's secret by ID, known at plan time"
  }

  assert {
    condition     = length(output.direct_secret_env) == 0
    error_message = "the only connection is direct: nothing differs for one-off work"
  }

  assert {
    condition     = output.permissions[0].permission == "roles/cloudsql.client" && output.permissions[0].scope == "project"
    error_message = "the app's identity needs Cloud SQL Client"
  }

  assert {
    condition     = output.cloud_sql_instances[0] == "p:asia-northeast1:my"
    error_message = "the instance must be named for the mount, at plan time"
  }
}

run "the_url_is_the_socket_of_the_database_itself" {
  command = plan

  assert {
    condition     = output.url_without_password == "postgres://my:PASSWORD@/my?host=/cloudsql/p:asia-northeast1:my"
    error_message = "the URL must be the connector's socket"
  }
}

run "parameters_go_into_the_url" {
  command = plan

  variables {
    url_parameters = { connect_timeout = "10" }
  }

  assert {
    condition     = output.url_without_password == "postgres://my:PASSWORD@/my?connect_timeout=10&host=/cloudsql/p:asia-northeast1:my"
    error_message = "url_parameters must reach the URL"
  }
}

run "a_name_that_needs_escaping_fails" {
  command = plan

  variables {
    username = "a@b"
  }

  expect_failures = [var.username]
}

run "with_no_network_only_the_connector_reaches_a_public_address" {
  command = plan

  assert {
    condition     = google_sql_database_instance.db.settings[0].ip_configuration[0].ipv4_enabled && google_sql_database_instance.db.settings[0].ip_configuration[0].private_network == null
    error_message = "with no network the instance keeps its public address"
  }

  assert {
    condition     = output.cloud_sql_instances[0] == "p:asia-northeast1:my" && length(output.permissions) == 1 && output.permissions[0].permission == "roles/cloudsql.client" && output.private_network == null
    error_message = "with no network the connector needs its mount and its role"
  }
}

run "with_a_network_it_has_a_private_address_alone" {
  command = plan

  variables {
    network = { platform = "gcp", network = "projects/p/global/networks/n", private_services_range = "n-private-services" }
  }

  override_resource {
    target = google_sql_database_instance.db
    values = {
      private_ip_address = "10.21.0.3"
    }
  }

  assert {
    condition     = !google_sql_database_instance.db.settings[0].ip_configuration[0].ipv4_enabled && google_sql_database_instance.db.settings[0].ip_configuration[0].private_network == "projects/p/global/networks/n" && google_sql_database_instance.db.settings[0].ip_configuration[0].allocated_ip_range == "n-private-services"
    error_message = "with a network the instance must have a private address and no public one"
  }

  assert {
    condition     = google_sql_database_instance.db.settings[0].ip_configuration[0].ssl_mode == "ENCRYPTED_ONLY"
    error_message = "connections must stay encrypted"
  }

  assert {
    condition     = length(output.cloud_sql_instances) == 0 && length(output.permissions) == 0 && output.private_network == "projects/p/global/networks/n"
    error_message = "on a private address nothing goes through the connector, and the app must be told the network"
  }

  assert {
    condition     = output.url_without_password == "postgres://my:PASSWORD@10.21.0.3:5432/my?sslmode=require"
    error_message = "the URL must name the private address over TLS, at the database's own port"
  }
}

run "a_network_with_no_private_services_access_fails" {
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
