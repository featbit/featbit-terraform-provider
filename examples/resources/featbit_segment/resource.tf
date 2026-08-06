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
  name           = "North American beta users"
  key            = "north-american-beta-users"
  description    = "Users enrolled in the regional beta"
  scopes = [
    "organization/${var.featbit_organization_key}:project/${local.project_key}:env/${local.environment_key}",
  ]

  included_users = ["beta-user-a", "beta-user-b"]
  excluded_users = ["beta-user-blocked"]

  rules = [
    {
      name = "Supported countries"
      conditions = [
        {
          property = "country"
          operator = "IsOneOf"
          value    = jsonencode(["CA", "US"])
        },
      ]
    },
  ]

  tags = ["beta", "checkout"]
}
