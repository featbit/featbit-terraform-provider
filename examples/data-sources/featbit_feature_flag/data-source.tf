variable "environment_id" {
  description = "Exact parent FeatBit Environment UUID."
  type        = string
}

variable "feature_flag_key" {
  description = "Exact case-sensitive FeatBit Feature Flag key."
  type        = string
}

data "featbit_feature_flag" "exact" {
  environment_id = var.environment_id
  key            = var.feature_flag_key
}
