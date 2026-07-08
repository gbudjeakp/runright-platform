output "namespace" {
  description = "Kubernetes namespace RunRight was deployed into"
  value       = kubernetes_namespace.this.metadata[0].name
}

output "helm_release_name" {
  description = "Helm release name"
  value       = helm_release.runright.name
}

output "rds_endpoint" {
  description = "RDS instance endpoint (empty when create_rds = false)"
  value       = var.create_rds ? aws_db_instance.this[0].address : ""
}
