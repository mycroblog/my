# SPDX-License-Identifier: AGPL-3.0-only

# A VPC on AWS that the other modules take as network = module.network: public and private subnets in each zone you name, and
# one NAT gateway as an option. It reads nothing of another module, so nothing can be circular. This module has never been
# applied: read the README.

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0, < 7.0"
    }
  }
}

variable "name" {
  description = "Value of the Name tag of the VPC, and the start of the others."
  type        = string
  default     = "my"
  nullable    = false
}

variable "cidr_block" {
  description = "Range of the VPC, from /16 to /22. Each subnet takes a sixteenth of it, so a /22 leaves /26 subnets, which still holds a load balancer."
  type        = string
  default     = "10.0.0.0/16"
  nullable    = false

  validation {
    condition     = can(cidrhost(var.cidr_block, 0)) && tonumber(split("/", var.cidr_block)[1]) >= 16 && tonumber(split("/", var.cidr_block)[1]) <= 22
    error_message = "cidr_block must be an IPv4 range from /16 to /22."
  }
}

variable "availability_zones" {
  description = "Zones of the subnets, by name. An input and not a lookup, so that a plan needs no call to AWS. A load balancer and RDS each need two."
  type        = list(string)
  nullable    = false

  validation {
    condition     = length(var.availability_zones) >= 2 && length(var.availability_zones) <= 8 && length(distinct(var.availability_zones)) == length(var.availability_zones)
    error_message = "availability_zones must name 2 to 8 different zones."
  }
}

variable "nat" {
  description = "Creates one NAT gateway and routes the private subnets to the internet through it, which AWS bills by the hour and by the GB. Without it, nothing in a private subnet reaches the internet: tasks with assign_public_ip = false fail the plan."
  type        = bool
  default     = false
  nullable    = false
}

locals {
  zones = { for i, zone in var.availability_zones : zone => i }

  # Known at plan time, unlike the IDs: what the server trusts is the ranges of the load balancer's subnets.
  public_cidrs  = { for zone, i in local.zones : zone => cidrsubnet(var.cidr_block, 4, i) }
  private_cidrs = { for zone, i in local.zones : zone => cidrsubnet(var.cidr_block, 4, 8 + i) }
}

resource "aws_vpc" "this" {
  cidr_block           = var.cidr_block
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = { Name = var.name }
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id

  tags = { Name = var.name }
}

resource "aws_subnet" "public" {
  for_each = local.public_cidrs

  vpc_id            = aws_vpc.this.id
  availability_zone = each.key
  cidr_block        = each.value

  tags = { Name = "${var.name}-public-${each.key}" }
}

resource "aws_subnet" "private" {
  for_each = local.private_cidrs

  vpc_id            = aws_vpc.this.id
  availability_zone = each.key
  cidr_block        = each.value

  tags = { Name = "${var.name}-private-${each.key}" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id

  tags = { Name = "${var.name}-public" }
}

resource "aws_route" "public_internet" {
  route_table_id         = aws_route_table.public.id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.this.id
}

resource "aws_route_table_association" "public" {
  for_each = local.zones

  subnet_id      = aws_subnet.public[each.key].id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table" "private" {
  vpc_id = aws_vpc.this.id

  tags = { Name = "${var.name}-private" }
}

resource "aws_route_table_association" "private" {
  for_each = local.zones

  subnet_id      = aws_subnet.private[each.key].id
  route_table_id = aws_route_table.private.id
}

resource "aws_eip" "nat" {
  domain = "vpc"

  tags = { Name = "${var.name}-nat" }

  lifecycle {
    enabled = var.nat
  }

  depends_on = [aws_internet_gateway.this]
}

# One gateway in the first zone: if that zone goes down, the private subnets lose the internet until it comes back.
resource "aws_nat_gateway" "this" {
  allocation_id = aws_eip.nat.id
  subnet_id     = aws_subnet.public[var.availability_zones[0]].id

  tags = { Name = var.name }

  lifecycle {
    enabled = var.nat
  }

  depends_on = [aws_route.public_internet]
}

resource "aws_route" "private_internet" {
  route_table_id         = aws_route_table.private.id
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.this.id

  lifecycle {
    enabled = var.nat
  }
}

output "platform" {
  description = "The platform of this network: only a module of AWS takes it."
  value       = "aws"
}

output "vpc_id" {
  description = "The VPC."
  value       = aws_vpc.this.id
}

output "vpc_cidr" {
  description = "The primary range of the VPC, which the load balancer's group allows out to reach the tasks."
  value       = var.cidr_block
}

output "public_subnet_ids" {
  description = "The public subnets, in the order of availability_zones, with their way to the internet in place."
  value       = [for zone in var.availability_zones : aws_subnet.public[zone].id]
  depends_on  = [aws_route.public_internet, aws_route_table_association.public]
}

output "public_subnet_cidrs" {
  description = "The ranges of the public subnets, in the same order."
  value       = [for zone in var.availability_zones : local.public_cidrs[zone]]
}

output "private_subnet_ids" {
  description = "The private subnets, in the order of availability_zones, with their way to the internet in place when there is one."
  value       = [for zone in var.availability_zones : aws_subnet.private[zone].id]
  depends_on  = [aws_route_table_association.private, aws_route.private_internet]
}

output "nat" {
  description = "Whether the private subnets reach the internet."
  value       = var.nat
  depends_on  = [aws_route.private_internet]
}
