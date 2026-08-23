variable "group_name" {
  description = "Organization-scoped, case-sensitive exact FeatBit Group name."
  type        = string
}

data "featbit_group" "exact" {
  name = var.group_name
}
