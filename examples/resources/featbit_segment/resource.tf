variable "featbit_organization_key" {
  description = "FeatBit organization key used in the Environment scope RN."
  type        = string
}

locals {
  project_key     = "checkout-service"
  environment_key = "staging"
}

resource "featbit_project" "example" {
  name = "Checkout service"
  key  = local.project_key
}

resource "featbit_environment" "example" {
  project_id = featbit_project.example.id
  name       = "Staging"
  key        = local.environment_key
}

resource "featbit_segment" "example" {
  environment_id = featbit_environment.example.id
  name           = "Checkout beta users"
  key            = "checkout-beta-users"
  description    = "Users selected by a built-in key rule"
  scopes = [
    "organization/${var.featbit_organization_key}:project/${local.project_key}:env/${local.environment_key}",
  ]

  rules = [
    {
      name = "Beta user key"
      conditions = [
        {
          property = "keyId"
          operator = "IsOneOf"
          value    = jsonencode(["checkout-beta-user"])
        },
      ]
    },
  ]

  tags = ["beta", "checkout"]
}
