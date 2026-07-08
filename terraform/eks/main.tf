terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
    helm = {
      source  = "hashicorp/helm"
      version = ">= 2.12"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.25"
    }
  }
}

# ── Data sources ───────────────────────────────────────────────────────────────

data "aws_eks_cluster" "this" {
  name = var.cluster_name
}

data "aws_eks_cluster_auth" "this" {
  name = var.cluster_name
}

# ── Providers (configured from EKS cluster outputs) ───────────────────────────

provider "kubernetes" {
  host                   = data.aws_eks_cluster.this.endpoint
  cluster_ca_certificate = base64decode(data.aws_eks_cluster.this.certificate_authority[0].data)
  token                  = data.aws_eks_cluster_auth.this.token
}

provider "helm" {
  kubernetes {
    host                   = data.aws_eks_cluster.this.endpoint
    cluster_ca_certificate = base64decode(data.aws_eks_cluster.this.certificate_authority[0].data)
    token                  = data.aws_eks_cluster_auth.this.token
  }
}

# ── Namespace ─────────────────────────────────────────────────────────────────

resource "kubernetes_namespace" "this" {
  metadata {
    name = var.namespace
    labels = {
      "app.kubernetes.io/managed-by" = "terraform"
    }
  }
}

# ── RDS PostgreSQL (optional — skip if you bring your own) ────────────────────

data "aws_region" "current" {}

resource "aws_db_subnet_group" "this" {
  count      = var.create_rds ? 1 : 0
  name       = "${var.name}-runright"
  subnet_ids = var.private_subnet_ids
  tags       = var.tags
}

resource "aws_security_group" "rds" {
  count       = var.create_rds ? 1 : 0
  name        = "${var.name}-runright-rds"
  description = "RunRight RDS — inbound from EKS node CIDR"
  vpc_id      = var.vpc_id
  tags        = var.tags

  ingress {
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    cidr_blocks = var.eks_node_cidrs
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_db_instance" "this" {
  count = var.create_rds ? 1 : 0

  identifier        = "${var.name}-runright"
  engine            = "postgres"
  engine_version    = "16"
  instance_class    = var.rds_instance_class
  allocated_storage = 20
  storage_type      = "gp3"

  db_name  = "runright"
  username = "runright"
  password = var.db_password

  db_subnet_group_name   = aws_db_subnet_group.this[0].name
  vpc_security_group_ids = [aws_security_group.rds[0].id]

  skip_final_snapshot       = false
  final_snapshot_identifier = "${var.name}-runright-final"
  deletion_protection       = true
  backup_retention_period   = 7
  storage_encrypted         = true

  tags = var.tags
}

locals {
  dsn = var.create_rds ? (
    "postgres://runright:${var.db_password}@${aws_db_instance.this[0].address}:5432/runright?sslmode=require"
  ) : var.external_dsn
}

# ── Kubernetes secret for DSN + API key ───────────────────────────────────────

resource "kubernetes_secret" "runright" {
  metadata {
    name      = "runright"
    namespace = kubernetes_namespace.this.metadata[0].name
  }
  data = {
    dsn     = local.dsn
    api-key = var.api_key
  }
  type = "Opaque"
}

# ── Helm release ──────────────────────────────────────────────────────────────

resource "helm_release" "runright" {
  name       = "runright"
  namespace  = kubernetes_namespace.this.metadata[0].name
  chart      = "${path.module}/../../helm/runright"

  set {
    name  = "image.repository"
    value = var.image_repository
  }
  set {
    name  = "image.tag"
    value = var.image_tag
  }
  set {
    name  = "postgresql.enabled"
    value = "false"
  }
  set_sensitive {
    name  = "externalDSN"
    value = local.dsn
  }
  set {
    name  = "ingress.enabled"
    value = tostring(var.ingress_enabled)
  }
  set {
    name  = "ingress.className"
    value = var.ingress_class_name
  }

  dynamic "set" {
    for_each = var.ingress_hostname != "" ? [1] : []
    content {
      name  = "ingress.hosts[0].host"
      value = var.ingress_hostname
    }
  }

  values = [yamlencode({
    existingSecret = {
      name      = kubernetes_secret.runright.metadata[0].name
      apiKeyKey = "api-key"
    }
  })]

  depends_on = [kubernetes_secret.runright]
}
