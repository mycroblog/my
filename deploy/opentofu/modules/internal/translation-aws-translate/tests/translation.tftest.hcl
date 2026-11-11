# SPDX-License-Identifier: AGPL-3.0-only

# The module has no resource, so a plan checks its outputs alone.

variables {
  region = "us-east-1"
}

run "the_region_and_the_actions" {
  command = plan

  assert {
    condition     = output.env == { MY_TRANSLATOR_AWS_REGION = "us-east-1" }
    error_message = "the provider aws must be registered in the region"
  }

  assert {
    condition     = toset([for p in output.permissions : p.permission]) == toset(["translate:TranslateText", "comprehend:DetectDominantLanguage"])
    error_message = "the role needs Amazon Translate, and Amazon Comprehend for a post whose language is unknown"
  }
}

run "a_region_that_is_no_region" {
  command = plan

  variables {
    region = "Tokyo"
  }

  expect_failures = [var.region]
}
