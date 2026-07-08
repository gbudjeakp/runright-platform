variable "name" {
  description = "Name prefix for all resources"
  type        = string
  default     = "runright"
}

variable "environment" {
  description = "Deployment environment (e.g. production, staging)"
  type        = string
  default     = "production"
}

variable "vpc_id" {
  description = "VPC to deploy into"
  type        = string
}

variable "private_subnet_ids" {
  description = "Private subnets for the ECS task and RDS instance"
  type        = list(string)
}

variable "allowed_cidrs" {
  description = "CIDRs allowed to reach the RunRight API (e.g. your VPC CIDR)"
  type        = list(string)
  default     = ["10.0.0.0/8"]
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

variable "task_cpu" {
  description = "ECS task CPU units (256, 512, 1024, 2048, 4096)"
  type        = number
  default     = 512
}

variable "task_memory" {
  description = "ECS task memory in MiB"
  type        = number
  default     = 1024
}

variable "desired_count" {
  description = "Desired number of ECS task replicas"
  type        = number
  default     = 1
}

variable "api_key" {
  description = "RunRight API key stored in Secrets Manager. Leave empty to disable auth."
  type        = string
  default     = ""
  sensitive   = true
}

variable "log_level" {
  description = "Log level (debug, info, warn, error)"
  type        = string
  default     = "info"
}

variable "log_retention_days" {
  description = "CloudWatch log retention in days"
  type        = number
  default     = 30
}

# ── RDS ───────────────────────────────────────────────────────────────────────

variable "create_rds" {
  description = "Create a managed RDS PostgreSQL instance. Set false to use external_dsn."
  type        = bool
  default     = true
}

variable "rds_instance_class" {
  description = "RDS instance class"
  type        = string
  default     = "db.t4g.micro"
}

variable "db_password" {
  description = "PostgreSQL password (only used when create_rds = true)"
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

# ── ALB (optional) ────────────────────────────────────────────────────────────

variable "alb_target_group_arn" {
  description = "ARN of an existing ALB target group to register the task with. Leave empty to skip."
  type        = string
  default     = ""
}

variable "tags" {
  description = "Tags applied to all resources"
  type        = map(string)
  default     = {}
}
