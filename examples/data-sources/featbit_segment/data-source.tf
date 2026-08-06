variable "environment_id" {
  description = "Exact Environment UUID from which the Segment is visible."
  type        = string
}

variable "segment_id" {
  description = "Exact FeatBit Segment UUID."
  type        = string
}

data "featbit_segment" "exact" {
  environment_id = var.environment_id
  id             = var.segment_id
}
