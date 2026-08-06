variable "project_id" {
  description = "Exact parent FeatBit Project UUID."
  type        = string
}

variable "environment_id" {
  description = "Exact FeatBit Environment UUID."
  type        = string
}

data "featbit_environment" "exact" {
  project_id = var.project_id
  id         = var.environment_id
}
