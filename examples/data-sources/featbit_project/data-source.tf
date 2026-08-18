variable "project_key" {
  description = "Organization-scoped, case-sensitive exact FeatBit Project key."
  type        = string
}

data "featbit_project" "exact" {
  key = var.project_key
}
