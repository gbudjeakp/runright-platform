terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
  }
}

locals {
  name = var.name
  dsn  = var.create_rds ? "postgres://runright:${var.db_password}@${aws_db_instance.this[0].address}:5432/runright?sslmode=require" : var.external_dsn
  tags = merge(var.tags, {
    "managed-by"  = "terraform"
    "application" = "runright"
    "environment" = var.environment
  })
}

# ── CloudWatch log group ───────────────────────────────────────────────────────

resource "aws_cloudwatch_log_group" "this" {
  name              = "/ecs/${local.name}"
  retention_in_days = var.log_retention_days
  tags              = local.tags
}

# ── Secrets Manager ───────────────────────────────────────────────────────────

resource "aws_secretsmanager_secret" "dsn" {
  name                    = "${local.name}/dsn"
  recovery_window_in_days = 0
  tags                    = local.tags
}

resource "aws_secretsmanager_secret_version" "dsn" {
  secret_id     = aws_secretsmanager_secret.dsn.id
  secret_string = local.dsn
}

resource "aws_secretsmanager_secret" "api_key" {
  name                    = "${local.name}/api-key"
  recovery_window_in_days = 0
  tags                    = local.tags
}

resource "aws_secretsmanager_secret_version" "api_key" {
  count         = var.api_key != "" ? 1 : 0
  secret_id     = aws_secretsmanager_secret.api_key.id
  secret_string = var.api_key
}

# ── IAM — task execution role ──────────────────────────────────────────────────

data "aws_iam_policy_document" "ecs_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "execution" {
  name               = "${local.name}-ecs-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
  tags               = local.tags
}

resource "aws_iam_role_policy_attachment" "execution_managed" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

data "aws_iam_policy_document" "secrets_read" {
  statement {
    actions   = ["secretsmanager:GetSecretValue"]
    resources = [aws_secretsmanager_secret.dsn.arn, aws_secretsmanager_secret.api_key.arn]
  }
}

resource "aws_iam_role_policy" "secrets_read" {
  name   = "secrets-read"
  role   = aws_iam_role.execution.id
  policy = data.aws_iam_policy_document.secrets_read.json
}

# ── Security groups ────────────────────────────────────────────────────────────

resource "aws_security_group" "ecs_task" {
  name        = "${local.name}-ecs-task"
  description = "RunRight ECS task — inbound from allowed CIDRs"
  vpc_id      = var.vpc_id
  tags        = local.tags

  ingress {
    description = "RunRight API"
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = var.allowed_cidrs
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "rds" {
  count       = var.create_rds ? 1 : 0
  name        = "${local.name}-rds"
  description = "RunRight RDS — inbound from ECS task only"
  vpc_id      = var.vpc_id
  tags        = local.tags

  ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_task.id]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# ── RDS PostgreSQL ─────────────────────────────────────────────────────────────

resource "aws_db_subnet_group" "this" {
  count      = var.create_rds ? 1 : 0
  name       = local.name
  subnet_ids = var.private_subnet_ids
  tags       = local.tags
}

resource "aws_db_instance" "this" {
  count = var.create_rds ? 1 : 0

  identifier        = local.name
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
  final_snapshot_identifier = "${local.name}-final"
  deletion_protection       = true
  backup_retention_period   = 7
  storage_encrypted         = true

  tags = local.tags
}

# ── ECS cluster ────────────────────────────────────────────────────────────────

resource "aws_ecs_cluster" "this" {
  name = local.name
  tags = local.tags

  setting {
    name  = "containerInsights"
    value = "enabled"
  }
}

resource "aws_ecs_cluster_capacity_providers" "this" {
  cluster_name       = aws_ecs_cluster.this.name
  capacity_providers = ["FARGATE", "FARGATE_SPOT"]
}

# ── ECS task definition ────────────────────────────────────────────────────────

data "aws_region" "current" {}

resource "aws_ecs_task_definition" "this" {
  family                   = local.name
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.task_cpu
  memory                   = var.task_memory
  execution_role_arn       = aws_iam_role.execution.arn
  tags                     = local.tags

  container_definitions = jsonencode([{
    name      = "runright"
    image     = "${var.image_repository}:${var.image_tag}"
    essential = true

    portMappings = [{
      containerPort = 8080
      protocol      = "tcp"
    }]

    environment = [
      { name = "RUNRIGHT_LOG_LEVEL", value = var.log_level }
    ]

    secrets = concat(
      [{ name = "RUNRIGHT_DSN", valueFrom = aws_secretsmanager_secret.dsn.arn }],
      var.api_key != "" ? [{ name = "RUNRIGHT_API_KEY", valueFrom = aws_secretsmanager_secret.api_key.arn }] : []
    )

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.this.name
        "awslogs-region"        = data.aws_region.current.name
        "awslogs-stream-prefix" = "ecs"
      }
    }

    healthCheck = {
      command     = ["CMD-SHELL", "wget -qO- http://localhost:8080/healthz || exit 1"]
      interval    = 30
      timeout     = 5
      retries     = 3
      startPeriod = 15
    }
  }])
}

# ── ECS service ────────────────────────────────────────────────────────────────

resource "aws_ecs_service" "this" {
  name            = local.name
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.this.arn
  desired_count   = var.desired_count
  launch_type     = "FARGATE"
  tags            = local.tags

  network_configuration {
    subnets          = var.private_subnet_ids
    security_groups  = [aws_security_group.ecs_task.id]
    assign_public_ip = false
  }

  dynamic "load_balancer" {
    for_each = var.alb_target_group_arn != "" ? [1] : []
    content {
      target_group_arn = var.alb_target_group_arn
      container_name   = "runright"
      container_port   = 8080
    }
  }

  lifecycle {
    ignore_changes = [desired_count]
  }
}

