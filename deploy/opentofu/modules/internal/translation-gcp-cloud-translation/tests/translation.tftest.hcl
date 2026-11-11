# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches Google Cloud.

mock_provider "google" {}

variables {
  project_id     = "my-project"
  app_project_id = "my-project"
}

run "in_the_app_project" {
  command = plan

  assert {
    condition     = google_project_service.translate.service == "translate.googleapis.com" && google_project_service.translate.project == "my-project" && !google_project_service.translate.disable_on_destroy
    error_message = "the API must be enabled in the project, and stay enabled when the module goes"
  }

  assert {
    condition     = length(output.permissions) == 1 && output.permissions[0].permission == "roles/cloudtranslate.user" && output.permissions[0].scope == "project" && output.permissions[0].resource == null
    error_message = "the role goes on the app's own project, which a grant names by no resource"
  }

  assert {
    condition     = jsonencode(output.env) == jsonencode({ MY_TRANSLATOR_GOOGLE_PROJECT = "my-project", MY_TRANSLATOR_GOOGLE_LOCATION = "global" })
    error_message = "the provider must be registered in the project, at the global location by default"
  }
}

run "in_another_project" {
  command = plan

  variables {
    project_id = "my-translations"
    location   = "us-central1"
  }

  assert {
    condition     = output.permissions[0].resource == "my-translations" && google_project_service.translate.project == "my-translations"
    error_message = "the API and the role go on the project that pays for the translations"
  }

  assert {
    condition     = output.env["MY_TRANSLATOR_GOOGLE_PROJECT"] == "my-translations" && output.env["MY_TRANSLATOR_GOOGLE_LOCATION"] == "us-central1"
    error_message = "the server must bill the translations to that project, at the location"
  }
}

run "a_location_that_is_no_name" {
  command = plan

  variables {
    location = "projects/my-project/locations/global"
  }

  expect_failures = [var.location]
}
