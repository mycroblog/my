# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches Google Cloud.

mock_provider "google" {}

variables {
  project_id = "my-project"
}

run "it_creates_the_topic_and_the_role_of_the_subscriptions" {
  command = plan

  assert {
    condition     = google_pubsub_topic.stream.name == "my-stream" && google_project_iam_custom_role.subscriptions.role_id == "my_stream_subscriptions"
    error_message = "a role ID holds no hyphen"
  }

  assert {
    condition = toset(google_project_iam_custom_role.subscriptions.permissions) == toset([
      "pubsub.subscriptions.consume", "pubsub.subscriptions.create", "pubsub.subscriptions.delete", "pubsub.subscriptions.list",
    ])
    error_message = "the server needs exactly these on the subscriptions of the project"
  }

  assert {
    condition = output.env == {
      MY_STREAM                            = "pubsub"
      MY_STREAM_PUBSUB_TOPIC               = "projects/my-project/topics/my-stream"
      MY_STREAM_PUBSUB_SUBSCRIPTION_PREFIX = "my-stream"
    }
    error_message = "the server must take the topic and the start of its subscription IDs, known at plan time"
  }

  assert {
    condition = output.permissions == [
      { permission = "roles/pubsub.publisher", scope = "topic", resource = "projects/my-project/topics/my-stream" },
      { permission = "roles/pubsub.subscriber", scope = "topic", resource = "projects/my-project/topics/my-stream" },
      { permission = "projects/my-project/roles/my_stream_subscriptions", scope = "project", resource = null },
    ]
    error_message = "the service account must publish to and attach to this topic, and manage the subscriptions by the custom role"
  }

  assert {
    condition     = output.platform == "gcp" && output.secret_env == {}
    error_message = "the server authenticates with its service account"
  }
}

run "a_name_starting_with_goog_fails" {
  command = plan

  variables {
    name = "google-stream"
  }

  expect_failures = [var.name]
}
