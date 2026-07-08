variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region for the GKE cluster and Cloud SQL instance"
  type        = string
  default     = "us-central1"
}

variable "name" {
  description = "Name prefix for GCP resources"
  type        = string
  default     = "runright"
}

variable "namespace" {
  description = "Kubernetes namespace to deploy RunRight into"
  type        = string
  default     = "runright"
}

variable "create_cluster" {
  description = "Create a new GKE Autopilot cluster. Set to false to target an existing cluster."
  type        = bool
  default     = false
}

variable "cluster_name" {
  description = "Name of the GKE cluster (created or existing)"
  type        = string
}

variable "vpc_network" {
  description = "VPC network name for the GKE cluster and Cloud SQL instance"
  type        = string
  default     = "default"
}

variable "vpc_subnetwork" {
  description = "VPC subnetwork name for the GKE cluster"
  type        = string
  default     = "default"
}

variable "image_repository" {
  description = "Container image repository"
  type        = string
  default     = "ghcr.io/gbudjeakp/run-right"
}

variable "image_tag" {
  description = "Container image tag"
  type        = string
  default     = "latest"
}

variable "api_key" {
  description = "RunRight API key. Leave empty to disable auth."
  type        = string
  default     = ""
  sensitive   = true
}

variable "create_cloud_sql" {
  description = "Create a managed Cloud SQL for PostgreSQL instance"
  type        = bool
  default     = true
}

variable "db_tier" {
  description = "Cloud SQL machine tier (e.g. db-g1-small, db-n1-standard-1)"
  type        = string
  default     = "db-g1-small"
}

variable "db_password" {
  description = "PostgreSQL password (required when create_cloud_sql = true)"
  type        = string
  default     = ""
  sensitive   = true
}

variable "external_dsn" {
  description = "Full PostgreSQL DSN when create_cloud_sql = false"
  type        = string
  default     = ""
  sensitive   = true
}

variable "ingress_enabled" {
  description = "Enable Kubernetes Ingress"
  type        = bool
  default     = false
}

variable "ingress_class_name" {
  description = "Ingress class (e.g. gce, nginx)"
  type        = string
  default     = "gce"
}

variable "ingress_hostname" {
  description = "Hostname for the Ingress rule"
  type        = string
  default     = ""
}
