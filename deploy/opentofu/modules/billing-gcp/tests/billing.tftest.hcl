# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches Google Cloud.

mock_provider "google" {
  mock_resource "google_pubsub_topic" {
    defaults = {
      id = "projects/p/topics/my-billing-budget"
    }
  }
}

variables {
  project_id         = "billing-project"
  billing_account_id = "012345-6789AB-CDEF01"
}

run "bigquery_names_the_table_and_returns_its_roles" {
  command = plan

  assert {
    condition = output.env == {
      MY_BILLING_GCP_METHOD = "bigquery"
      MY_BILLING_GCP_TABLE  = "billing-project.billing_export.gcp_billing_export_v1_012345_6789AB_CDEF01"
    }
    error_message = "the table's name must replace the hyphens of the account's ID"
  }

  assert {
    condition = output.permissions == tolist([
      { permission = "roles/bigquery.jobUser", scope = "project", resource = "billing-project" },
      { permission = "READER", scope = "dataset", resource = "projects/billing-project/datasets/billing_export" },
    ])
    error_message = "the server needs to run queries, and to read the dataset alone"
  }

  assert {
    condition     = google_bigquery_dataset.export.default_table_expiration_ms == null && google_bigquery_dataset.export.location == "US"
    error_message = "rows of the export must not expire, and a multi-region receives the months before the export starts"
  }

  assert {
    condition     = google_pubsub_topic.budget == null && google_pubsub_subscription.budget == null
    error_message = "the bigquery method needs no Pub/Sub"
  }

  assert {
    condition     = google_project_iam_member.external_query == null && google_bigquery_dataset_access.external_export == null
    error_message = "the app of Google Cloud grants the roles"
  }

  assert {
    condition     = length(output.billing_sources) == 1 && output.billing_sources[0] == "gcp" && output.budget_topic == null
    error_message = "the module adds gcp to the sources"
  }
}

run "another_query_project_gets_the_role_and_the_setting" {
  command = plan

  variables {
    query_project = "server-project"
  }

  assert {
    condition     = output.env["MY_BILLING_GCP_QUERY_PROJECT"] == "server-project" && output.permissions[0].resource == "server-project"
    error_message = "the role must go where the queries run"
  }
}

run "an_existing_dataset_is_left_alone" {
  command = plan

  variables {
    create_dataset = false
    dataset_id     = "existing"
  }

  assert {
    condition     = google_bigquery_dataset.export == null && output.env["MY_BILLING_GCP_TABLE"] == "billing-project.existing.gcp_billing_export_v1_012345_6789AB_CDEF01"
    error_message = "the module must read an existing dataset without creating one"
  }
}

run "budget_makes_a_pull_subscription_that_keeps_its_messages" {
  command = plan

  variables {
    method             = "budget"
    billing_account_id = null
  }

  assert {
    condition = output.env == {
      MY_BILLING_GCP_METHOD              = "budget"
      MY_BILLING_GCP_BUDGET_SUBSCRIPTION = "projects/billing-project/subscriptions/my-billing-budget"
    }
    error_message = "the setting must be the subscription's full name"
  }

  assert {
    condition     = output.permissions == tolist([{ permission = "roles/pubsub.subscriber", scope = "subscription", resource = "projects/billing-project/subscriptions/my-billing-budget" }])
    error_message = "the server needs to consume the subscription alone"
  }

  assert {
    condition = (
      google_pubsub_subscription.budget.message_retention_duration == "2678400s" &&
      google_pubsub_subscription.budget.ack_deadline_seconds == 60 &&
      google_pubsub_subscription.budget.expiration_policy[0].ttl == "" &&
      length(google_pubsub_subscription.budget.push_config) == 0
    )
    error_message = "a pull subscription, with 31 days of retention, that never expires"
  }

  assert {
    condition     = google_bigquery_dataset.export == null
    error_message = "the budget method needs no dataset"
  }

  assert {
    condition     = output.budget_topic == "projects/billing-project/topics/my-billing-budget"
    error_message = "the operator connects the budget to this topic"
  }
}

run "bigquery_needs_the_billing_account" {
  command = plan

  variables {
    billing_account_id = null
  }

  expect_failures = [var.billing_account_id]
}

run "an_unknown_method_fails" {
  command = plan

  variables {
    method = "invoice"
  }

  expect_failures = [var.method]
}

run "outside_google_cloud_the_module_grants_bigquery" {
  command = plan

  variables {
    external_member = "principalSet://iam.googleapis.com/projects/1/locations/global/workloadIdentityPools/my/attribute.aws_role/my-server"
  }

  assert {
    condition     = length(output.permissions) == 0
    error_message = "an AWS app refuses these permissions, so the module must not return them"
  }

  assert {
    condition = (
      google_project_iam_member.external_query.role == "roles/bigquery.jobUser" &&
      google_project_iam_member.external_query.project == "billing-project" &&
      google_bigquery_dataset_access.external_export.role == "READER" &&
      google_bigquery_dataset_access.external_export.iam_member == var.external_member
    )
    error_message = "the member must get both roles of the bigquery method"
  }
}

run "outside_google_cloud_the_module_grants_the_subscription" {
  command = plan

  variables {
    method             = "budget"
    billing_account_id = null
    external_member    = "principalSet://iam.googleapis.com/projects/1/locations/global/workloadIdentityPools/my/attribute.aws_role/my-server"
  }

  assert {
    condition     = length(output.permissions) == 0 && google_pubsub_subscription_iam_member.external_budget.subscription == "my-billing-budget"
    error_message = "the member must get the subscriber role"
  }
}
