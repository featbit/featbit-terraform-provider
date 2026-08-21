terraform {
  required_version = ">= 1.0.0"

  required_providers {
    featbit = {
      source  = "featbit/featbit"
      version = "= 0.2.0-beta.2"
    }
  }
}

# Set FEATBIT_ACCESS_TOKEN in the process environment. Keep token values out
# of Terraform configuration, variable defaults, plans, state, and logs.
provider "featbit" {
  # Omit api_url for FeatBit Cloud. For another documented public API root,
  # uncomment the next line or set FEATBIT_API_URL to the same form.
  # api_url = "https://featbit.example.com/api/v1"
}
