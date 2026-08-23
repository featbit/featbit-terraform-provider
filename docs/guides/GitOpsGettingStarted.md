---
page_title: "FeatBit Terraform GitOps Tutorial"
---

<a id="top"></a>

# FeatBit Terraform GitOps Tutorial

This hands-on tutorial shows how to use the
[FeatBit Terraform Provider](https://registry.terraform.io/providers/featbit/featbit/latest/docs)
as the infrastructure-as-code layer of a GitOps workflow. You will evolve one
Terraform configuration that manages a FeatBit Project, Dev, Stage, Prod,
Segments, and Feature Flag definitions, then validate and promote runtime
behavior across those Environments.

[Get started](#getting-started) · [Build the baseline](#create-resources) ·
[Validate and promote](#validate-dev) · [Change a Flag](#add-feature-flag) ·
[Clean up](#cleanup)

## How this tutorial maps to GitOps

GitOps evolves declarative configuration through reviewed changes against
durable state. The numbered steps therefore keep adding to and modifying the
same Terraform root instead of creating a separate project for each step.

For hands-on safety, keep the tutorial files in the ignored `mypractice/`
directory instead of editing the Provider repository's checked-in examples.
This demonstrates the Terraform change model, not end-to-end GitOps
automation. In a real GitOps repository, track the equivalent HCL in Git,
review its plans, and use CI/CD to apply approved changes.

## What you will build

The baseline configuration creates:

| Environment | Feature Flag definitions | Segment |
|---|---|---|
| `Dev` (`dev`) | `checkout-one-page`, `checkout-payment-copy` | Dev `checkout-beta-users` |
| `Stage` (`stage`) | `checkout-one-page`, `checkout-payment-copy` | Stage `checkout-beta-users` |
| `Prod` (`prod`) | `checkout-one-page`, `checkout-payment-copy` | Prod `checkout-beta-users` |

Each Segment is intentionally empty and demonstrates only
Environment-specific creation, metadata, and scope.

Later steps add `checkout-address-validation` to all three Environments with
one catalog change, then remove it from all three through Terraform.

Terraform owns the Project, Stage, Flag definitions, and
Environment-specific Segments. FeatBit creates Dev and Prod with the Project.
Use the FeatBit UI for Flag enabled state, targeting, and rollouts that
Provider `v0.2.x` cannot express.

<a id="getting-started"></a>

## Before you begin

You need:

| Tool or access | Requirement |
|---|---|
| Terraform | `>= 1.5.0, < 2.0.0` |
| Command shell | PowerShell `>= 7` or Bash `>= 3.2` |
| FeatBit service token | Permission to create and delete the isolated Demo Project |
| FeatBit Organization key | Used to build Environment-specific Segment scopes |
| Application or SDK test harness | Can evaluate Flags for supplied user keys |

From the repository root, create and enter the ignored practice directory.

**PowerShell**

```powershell
New-Item -ItemType Directory -Force mypractice | Out-Null
Set-Location mypractice
```

**Bash**

```bash
mkdir -p mypractice
cd mypractice
```

Run every remaining tutorial command from `mypractice/`. Do not create a
`setup/` directory and do not use `terraform -chdir`.

If you applied an earlier multi-root revision of this tutorial, do not place
the new files over those old states. Destroy the disposable Demo with the old
configuration or migrate its state first.

The tutorial uses local state. Keep `terraform.tfstate` until cleanup and never
commit it. A real team should configure one encrypted, versioned, locked remote
backend for the same root.

## Step 1: Create the Project and Stage

Create the initial files.

**PowerShell**

```powershell
@(
  "versions.tf",
  "variables.tf",
  "project.tf"
) | ForEach-Object {
  New-Item -ItemType File -Force $_ | Out-Null
}
```

**Bash**

```bash
touch versions.tf variables.tf project.tf
```

Put this in `versions.tf`:

```hcl
terraform {
  required_version = ">= 1.5.0, < 2.0.0"

  required_providers {
    featbit = {
      source  = "featbit/featbit"
      version = "= 0.2.0"
    }
  }
}

provider "featbit" {}
```

If this practice root was already initialized with Provider `v0.1.x` or a
`v0.2.0` prerelease, update the constraint above and refresh the dependency
lock selection before continuing:

```console
terraform init -upgrade -input=false -no-color
```

The command upgrades the Provider dependency; it does not change the managed
FeatBit resources by itself. Review the next `terraform plan` as usual.

Put this in `variables.tf`:

```hcl
variable "demo_suffix" {
  description = "Stable lowercase suffix for the Demo Project. Change it before the first apply and keep it unchanged."
  type        = string
  default     = "reference"

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9-]{2,23}$", var.demo_suffix))
    error_message = "demo_suffix must contain 3 through 24 lowercase letters, digits, or hyphens and start with a letter or digit."
  }
}

variable "organization_key" {
  description = "FeatBit Organization key used to construct Environment-specific Segment scope RNs."
  type        = string
  sensitive   = true
}
```

Replace `reference` with a stable suffix that is unique in your FeatBit
Organization. The suffix is desired configuration, not a credential. Do not
change it after the first apply unless you intend to replace the Demo Project.

Put this in `project.tf`:

```hcl
locals {
  project_key = "tf-gitops-demo-${var.demo_suffix}"
}

resource "featbit_project" "demo" {
  name = "Terraform GitOps Demo ${var.demo_suffix}"
  key  = local.project_key
}

resource "featbit_environment" "stage" {
  project_id  = featbit_project.demo.id
  name        = "Stage"
  key         = "stage"
  description = "Staging environment for the isolated Terraform GitOps demo"
}

locals {
  project_environment_ids = {
    for environment in featbit_project.demo.environments :
    environment.key => environment.id
  }

  environment_keys = toset(["dev", "stage", "prod"])

  environment_ids = {
    dev   = local.project_environment_ids["dev"]
    stage = featbit_environment.stage.id
    prod  = local.project_environment_ids["prod"]
  }
}
```

FeatBit creates Dev and Prod together with the Project. Terraform reads their
IDs from `featbit_project.demo.environments` and explicitly creates only
Stage.

Format, initialize, and validate the root.

**PowerShell**

```powershell
terraform fmt
if ($LASTEXITCODE -ne 0) { throw "terraform fmt failed." }

terraform init -input=false -no-color
if ($LASTEXITCODE -ne 0) { throw "terraform init failed." }

terraform validate -no-color
if ($LASTEXITCODE -ne 0) { throw "terraform validate failed." }
```

**Bash**

```bash
if ! terraform fmt; then
  echo "terraform fmt failed." >&2
  exit 1
fi

if ! terraform init -input=false -no-color; then
  echo "terraform init failed." >&2
  exit 1
fi

if ! terraform validate -no-color; then
  echo "terraform validate failed." >&2
  exit 1
fi
```

### Provide credentials in the current shell

Prefer a secret manager that injects `FEATBIT_ACCESS_TOKEN` and
`FEATBIT_ORGANIZATION_KEY` into the Terraform process. For local practice,
these prompts mask the values and keep them out of command history.

**PowerShell**

```powershell
if ([string]::IsNullOrWhiteSpace($env:FEATBIT_ACCESS_TOKEN)) {
  $env:FEATBIT_ACCESS_TOKEN = Read-Host "FeatBit service token" -MaskInput
}

if ([string]::IsNullOrWhiteSpace($env:FEATBIT_ORGANIZATION_KEY)) {
  $env:FEATBIT_ORGANIZATION_KEY = Read-Host "FeatBit Organization key" -MaskInput
}

$env:TF_VAR_organization_key = $env:FEATBIT_ORGANIZATION_KEY
```

**Bash**

```bash
if [[ -z "${FEATBIT_ACCESS_TOKEN:-}" ]]; then
  read -rsp "FeatBit service token: " FEATBIT_ACCESS_TOKEN
  printf '\n'
  export FEATBIT_ACCESS_TOKEN
fi

if [[ -z "${FEATBIT_ORGANIZATION_KEY:-}" ]]; then
  read -rsp "FeatBit Organization key: " FEATBIT_ORGANIZATION_KEY
  printf '\n'
  export FEATBIT_ORGANIZATION_KEY
fi

export TF_VAR_organization_key="$FEATBIT_ORGANIZATION_KEY"
```

FeatBit Cloud uses the Provider's default API URL. Only for another public
endpoint, set `FEATBIT_API_URL` in the same process:

**PowerShell**

```powershell
$env:FEATBIT_API_URL = "https://your-featbit-api.example.com"
```

**Bash**

```bash
export FEATBIT_API_URL="https://your-featbit-api.example.com"
```

Skip those optional API URL commands when you use FeatBit Cloud.

Create and apply a saved plan.

**PowerShell**

```powershell
terraform plan -out=tfplan
if ($LASTEXITCODE -ne 0) { throw "terraform plan failed." }

terraform apply tfplan
if ($LASTEXITCODE -ne 0) { throw "terraform apply failed." }

Remove-Item tfplan -ErrorAction SilentlyContinue

terraform plan
if ($LASTEXITCODE -ne 0) { throw "the verification plan failed." }
```

**Bash**

```bash
if ! terraform plan -out=tfplan; then
  echo "terraform plan failed." >&2
  exit 1
fi

if ! terraform apply tfplan; then
  echo "terraform apply failed." >&2
  exit 1
fi

rm -f tfplan

if ! terraform plan; then
  echo "the verification plan failed." >&2
  exit 1
fi
```

The first plan should report:

```text
Plan: 2 to add, 0 to change, 0 to destroy.
```

It adds `featbit_project.demo` and `featbit_environment.stage`. Dev and
Prod do not appear as separate Environment resources because FeatBit creates
them with the Project. The verification plan must report `No changes`.

## Step 2: Add Segments and Feature Flags to the same root

Create two more files in the current `mypractice/` directory.

**PowerShell**

```powershell
@(
  "segments.tf",
  "feature-flags.tf"
) | ForEach-Object {
  New-Item -ItemType File -Force $_ | Out-Null
}
```

**Bash**

```bash
touch segments.tf feature-flags.tf
```

Put this in `segments.tf`:

```hcl
resource "featbit_segment" "checkout_beta_users" {
  for_each = local.environment_keys

  environment_id = local.environment_ids[each.key]
  key            = "checkout-beta-users"
  name           = "Checkout beta users"
  description    = "Empty Segment used to demonstrate Environment-specific lifecycle"
  scopes = toset([
    "organization/${var.organization_key}:project/${featbit_project.demo.key}:env/${each.key}"
  ])
}
```

Provider `v0.2.x` can store included or excluded keys and rule payloads in
Segment targeting, including custom-property rules. It cannot use the
documented public API to create missing Environment End Users or register
missing custom-property metadata. Those targeting forms therefore require
prerequisites outside Terraform and are not a complete workflow in this
tutorial.

Rules over built-in properties such as `keyId` and `name` do not require
custom-property metadata, but this tutorial deliberately demonstrates no
Segment targeting at all.

For that reason, this example configures no included users, excluded users,
rules, or tags. The Segment is a lifecycle and scope example only. Do not add
targeting or tags to this Terraform-owned Segment in the UI: a later apply
will restore the configured empty values.

Put this in `feature-flags.tf`:

```hcl
locals {
  feature_flags = {
    "checkout-one-page" = {
      name           = "One-page checkout"
      description    = "Selects whether checkout uses the one-page experience"
      variation_type = "boolean"
      variations = [
        {
          name  = "Disabled"
          value = "false"
        },
        {
          name  = "Enabled"
          value = "true"
        }
      ]
    }

    "checkout-payment-copy" = {
      name           = "Checkout payment copy"
      description    = "Selects the payment messaging shown during checkout"
      variation_type = "string"
      variations = [
        {
          name  = "Standard"
          value = "standard"
        },
        {
          name  = "Simplified"
          value = "simplified"
        }
      ]
    }
  }

  feature_flag_instances = merge([
    for environment_key in local.environment_keys : {
      for flag_key, flag in local.feature_flags :
      "${environment_key}/${flag_key}" => {
        environment_key = environment_key
        flag_key        = flag_key
        flag            = flag
      }
    }
  ]...)
}

resource "featbit_feature_flag" "this" {
  for_each = local.feature_flag_instances

  environment_id = local.environment_ids[each.value.environment_key]
  key            = each.value.flag_key
  name           = each.value.flag.name
  description    = each.value.flag.description
  variation_type = each.value.flag.variation_type
  variations     = each.value.flag.variations
}
```

The keys of `feature_flag_instances` combine Environment and Flag keys, so
Terraform creates one resource per Flag per Environment. All references remain
inside the same Terraform graph; no ID is copied through shell variables,
`.tfvars`, or remote state.

Reformat, reinitialize, and validate after adding the files.

**PowerShell**

```powershell
terraform fmt
if ($LASTEXITCODE -ne 0) { throw "terraform fmt failed." }

terraform init -input=false -no-color
if ($LASTEXITCODE -ne 0) { throw "terraform init failed." }

terraform validate -no-color
if ($LASTEXITCODE -ne 0) { throw "terraform validate failed." }
```

**Bash**

```bash
if ! terraform fmt; then
  echo "terraform fmt failed." >&2
  exit 1
fi

if ! terraform init -input=false -no-color; then
  echo "terraform init failed." >&2
  exit 1
fi

if ! terraform validate -no-color; then
  echo "terraform validate failed." >&2
  exit 1
fi
```

<a id="create-resources"></a>

## Step 3: Apply the Segment and Feature Flag definitions

Plan and apply the same root once.

**PowerShell**

```powershell
terraform plan -out=tfplan
if ($LASTEXITCODE -ne 0) { throw "terraform plan failed." }

terraform apply tfplan
if ($LASTEXITCODE -ne 0) { throw "terraform apply failed." }

Remove-Item tfplan -ErrorAction SilentlyContinue

terraform plan
if ($LASTEXITCODE -ne 0) { throw "the verification plan failed." }
```

**Bash**

```bash
if ! terraform plan -out=tfplan; then
  echo "terraform plan failed." >&2
  exit 1
fi

if ! terraform apply tfplan; then
  echo "terraform apply failed." >&2
  exit 1
fi

rm -f tfplan

if ! terraform plan; then
  echo "the verification plan failed." >&2
  exit 1
fi
```

Because Step 1 already created the Project and Stage, the first plan should
report:

```text
Plan: 9 to add, 0 to change, 0 to destroy.
```

It adds six `featbit_feature_flag` resources and three `featbit_segment`
resources. Confirm that each Environment has the two disabled Flags and its
own empty `checkout-beta-users` Segment with the expected name, description,
key, and Environment scope. The verification plan must report `No changes`.

If this state already manages those nine resources from an earlier tutorial
revision, do not destroy them merely to replay the step. Update the HCL,
review the in-place removal of the old Segment users, rules, and tags, apply
it, then require a second plan to report `No changes`.

<a id="validate-dev"></a>

## Step 4: Validate in Dev

The empty Segment is not part of runtime validation and is not referenced by
a Feature Flag. In Dev, open `checkout-one-page`, select `true` as its default
result, enable the Flag, and evaluate it through your application or SDK with
a synthetic test context. Confirm that the evaluation returns `true`.

Next, select `simplified` for `checkout-payment-copy` in Dev and verify the
payment-copy change.

## Step 5: Promote runtime settings through Stage to Prod

Provider `v0.2.x` does not automate cross-Environment settings promotion.
Feature Flag and Segment settings promotion remain FeatBit UI workflows.

Terraform has already created matching Flag definitions and an empty Segment
in all three Environments. The Segments are topology examples and do not take
part in runtime promotion. Promote Flag behavior with FeatBit
[Compare and Copy Settings](https://docs.featbit.co/feature-flags/organizing-flags/compare-and-copy-settings):

1. Compare Dev with Stage and copy only compatible settings.
2. Validate the copied behavior through your application or SDK in Stage.
3. Compare Stage with Prod and copy only compatible settings.
4. Validate through your application or SDK in Prod before increasing
   exposure.

<a id="add-feature-flag"></a>

## Step 6: Add another Feature Flag

Add this entry inside `local.feature_flags` in `feature-flags.tf`:

```hcl
"checkout-address-validation" = {
  name           = "Checkout address validation"
  description    = "Selects whether checkout uses enhanced address validation"
  variation_type = "boolean"
  variations = [
    {
      name  = "Disabled"
      value = "false"
    },
    {
      name  = "Enabled"
      value = "true"
    }
  ]
}
```

Format, validate, and apply the same root.

**PowerShell**

```powershell
terraform fmt
if ($LASTEXITCODE -ne 0) { throw "terraform fmt failed." }

terraform validate -no-color
if ($LASTEXITCODE -ne 0) { throw "terraform validate failed." }

terraform plan -out=tfplan
if ($LASTEXITCODE -ne 0) { throw "terraform plan failed." }

terraform apply tfplan
if ($LASTEXITCODE -ne 0) { throw "terraform apply failed." }

Remove-Item tfplan -ErrorAction SilentlyContinue

terraform plan
if ($LASTEXITCODE -ne 0) { throw "the verification plan failed." }
```

**Bash**

```bash
if ! terraform fmt; then
  echo "terraform fmt failed." >&2
  exit 1
fi

if ! terraform validate -no-color; then
  echo "terraform validate failed." >&2
  exit 1
fi

if ! terraform plan -out=tfplan; then
  echo "terraform plan failed." >&2
  exit 1
fi

if ! terraform apply tfplan; then
  echo "terraform apply failed." >&2
  exit 1
fi

rm -f tfplan

if ! terraform plan; then
  echo "the verification plan failed." >&2
  exit 1
fi
```

The plan should add exactly one Flag in each Environment:

```text
Plan: 3 to add, 0 to change, 0 to destroy.
```

The verification plan must report `No changes`. Confirm that the new Flag
exists and remains disabled in Dev, Stage, and Prod. Validate its runtime
behavior in Dev before promotion.

## Step 7: Remove a Feature Flag

Remove the `checkout-address-validation` entry that you added to
`local.feature_flags` in `feature-flags.tf`. Then format, validate, and apply
the same root.

**PowerShell**

```powershell
terraform fmt
if ($LASTEXITCODE -ne 0) { throw "terraform fmt failed." }

terraform validate -no-color
if ($LASTEXITCODE -ne 0) { throw "terraform validate failed." }

terraform plan -out=tfplan
if ($LASTEXITCODE -ne 0) { throw "terraform plan failed." }

terraform apply tfplan
if ($LASTEXITCODE -ne 0) { throw "terraform apply failed." }

Remove-Item tfplan -ErrorAction SilentlyContinue

terraform plan
if ($LASTEXITCODE -ne 0) { throw "the verification plan failed." }
```

**Bash**

```bash
if ! terraform fmt; then
  echo "terraform fmt failed." >&2
  exit 1
fi

if ! terraform validate -no-color; then
  echo "terraform validate failed." >&2
  exit 1
fi

if ! terraform plan -out=tfplan; then
  echo "terraform plan failed." >&2
  exit 1
fi

if ! terraform apply tfplan; then
  echo "terraform apply failed." >&2
  exit 1
fi

rm -f tfplan

if ! terraform plan; then
  echo "the verification plan failed." >&2
  exit 1
fi
```

The first plan should remove exactly one Flag from each Environment:

```text
Plan: 0 to add, 0 to change, 3 to destroy.
```

Applying this plan permanently deletes the Flag from Dev, Stage, and Prod.
Provider `v0.2.x` cannot keep a Terraform-managed Flag archived. The
verification plan must report `No changes`.

<a id="cleanup"></a>

## Step 8: Roll back and clean up

If runtime behavior is unsafe, first disable the affected Prod Flag or restore
its last validated targeting and default result in FeatBit. Keep the Terraform
definition while correcting the change.

For a Terraform-owned change, revert or correct the HCL, review one root plan,
and apply the reviewed correction. Do not edit state manually.

The tutorial creates no Flag-to-Segment references, so no unlinking step is
needed. If you added such a reference outside the tutorial, remove it before
destroying the Segment. Then create and apply one destroy plan.

**PowerShell**

```powershell
terraform plan -destroy -out=tfplan
if ($LASTEXITCODE -ne 0) { throw "terraform destroy plan failed." }

terraform apply tfplan
if ($LASTEXITCODE -ne 0) { throw "terraform destroy apply failed." }

Remove-Item tfplan -ErrorAction SilentlyContinue
```

**Bash**

```bash
if ! terraform plan -destroy -out=tfplan; then
  echo "terraform destroy plan failed." >&2
  exit 1
fi

if ! terraform apply tfplan; then
  echo "terraform destroy apply failed." >&2
  exit 1
fi

rm -f tfplan
```

Review the destroy plan before applying it and confirm that it contains only
the isolated Demo tree. Terraform removes Flags before Segments, Stage before
the Project, and the Project together with its default Dev and Prod
Environments.

Clear process-scoped credentials:

**PowerShell**

```powershell
@(
  "FEATBIT_ACCESS_TOKEN",
  "FEATBIT_ORGANIZATION_KEY",
  "FEATBIT_API_URL",
  "TF_VAR_organization_key"
) | ForEach-Object {
  Remove-Item "Env:$_" -ErrorAction SilentlyContinue
}
```

**Bash**

```bash
unset FEATBIT_ACCESS_TOKEN FEATBIT_ORGANIZATION_KEY FEATBIT_API_URL TF_VAR_organization_key
```

## Further reading

- [FeatBit Terraform Provider](https://registry.terraform.io/providers/featbit/featbit/latest/docs)
- [FeatBit Connect an SDK](https://docs.featbit.co/getting-started/connect-an-sdk)
- [FeatBit Compare and Copy Settings](https://docs.featbit.co/feature-flags/organizing-flags/compare-and-copy-settings)

[Back to top](#top)
