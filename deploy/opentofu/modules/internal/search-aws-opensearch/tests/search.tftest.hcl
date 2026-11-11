# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches AWS.

mock_provider "aws" {
  mock_data "aws_opensearch_domain" {
    defaults = {
      arn         = "arn:aws:es:ap-northeast-1:111122223333:domain/my-search"
      endpoint    = "search-my-search-abc123.ap-northeast-1.es.amazonaws.com"
      vpc_options = []
    }
  }
}

variables {
  domain_name = "my-search"
}

run "a_domain_with_a_public_endpoint" {
  command = plan

  assert {
    condition = jsonencode(output.env) == jsonencode({
      MY_SEARCH                        = "opensearch"
      MY_SEARCH_OPENSEARCH_URL         = "https://search-my-search-abc123.ap-northeast-1.es.amazonaws.com"
      MY_SEARCH_OPENSEARCH_AWS_SIGNING = "es"
      MY_SEARCH_OPENSEARCH_AWS_REGION  = "ap-northeast-1"
    })
    error_message = "the server must sign its requests to the domain's endpoint in its region: ${jsonencode(output.env)}"
  }

  assert {
    condition = (
      toset([for p in output.permissions : p.permission]) == toset(["es:ESHttpGet", "es:ESHttpPost", "es:ESHttpPut", "es:ESHttpDelete"]) &&
      alltrue([for p in output.permissions : p.resource == "arn:aws:es:ap-northeast-1:111122223333:domain/my-search/*"])
    )
    error_message = "the role needs the four methods the server sends, on the domain alone"
  }

  assert {
    condition     = output.private_network == null && length(output.security_group_ids) == 0
    error_message = "a public endpoint needs no network"
  }
}

run "a_domain_in_a_vpc" {
  command = plan

  variables {
    index              = "social-statuses"
    security_group_ids = ["sg-search-clients"]
  }

  override_data {
    target = data.aws_opensearch_domain.this
    values = {
      arn         = "arn:aws:es:ap-northeast-1:111122223333:domain/my-search"
      endpoint    = "vpc-my-search-abc123.ap-northeast-1.es.amazonaws.com"
      vpc_options = [{ vpc_id = "vpc-0123", subnet_ids = ["subnet-a"], security_group_ids = ["sg-domain"], availability_zones = ["ap-northeast-1a"] }]
    }
  }

  assert {
    condition     = output.private_network == "vpc-0123" && tolist(output.security_group_ids) == tolist(["sg-search-clients"])
    error_message = "a domain in a VPC must be on the app's network, and the tasks join the groups that reach it"
  }

  assert {
    condition     = output.env["MY_SEARCH_OPENSEARCH_INDEX"] == "social-statuses" && output.env["MY_SEARCH_OPENSEARCH_URL"] == "https://vpc-my-search-abc123.ap-northeast-1.es.amazonaws.com"
    error_message = "the alias and the VPC endpoint must reach the server"
  }
}

run "a_name_that_is_no_domain" {
  command = plan

  variables {
    domain_name = "My_Search"
  }

  expect_failures = [var.domain_name]
}

run "an_index_that_is_no_alias" {
  command = plan

  variables {
    index = "My Statuses"
  }

  expect_failures = [var.index]
}
