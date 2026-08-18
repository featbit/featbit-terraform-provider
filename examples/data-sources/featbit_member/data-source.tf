variable "member_email" {
  description = "Organization-scoped full email of an existing FeatBit Member."
  type        = string
  sensitive   = true
}

data "featbit_member" "exact" {
  email = var.member_email
}
