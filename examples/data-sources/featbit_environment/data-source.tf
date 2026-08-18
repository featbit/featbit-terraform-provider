variable "project_key" {
  description = "Organization-scoped, case-sensitive exact FeatBit Project key."
  type        = string
}

variable "environment_key" {
  description = "Case-sensitive exact Environment key within the selected Project."
  type        = string
}

data "featbit_project" "parent" {
  key = var.project_key
}

data "featbit_environment" "exact" {
  project_id = data.featbit_project.parent.id
  key        = var.environment_key
}