# ── Data sources ──────────────────────────────────────────────────────────────

data "aws_region" "current" {}
data "aws_caller_identity" "current" {}

locals {
  name_prefix = "${var.name}-${var.environment}"
  account_id  = data.aws_caller_identity.current.account_id
  region      = data.aws_region.current.name
}

# ── ECS Cluster ───────────────────────────────────────────────────────────────

resource "aws_ecs_cluster" "this" {
  name = local.name_prefix

  setting {
    name  = "containerInsights"
    value = "enabled"
  }

  tags = var.tags
}

# ── CloudWatch log group ───────────────────────────────────────────────────────

resource "aws_cloudwatch_log_group" "this" {
  name              = "/ecs/${local.name_prefix}"
  retention_in_days = var.log_retention_days
  tags              = var.tags
}

# ── IAM: ECS task execution role ──────────────────────────────────────────────

resource "aws_iam_role" "ecs_exec" {
  name = "${local.name_prefix}-ecs-exec"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })

  tags = var.tags
}

resource "aws_iam_role_policy_attachment" "ecs_exec_managed" {
  role       = aws_iam_role.ecs_exec.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# Allow ECS to pull the api key secret from Secrets Manager.
resource "aws_iam_role_policy" "ecs_exec_secrets" {
  name = "secrets"
  role = aws_iam_role.ecs_exec.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["secretsmanager:GetSecretValue"]
      Resource = [aws_secretsmanager_secret.api_key.arn]
    }]
  })
}

# ── Secrets Manager: API key ──────────────────────────────────────────────────

resource "aws_secretsmanager_secret" "api_key" {
  name                    = "${local.name_prefix}/api-key"
  recovery_window_in_days = 7
  tags                    = var.tags
}

resource "aws_secretsmanager_secret_version" "api_key" {
  secret_id     = aws_secretsmanager_secret.api_key.id
  secret_string = var.api_key
}

# ── RDS PostgreSQL (optional — skip if using external DSN) ───────────────────

resource "aws_db_subnet_group" "this" {
  count      = var.create_rds ? 1 : 0
  name       = local.name_prefix
  subnet_ids = var.private_subnet_ids
  tags       = var.tags
}

resource "aws_security_group" "rds" {
  count       = var.create_rds ? 1 : 0
  name        = "${local.name_prefix}-rds"
  description = "RunRight RDS"
  vpc_id      = var.vpc_id

  ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_task.id]
  }

  tags = var.tags
}

resource "aws_db_instance" "this" {
  count = var.create_rds ? 1 : 0

  identifier        = local.name_prefix
  engine            = "postgres"
  engine_version    = "16"
  instance_class    = var.rds_instance_class
  allocated_storage = 20
  storage_encrypted = true

  db_name  = "runright"
  username = "runright"
  password = var.db_password

  db_subnet_group_name   = aws_db_subnet_group.this[0].name
  vpc_security_group_ids = [aws_security_group.rds[0].id]
  skip_final_snapshot    = var.environment != "production"

  tags = var.tags
}

# ── Security group: ECS task ──────────────────────────────────────────────────

resource "aws_security_group" "ecs_task" {
  name        = "${local.name_prefix}-task"
  description = "RunRight ECS task"
  vpc_id      = var.vpc_id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = var.tags
}

resource "aws_security_group_rule" "ecs_task_http" {
  type              = "ingress"
  from_port         = 8080
  to_port           = 8080
  protocol          = "tcp"
  security_group_id = aws_security_group.ecs_task.id
  cidr_blocks       = var.allowed_cidrs
}

# ── ECS task definition ───────────────────────────────────────────────────────

locals {
  dsn = var.create_rds ? (
    "postgres://runright:${var.db_password}@${aws_db_instance.this[0].address}:5432/runright?sslmode=require"
  ) : var.external_dsn
}

resource "aws_ecs_task_definition" "this" {
  family                   = local.name_prefix
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = var.task_cpu
  memory                   = var.task_memory
  execution_role_arn       = aws_iam_role.ecs_exec.arn

  container_definitions = jsonencode([{
    name      = "runright"
    image     = "${var.image_repository}:${var.image_tag}"
    essential = true

    command = ["serve"]

    environment = [
      { name = "RUNRIGHT_DSN",       value = local.dsn },
      { name = "RUNRIGHT_LOG_LEVEL", value = var.log_level },
    ]

    secrets = [
      { name = "RUNRIGHT_API_KEY", valueFrom = aws_secretsmanager_secret.api_key.arn }
    ]

    portMappings = [{ containerPort = 8080, protocol = "tcp" }]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.this.name
        "awslogs-region"        = local.region
        "awslogs-stream-prefix" = "ecs"
      }
    }

    healthCheck = {
      command     = ["CMD-SHELL", "wget -qO- http://localhost:8080/healthz || exit 1"]
      interval    = 30
      timeout     = 5
      retries     = 3
      startPeriod = 15
    }
  }])

  tags = var.tags
}

# ── ECS Service ───────────────────────────────────────────────────────────────

resource "aws_ecs_service" "this" {
  name            = local.name_prefix
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.this.arn
  desired_count   = var.desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = var.private_subnet_ids
    security_groups  = [aws_security_group.ecs_task.id]
    assign_public_ip = false
  }

  dynamic "load_balancer" {
    for_each = var.alb_target_group_arn != "" ? [1] : []
    content {
      target_group_arn = var.alb_target_group_arn
      container_name   = "runright"
      container_port   = 8080
    }
  }

  lifecycle {
    ignore_changes = [desired_count]
  }

  tags = var.tags
}
