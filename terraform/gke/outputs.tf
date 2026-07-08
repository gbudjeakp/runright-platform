output "namespace" {
  description = "Kubernetes namespace RunRight was deployed into"
  value       = kubernetes_namespace.this.metadata[0].name
}

output "helm_release_name" {
  description = "Helm release name"
  value       = helm_release.runright.name
}

output "cloud_sql_private_ip" {
  description = "Cloud SQL private IP address (empty when create_cloud_sql = false)"
  value       = var.create_cloud_sql ? google_sql_database_instance.this[0].private_ip_address : ""
}

output "workload_identity_service_account" {
  description = "GCP service account email bound to the Kubernetes workload"
  value       = google_service_account.runright.email
}

output "dsn_secret_name" {
  description = "Secret Manager secret ID for the database DSN"
  value       = google_secret_manager_secret.dsn.secret_id
}
