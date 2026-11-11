# SPDX-License-Identifier: AGPL-3.0-only

# On mocked providers: nothing here reaches AWS. scripts/tofu-aws-plan-test.sh plans this module with no credentials. By plan
# alone here: prevent_destroy refuses the destroy that follows an apply.

mock_provider "aws" {
  mock_resource "aws_secretsmanager_secret" {
    defaults = {
      arn = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:my-database-url-AbCdEf"
    }
  }
}

mock_provider "random" {}

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

run "backups_and_protection_are_on_by_default" {
  command = plan

  assert {
    condition     = aws_db_instance.db.backup_retention_period == 7
    error_message = "backups must be on by default"
  }

  assert {
    condition     = aws_db_instance.db.deletion_protection && !aws_db_instance.db.skip_final_snapshot && !aws_db_instance.db.delete_automated_backups
    error_message = "the instance must refuse deletion, and leave a snapshot and its backups if it goes"
  }
}

run "nothing_reaches_it_from_outside_the_vpc" {
  command = plan

  assert {
    condition     = !aws_db_instance.db.publicly_accessible && aws_db_instance.db.storage_encrypted
    error_message = "the instance must be private and encrypted"
  }
}

run "the_password_goes_through_the_write_only_attributes_at_one_version" {
  command = plan

  variables {
    password_version = 3
  }

  assert {
    condition     = aws_db_instance.db.password == null && aws_db_instance.db.password_wo_version == 3
    error_message = "the password must go through the write-only attribute"
  }

  assert {
    condition     = aws_secretsmanager_secret_version.url.secret_string == null && aws_secretsmanager_secret_version.url.secret_string_wo_version == 3
    error_message = "the URL must go through the write-only attribute, at the instance's version"
  }
}

run "the_contract" {
  command = plan

  assert {
    condition     = output.env["MY_DATABASE"] == "postgres"
    error_message = "env must name the database"
  }

  assert {
    condition     = length(output.direct_secret_env) == 0 && length(output.permissions) == 0
    error_message = "the only connection is direct, and the app grants what its secrets need"
  }

  assert {
    condition     = length(output.security_group_ids) == 1
    error_message = "the app must be told which group to join"
  }
}

run "one_subnet_fails" {
  command = plan

  variables {
    network = { platform = "aws", vpc_id = "vpc-1", private_subnet_ids = ["subnet-a"] }
  }

  expect_failures = [var.network]
}
