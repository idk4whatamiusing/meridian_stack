# Terraform for a single EC2 host running the whole stack via
# compose.prod.yaml (Caddy on 80/443 fronts Next.js + API + realtime + AI).

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

variable "region" {
  default = "us-east-1"
}

variable "instance_type" {
  description = "t3.small handles the whole stack; t3.medium if the API needs headroom"
  default     = "t3.small"
}

variable "ssh_public_key_path" {
  default = "~/.ssh/id_ed25519.pub"
}

variable "ssh_allowed_cidr" {
  description = "CIDR allowed to SSH (22). Lock it down post-setup."
  default     = "0.0.0.0/0"
}

provider "aws" { region = var.region }

data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"]
  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-noble-24.04-amd64-server-*"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

resource "aws_key_pair" "app" {
  key_name   = "omnistack"
  public_key = file(var.ssh_public_key_path)
}

resource "aws_security_group" "app" {
  name        = "omnistack-app"
  description = "web (80/443) + ssh (22)"

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.ssh_allowed_cidr]
  }
  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_instance" "app" {
  ami                    = data.aws_ami.ubuntu.id
  instance_type          = var.instance_type
  key_name               = aws_key_pair.app.key_name
  vpc_security_group_ids = [aws_security_group.app.id]
  root_block_device {
    volume_size = 30
    volume_type = "gp3"
  }
  user_data = <<-EOF
    #!/bin/bash
    set -eux
    apt-get update
    apt-get install -y docker.io docker-compose-v2 git
    systemctl enable --now docker
    usermod -aG docker ubuntu
  EOF
  tags = { Name = "omnistack-app" }
}

resource "aws_eip" "app" {
  instance = aws_instance.app.id
  domain   = "vpc"
}

output "public_ip" {
  value = aws_eip.app.public_ip
}

output "ssh" {
  value = "ssh ubuntu@${aws_eip.app.public_ip}"
}

output "next_steps" {
  value = <<-EOT
    1. git clone <your repo> on the host, then edit .env (POSTGRES_PASSWORD, BACKEND_SECRET, DOMAIN)
    2. docker compose -f compose.prod.yaml up -d --build
    3. Caddy auto-TLS: point DNS at this IP or use sslip.io (see DEPLOY.md)
  EOT
}