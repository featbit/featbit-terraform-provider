resource "featbit_project" "example" {
  name = "Checkout service"
  key  = "checkout-service"
}

resource "featbit_environment" "example" {
  project_id = featbit_project.example.id
  name       = "Staging"
  key        = "staging"
}

resource "featbit_feature_flag" "example" {
  environment_id = featbit_environment.example.id
  name           = "New checkout"
  key            = "new-checkout"
  description    = "Selects the checkout implementation"
  variation_type = "boolean"

  variations = [
    {
      name  = "Enabled"
      value = "true"
    },
    {
      name  = "Disabled"
      value = "false"
    },
  ]
}
