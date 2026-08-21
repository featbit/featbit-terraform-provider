variable "member_emails_by_alias" {
  description = "Non-identifying aliases mapped to organization-scoped full emails of existing FeatBit Members."
  type        = map(string)
  sensitive   = true
}

variable "policy_key" {
  description = "Organization-scoped, case-sensitive exact custom or built-in Policy key."
  type        = string
}

locals {
  member_aliases = nonsensitive(toset(keys(var.member_emails_by_alias)))
}

data "featbit_member" "target" {
  for_each = local.member_aliases

  email = var.member_emails_by_alias[each.key]
}

data "featbit_policy" "target" {
  key = var.policy_key
}

resource "featbit_member_policy_binding" "example" {
  for_each = local.member_aliases

  member_id = data.featbit_member.target[each.key].id
  policy_id = data.featbit_policy.target.id
}
