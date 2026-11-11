# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches AWS.

mock_provider "aws" {}

variables {
  availability_zones = ["ap-northeast-1a", "ap-northeast-1c"]
}

run "what_it_hands_over" {
  command = plan

  assert {
    condition     = output.platform == "aws" && output.vpc_cidr == "10.0.0.0/16" && !output.nat
    error_message = "no NAT by default"
  }

  assert {
    condition     = output.public_subnet_cidrs == ["10.0.0.0/20", "10.0.16.0/20"]
    error_message = "the ranges of the public subnets must be known at plan time, in the order of the zones"
  }

  assert {
    condition     = aws_subnet.private["ap-northeast-1a"].cidr_block == "10.0.128.0/20" && aws_subnet.private["ap-northeast-1c"].cidr_block == "10.0.144.0/20"
    error_message = "the private subnets must not overlap the public ones"
  }
}

run "only_the_public_subnets_reach_the_internet_by_default" {
  command = plan

  assert {
    condition     = aws_route.public_internet.destination_cidr_block == "0.0.0.0/0" && aws_route.private_internet == null && aws_nat_gateway.this == null && aws_eip.nat == null
    error_message = "with NAT off the private subnets have no way out, and nothing is billed for one"
  }
}

run "nat_gives_the_private_subnets_a_way_out" {
  command = plan

  variables {
    nat = true
  }

  assert {
    condition     = aws_route.private_internet.destination_cidr_block == "0.0.0.0/0" && aws_eip.nat.domain == "vpc" && output.nat
    error_message = "NAT must route the private subnets to the internet, and say so"
  }
}

run "one_zone_fails" {
  command = plan

  variables {
    availability_zones = ["ap-northeast-1a"]
  }

  expect_failures = [var.availability_zones]
}

run "a_range_too_small_for_a_load_balancer_fails" {
  command = plan

  variables {
    cidr_block = "10.0.0.0/24"
  }

  expect_failures = [var.cidr_block]
}
