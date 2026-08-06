resource "featbit_project" "example" {
  name = "Checkout service"
  key  = "checkout-service"
}

resource "featbit_environment" "example" {
  project_id  = featbit_project.example.id
  name        = "Staging"
  key         = "staging"
  description = "Pre-production validation"
}
