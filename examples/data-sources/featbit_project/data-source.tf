variable "project_id" {
  description = "Exact FeatBit Project UUID."
  type        = string
}

data "featbit_project" "exact" {
  id = var.project_id
}
