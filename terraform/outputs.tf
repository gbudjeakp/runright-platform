output "cluster_arn" {
  description = "ECS cluster ARN"
  value       = aws_ecs_cluster.this.arn
}

output "service_name" {
  description = "ECS service name"
  value       = aws_ecs_service.this.name
}

output "task_security_group_id" {
  description = "Security group ID attached to the ECS task"
  value       = aws_security_group.ecs_task.id
}

output "api_key_secret_arn" {
  description = "Secrets Manager ARN for the RunRight API key"
  value       = aws_secretsmanager_secret.api_key.arn
}

output "rds_endpoint" {
  description = "RDS instance endpoint (empty when create_rds = false)"
  value       = var.create_rds ? aws_db_instance.this[0].address : ""
}

output "log_group_name" {
  description = "CloudWatch log group name"
  value       = aws_cloudwatch_log_group.this.name
}
