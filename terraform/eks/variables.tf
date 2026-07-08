variable "cluster_name" {
  description = "Name of the existing EKS cluster"
  type        = string
}

variable "namespace" {
  description = "Kubernetes namespace to deploy RunRight into"
  type        = string
  default     = "runright"
}

variable "name" {
  description = "Name prefix for AWS resources (RDS, security groups)"
  type        = string
  default     = "runright"
}

variable "vpc_id" {
  description = "VPC ID where the EKS cluster runs"
  type        = string
}

variable "private_subnet_ids" {
  description = "Subnets for the RDS instance (must be in the same VPC)"
  type        = list(string)
}

variable "eks_node_cidrs" {
  description = "CIDRs of EKS node groups — allowed to reach RDS on port 5432"
  type        = list(string)
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

variable "create_rds" {
  description = "Create a managed RDS PostgreSQL instance"
  type        = bool
  default     = true
}

variable "rds_instance_class" {
  description = "RDS instance class"
  type        = string
  default     = "db.t4g.micro"
}

variable "db_password" {
  description = "PostgreSQL password (required when create_rds = true)"
  type        = string
  default     = ""
  sensitive   = true
}

variable "external_dsn" {
  description = "Full PostgreSQL DSN when create_rds = false"
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
  description = "Ingress class (e.g. alb, nginx)"
  type        = string
  default     = "alb"
}

variable "ingress_hostname" {
  description = "Hostname for the Ingress rule"
  type        = string
  default     = ""
}

variable "tags" {
  description = "Tags applied to AWS resources"
  type        = map(string)
  default     = {}
}
