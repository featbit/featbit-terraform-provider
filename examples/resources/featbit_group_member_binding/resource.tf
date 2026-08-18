variable "group_name" {
  description = "Organization-scoped, case-sensitive exact Group name."
  type        = string
}

variable "member_email" {
  description = "Organization-scoped full email of an existing FeatBit Member."
  type        = string
  sensitive   = true
}

data "featbit_group" "target" {
  name = var.group_name
}

data "featbit_member" "target" {
  email = var.member_email
}

resource "featbit_group_member_binding" "example" {
  group_id  = data.featbit_group.target.id
  member_id = data.featbit_member.target.id
}
