variable "project_key" {
  description = "Organization-scoped, case-sensitive exact Project key."
  type        = string
}

variable "environment_key" {
  description = "Case-sensitive exact Environment key within the selected Project."
  type        = string
}

data "featbit_project" "target" {
  key = var.project_key
}

data "featbit_environment" "target" {
  project_id = data.featbit_project.target.id
  key        = var.environment_key
}

resource "featbit_policy" "developer" {
  name        = "Checkout developers"
  key         = "checkout-developers"
  description = "Parent visibility plus scoped Flag and Segment operations"

  statements = [
    {
      resource_type = "project"
      effect        = "allow"
      actions       = ["CanAccessProject"]
      resources     = ["project/${data.featbit_project.target.key}"]
    },
    {
      resource_type = "env"
      effect        = "allow"
      actions       = ["CanAccessEnv"]
      resources     = ["project/${data.featbit_project.target.key}:env/${data.featbit_environment.target.key}"]
    },
    {
      resource_type = "flag"
      effect        = "allow"
      actions       = ["ToggleFlag", "UpdateFlagName"]
      resources     = ["project/${data.featbit_project.target.key}:env/${data.featbit_environment.target.key}:flag/*"]
    },
    {
      resource_type = "segment"
      effect        = "allow"
      actions       = ["UpdateSegmentDescription", "UpdateSegmentRules"]
      resources     = ["project/${data.featbit_project.target.key}:env/${data.featbit_environment.target.key}:segment/*"]
    },
  ]
}
