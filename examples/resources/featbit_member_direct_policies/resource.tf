variable "member_email" {
  description = "Organization-scoped full email of the existing Member whose direct Policies will be managed."
  type        = string
  sensitive   = true
}

variable "direct_policy_keys" {
  description = "Complete intended set of organization-scoped exact direct Policy keys for the Member."
  type        = set(string)
}

data "featbit_member" "target" {
  email = var.member_email
}

data "featbit_policy" "direct" {
  for_each = var.direct_policy_keys

  key = each.value
}

resource "featbit_member_direct_policies" "example" {
  member_id = data.featbit_member.target.id
  policy_ids = toset([
    for policy in data.featbit_policy.direct : policy.id
  ])
}
