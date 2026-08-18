variable "group_name" {
  description = "Organization-scoped, case-sensitive exact Group name."
  type        = string
}

variable "policy_key" {
  description = "Organization-scoped, case-sensitive exact custom or built-in Policy key."
  type        = string
}

data "featbit_group" "target" {
  name = var.group_name
}

data "featbit_policy" "target" {
  key = var.policy_key
}

resource "featbit_group_policy_binding" "example" {
  group_id  = data.featbit_group.target.id
  policy_id = data.featbit_policy.target.id
}
