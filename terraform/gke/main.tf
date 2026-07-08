terraform {
  required_version = ">= 1.5"
  required_providers {
    google = {
      source  = "hashicorp/google"
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

data "google_client_config" "this" {}

data "google_container_cluster" "this" {
  name     = var.cluster_name
  location = var.region
  project  = var.project_id
}

# ── Providers (configured from GKE cluster outputs) ───────────────────────────

provider "kubernetes" {
  host                   = "https://${data.google_container_cluster.this.endpoint}"
  cluster_ca_certificate = base64decode(data.google_container_cluster.this.master_auth[0].cluster_ca_certificate)
  token                  = data.google_client_config.this.access_token
}

provider "helm" {
  kubernetes {
    host                   = "https://${data.google_container_cluster.this.endpoint}"
    cluster_ca_certificate = base64decode(data.google_container_cluster.this.master_auth[0].cluster_ca_certificate)
    token                  = data.google_client_config.this.access_token
  }
}

# ── GKE Autopilot cluster (optional — skip if targeting an existing cluster) ──

resource "google_container_cluster" "this" {
  count    = var.create_cluster ? 1 : 0
  name     = var.cluster_name
  location = var.region
  project  = var.project_id

  enable_autopilot = true
  network          = var.vpc_network
  subnetwork       = var.vpc_subnetwork

  deletion_protection = false
}

# ── Namespace ─────────────────────────────────────────────────────────────────

resource "kubernetes_namespace" "this" {
  metadata {
    name = var.namespace
    labels = {
      "app.kubernetes.io/managed-by" = "terraform"
    }
  }

  depends_on = [google_container_cluster.this]
}

# ── Cloud SQL for PostgreSQL (optional — skip if you bring your own) ──────────

resource "google_sql_database_instance" "this" {
  count            = var.create_cloud_sql ? 1 : 0
  name             = "${var.name}-runright"
  database_version = "POSTGRES_16"
  region           = var.region
  project          = var.project_id

  settings {
    tier              = var.db_tier
    availability_type = "ZONAL"
    disk_autoresize   = true
    disk_type         = "PD_SSD"

    backup_configuration {
      enabled    = true
      start_time = "03:00"
    }

    ip_configuration {
      ipv4_enabled    = false
      private_network = "projects/${var.project_id}/global/networks/${var.vpc_network}"
    }
  }

  deletion_protection = false
}

resource "google_sql_database" "this" {
  count    = var.create_cloud_sql ? 1 : 0
  name     = "runright"
  instance = google_sql_database_instance.this[0].name
  project  = var.project_id
}

resource "google_sql_user" "this" {
  count    = var.create_cloud_sql ? 1 : 0
  name     = "runright"
  instance = google_sql_database_instance.this[0].name
  password = var.db_password
  project  = var.project_id
}

locals {
  dsn = var.create_cloud_sql ? (
    "postgres://runright:${var.db_password}@${google_sql_database_instance.this[0].private_ip_address}:5432/runright?sslmode=require"
  ) : var.external_dsn
}

# ── GCP Service Account + Workload Identity binding ───────────────────────────

resource "google_service_account" "runright" {
  account_id   = "${var.name}-runright"
  display_name = "RunRight workload service account"
  project      = var.project_id
}

# Allow the Kubernetes service account (created by Helm) to impersonate the GSA
resource "google_service_account_iam_member" "workload_identity" {
  service_account_id = google_service_account.runright.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[${var.namespace}/runright]"
}

# ── Secret Manager secrets ────────────────────────────────────────────────────

resource "google_secret_manager_secret" "dsn" {
  secret_id = "${var.name}-runright-dsn"
  project   = var.project_id

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "dsn" {
  secret      = google_secret_manager_secret.dsn.id
  secret_data = local.dsn
}

resource "google_secret_manager_secret" "api_key" {
  secret_id = "${var.name}-runright-api-key"
  project   = var.project_id

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "api_key" {
  secret      = google_secret_manager_secret.api_key.id
  secret_data = var.api_key
}

# Grant the GSA access to both secrets
resource "google_secret_manager_secret_iam_member" "dsn" {
  secret_id = google_secret_manager_secret.dsn.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.runright.email}"
  project   = var.project_id
}

resource "google_secret_manager_secret_iam_member" "api_key" {
  secret_id = google_secret_manager_secret.api_key.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.runright.email}"
  project   = var.project_id
}

# ── Kubernetes secret (injected into Pod via Helm) ────────────────────────────

resource "kubernetes_secret" "runright" {
  metadata {
    name      = "runright"
    namespace = kubernetes_namespace.this.metadata[0].name
    annotations = {
      "iam.gke.io/gcp-service-account" = google_service_account.runright.email
    }
  }
  data = {
    dsn     = local.dsn
    api-key = var.api_key
  }
  type = "Opaque"

  depends_on = [kubernetes_namespace.this]
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

  # Annotate the Kubernetes service account with the GSA email for Workload Identity
  set {
    name  = "serviceAccount.annotations.iam\\.gke\\.io/gcp-service-account"
    value = google_service_account.runright.email
  }

  values = [yamlencode({
    existingSecret = {
      name      = kubernetes_secret.runright.metadata[0].name
      apiKeyKey = "api-key"
    }
  })]

  depends_on = [kubernetes_secret.runright]
}
