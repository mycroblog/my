# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches AWS.

mock_provider "aws" {
  mock_resource "aws_security_group" {
    defaults = {
      id = "sg-0123456789abcdef0"
    }
  }
  mock_resource "aws_elasticache_serverless_cache" {
    defaults = {
      endpoint = [{ address = "my-valkey-abc.serverless.apne1.cache.amazonaws.com", port = 6379 }]
    }
  }
}

variables {
  network = {
    platform            = "aws"
    vpc_id              = "vpc-1"
    vpc_cidr            = "10.0.0.0/16"
    public_subnet_ids   = ["subnet-1", "subnet-2"]
    public_subnet_cidrs = ["10.0.0.0/24", "10.0.1.0/24"]
    private_subnet_ids  = ["subnet-a", "subnet-b"]
    nat                 = true
  }
}

run "the_settings" {
  command = apply

  assert {
    condition     = output.env == { MY_KV = "valkey", MY_VALKEY_URL = "valkeys://my-valkey-abc.serverless.apne1.cache.amazonaws.com:6379" }
    error_message = "the URL must use TLS, which a serverless cache always has, and hold no password"
  }

  assert {
    condition     = length(output.secret_env) == 0 && length(output.permissions) == 0
    error_message = "the module has no secret and asks for no IAM action"
  }

  assert {
    condition     = length(output.security_group_ids) == 1 && output.security_group_ids[0] == aws_security_group.client.id
    error_message = "the app must take the client group"
  }

  assert {
    condition     = output.private_network == "vpc-1" && aws_elasticache_serverless_cache.valkey.subnet_ids == toset(["subnet-a", "subnet-b"])
    error_message = "the cache must be in the private subnets of the network, and say which"
  }
}

run "only_the_clients_reach_the_cache" {
  command = apply

  assert {
    condition     = aws_vpc_security_group_ingress_rule.from_client.referenced_security_group_id == aws_security_group.client.id && aws_vpc_security_group_ingress_rule.from_client.cidr_ipv4 == null && aws_vpc_security_group_ingress_rule.from_client.from_port == 6379 && aws_vpc_security_group_ingress_rule.from_client.to_port == 6379
    error_message = "the cache must take 6379 from the client group alone"
  }

  assert {
    condition     = aws_elasticache_serverless_cache.valkey.engine == "valkey" && aws_elasticache_serverless_cache.valkey.user_group_id == null
    error_message = "the cache must be Valkey, with no user group"
  }
}

run "no_subnet_fails" {
  command = plan

  variables {
    network = { platform = "aws", vpc_id = "vpc-1" }
  }

  expect_failures = [var.network]
}
