---
page_title: "FeatBit Terraform IAM GitOps Tutorial"
---

<a id="top"></a>

# FeatBit Terraform IAM GitOps Tutorial

This hands-on tutorial shows how to use the
[FeatBit Terraform Provider](https://registry.terraform.io/providers/featbit/featbit/latest/docs)
to manage one isolated FeatBit IAM scenario as reviewable Terraform code. You
will evolve one Terraform root that creates a Project and Feature Flags,
defines two complementary custom Policies and one Group, exercises inherited
and direct access with exactly two existing Members, updates scoped
permissions, and then removes direct access, bindings, endpoints, and the
Project in the required dependency order.

[Get started](#getting-started) · [Create the baseline](#create-resources) ·
[Assign access](#assign-access) · [Update a Policy](#update-policy) ·
[Prepare cleanup](#prepare-cleanup) · [Clean up](#cleanup)

## How this tutorial maps to GitOps

GitOps evolves declarative configuration through reviewed changes against
durable state. The numbered steps therefore keep adding to and modifying the
same Terraform root instead of creating a separate root for each IAM object.

For hands-on safety, keep the tutorial files in the ignored
`mypractice/iam/` directory instead of editing the Provider repository's
checked-in examples. This demonstrates Terraform's change and dependency
model, not end-to-end GitOps automation. In a real GitOps repository, track
the equivalent HCL in Git, review every saved plan, and let CI/CD apply only
approved changes.

## What you will build

The baseline configuration creates one Project. FeatBit creates its Dev and
Prod Environments automatically, and Terraform creates three Feature Flag
definitions in each Environment:

| Environment | Feature Flag definitions |
|---|---|
| `Dev` (`dev`) | `checkout-enabled`, `payment-retry`, `fraud-review` |
| `Prod` (`prod`) | `checkout-enabled`, `payment-retry`, `fraud-review` |

The IAM configuration then adds:

| Object | Intended access |
|---|---|
| Project observer Policy | Read-only Project and Environment visibility across Dev and Prod |
| Dev operator Policy | Project visibility plus Dev Environment, Feature Flag, and Segment actions except permanent delete capabilities |
| Project operators Group | Holds both complementary Policies; Member A joins it and inherits their union |
| Existing Member A (`group-member`) | Joins the Project operators Group, has its complete direct Policy set cleared, and receives both Policies only through Group inheritance |
| Existing Member B (`direct-member`) | Stays outside the Group and has a complete direct Policy set containing only the Dev operator Policy |

The Dev operator Policy is later updated with `CanAccessEnv` for Prod and
`ToggleFlag` for only the Prod `checkout-enabled` Flag. It does not gain
access to either sibling Prod Flag.

This tutorial interprets "all actions except delete" conservatively:

- it excludes `DeleteEnv`, `DeleteEnvSecret`, `DeleteFlag`, and
  `DeleteSegment`;
- it does not grant Project deletion to the Dev operator Policy; and
- it keeps reversible `ArchiveFlag`, `RestoreFlag`, `ArchiveSegment`, and
  `RestoreSegment` actions.

The Project observer Policy grants only the parent visibility actions. The Dev
operator Policy enumerates its allowed actions rather than using `*`, because
`*` would also grant deletion.

<a id="getting-started"></a>

## Before you begin

You need:

| Tool or access | Requirement |
|---|---|
| Terraform | `>= 1.5.0, < 2.0.0` |
| Command shell | PowerShell `>= 7` or Bash `>= 3.2` |
| FeatBit API access token | A personal or service token from the target FeatBit Organization, with permission to manage the isolated Project, Feature Flags, custom Policies, Groups, and IAM relationships, and to read both selected Members |
| Existing FeatBit test Members | Exactly two Members with no unrelated effective access to the tutorial Project; Member A must be safe to have every direct Policy removed, while Member B must be safe to have its complete direct set replaced with exactly the Dev operator Policy |
| Member test sessions | A separate login or test harness for each selected Member, kept completely outside Terraform |

Only the API access token is required for Provider authentication. You do
**not** need an Organization key for this tutorial: the public Project and IAM
APIs derive the Organization from the token. The empty `provider "featbit" {}`
block below reads the token from `FEATBIT_ACCESS_TOKEN` and, when needed, the
API origin from `FEATBIT_API_URL`.

Create the token on FeatBit's **Integrations / Access tokens** page, as
described in the official
[API access-token guide](https://docs.featbit.co/integrations/api-access-tokens).
Use the one-time secret value shown immediately after creating a personal or
service token. Do not use a token name or ID, an obscured value from the token
list, an Organization key, a Project or Environment secret, or an SDK key. If
you did not save the one-time value, create a new token; FeatBit does not show
an existing token's secret again.

From the repository root, create and enter the ignored practice directory.

**PowerShell**

```powershell
New-Item -ItemType Directory -Force mypractice/iam | Out-Null
Set-Location mypractice/iam
```

**Bash**

```bash
mkdir -p mypractice/iam
cd mypractice/iam
```

Run every remaining tutorial command from `mypractice/iam/`. Do not create a
second Terraform root and do not use `terraform -chdir`.

The tutorial uses local state. Keep `terraform.tfstate` until every cleanup
step has passed and never commit it. A real team should configure one
encrypted, versioned, locked remote backend for the same root.

<a id="create-resources"></a>

## Step 1: Create the Project and six Feature Flags

Create the initial files.

**PowerShell**

```powershell
@(
  "versions.tf",
  "variables.tf",
  "project.tf",
  "feature-flags.tf"
) | ForEach-Object {
  New-Item -ItemType File -Force $_ | Out-Null
}
```

**Bash**

```bash
touch versions.tf variables.tf project.tf feature-flags.tf
```

Put this in `versions.tf`:

```hcl
terraform {
  required_version = ">= 1.5.0, < 2.0.0"

  required_providers {
    featbit = {
      source  = "featbit/featbit"
      version = "= 0.2.0-beta.2"
    }
  }
}

provider "featbit" {}
```

This manual exercise pins the exact qualified `0.2.0-beta.2` prerelease. Both
Members intentionally use `featbit_member_direct_policies` for complete-set
ownership; the beta.2 additive `featbit_member_policy_binding` remains
available but is not used by this closed-world scenario. Do not substitute
`0.2.0-beta.1` or `latest`.

If this practice root was initialized before selecting `0.2.0-beta.2`, refresh
the dependency lock selection before continuing:

```console
terraform init -upgrade -input=false -no-color
```

The command upgrades only the Provider dependency selection; it does not
change managed FeatBit resources by itself. Review the next `terraform plan`
as usual.

Put this in `variables.tf`:

```hcl
variable "demo_suffix" {
  description = "Stable lowercase suffix for the IAM Demo. Change it before the first apply and keep it unchanged."
  type        = string
  default     = "reference"

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9-]{2,23}$", var.demo_suffix))
    error_message = "demo_suffix must contain 3 through 24 lowercase letters, digits, or hyphens and start with a letter or digit."
  }
}
```

Replace `reference` with a stable suffix that is unique in your FeatBit
Organization. The suffix is desired configuration, not a credential. Do not
change it after the first apply unless you intend to replace the Demo objects.

Put this in `project.tf`:

```hcl
locals {
  project_key = "tf-iam-demo-${var.demo_suffix}"
}

resource "featbit_project" "iam" {
  name = "Terraform IAM Demo ${var.demo_suffix}"
  key  = local.project_key
}

locals {
  project_environment_ids = {
    for environment in featbit_project.iam.environments :
    environment.key => environment.id
  }

  environment_keys = toset(["dev", "prod"])

  environment_ids = {
    dev  = local.project_environment_ids["dev"]
    prod = local.project_environment_ids["prod"]
  }
}
```

FeatBit creates Dev and Prod together with the Project. Terraform reads their
IDs from `featbit_project.iam.environments`; there are no separate Environment
resources in this tutorial.

Put this in `feature-flags.tf`:

```hcl
locals {
  feature_flags = {
    "checkout-enabled" = {
      name        = "Checkout enabled"
      description = "Controls access to the checkout flow"
    }

    "payment-retry" = {
      name        = "Payment retry"
      description = "Controls the payment retry flow"
    }

    "fraud-review" = {
      name        = "Fraud review"
      description = "Controls the manual fraud review flow"
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

<a id="replace-api-access-token"></a>

### Required: replace the API access token in the current shell

Prefer a secret manager that injects `FEATBIT_ACCESS_TOKEN` into the Terraform
process. For this local manual exercise, the commands below deliberately clear
any existing value first. This prevents a stale, revoked, or wrong-Organization
token from being silently reused. The prompt masks the replacement value and
keeps it out of command history.

Paste the raw personal or service access-token value only. Do not add a
`Bearer ` prefix and do not include surrounding quotes.

**PowerShell**

```powershell
Remove-Item Env:FEATBIT_ACCESS_TOKEN -ErrorAction SilentlyContinue
$env:FEATBIT_ACCESS_TOKEN = Read-Host "FeatBit API access token (raw value, no Bearer prefix)" -MaskInput

if ([string]::IsNullOrWhiteSpace($env:FEATBIT_ACCESS_TOKEN)) {
  throw "FEATBIT_ACCESS_TOKEN must not be empty."
}

if ($env:FEATBIT_ACCESS_TOKEN -match '^Bearer\s') {
  throw "Use the raw access-token value without a Bearer prefix."
}

Write-Host "FeatBit API access token loaded (value hidden)."
```

**Bash**

```bash
unset FEATBIT_ACCESS_TOKEN
read -rsp "FeatBit API access token (raw value, no Bearer prefix): " FEATBIT_ACCESS_TOKEN
printf '\n'

if [[ -z "$FEATBIT_ACCESS_TOKEN" ]]; then
  echo "FEATBIT_ACCESS_TOKEN must not be empty." >&2
  exit 1
fi

if [[ "$FEATBIT_ACCESS_TOKEN" == Bearer\ * ]]; then
  echo "Use the raw access-token value without a Bearer prefix." >&2
  exit 1
fi

export FEATBIT_ACCESS_TOKEN
echo "FeatBit API access token loaded (value hidden)."
```

<a id="select-api-origin"></a>

### Required: select the matching API origin

Choose exactly one of the following origins. The access token and API origin
must belong to the same FeatBit deployment.

For **FeatBit Cloud**, clear any old override and let the Provider use its
`https://app-api.featbit.co` default:

**PowerShell**

```powershell
Remove-Item Env:FEATBIT_API_URL -ErrorAction SilentlyContinue
```

**Bash**

```bash
unset FEATBIT_API_URL
```

For a **self-hosted FeatBit deployment**, run this alternative instead, using
that deployment's documented public API origin:

**PowerShell**

```powershell
$env:FEATBIT_API_URL = "https://your-featbit-api.example.com/api/v1"
```

**Bash**

```bash
export FEATBIT_API_URL="https://your-featbit-api.example.com/api/v1"
```

Do not continue until you have completed both required authentication steps.
Terraform deliberately does not write the environment-provided token into the
HCL configuration.

<a id="verify-api-authentication"></a>

### Required: verify authentication before Terraform mutates anything

The following read-only request uses the same API path and direct
`Authorization` header as the Provider's Project create preflight. It does not
print the token or the returned Projects.

**PowerShell**

```powershell
$featbitApiRoot = if ([string]::IsNullOrWhiteSpace($env:FEATBIT_API_URL)) {
  "https://app-api.featbit.co/api/v1"
} else {
  $env:FEATBIT_API_URL.TrimEnd('/')
}

try {
  $featbitAuthResponse = Invoke-WebRequest `
    -Method Get `
    -Uri "$featbitApiRoot/projects" `
    -Headers @{ Authorization = $env:FEATBIT_ACCESS_TOKEN } `
    -SkipHttpErrorCheck
} catch {
  throw "Could not reach the FeatBit API origin. Check FEATBIT_API_URL and network access."
}

switch ([int]$featbitAuthResponse.StatusCode) {
  200 {
    Write-Host "FeatBit API authentication preflight passed (HTTP 200)."
  }
  401 {
    throw "FeatBit returned HTTP 401. Create a fresh personal or service token under Integrations / Access tokens, copy its one-time raw secret, and verify that the token belongs to this API origin."
  }
  403 {
    throw "FeatBit returned HTTP 403. The token is recognized but cannot list Projects; create a token with all permissions required by this IAM tutorial."
  }
  default {
    throw "FeatBit API authentication preflight failed with HTTP $($featbitAuthResponse.StatusCode)."
  }
}
```

**Bash**

```bash
featbit_api_root="${FEATBIT_API_URL:-https://app-api.featbit.co/api/v1}"
featbit_api_root="${featbit_api_root%/}"

if ! featbit_status_code="$(curl --silent --show-error --output /dev/null \
  --write-out '%{http_code}' \
  --header "Authorization: ${FEATBIT_ACCESS_TOKEN}" \
  "${featbit_api_root}/projects")"; then
  echo "Could not reach the FeatBit API origin. Check FEATBIT_API_URL and network access." >&2
  exit 1
fi

case "$featbit_status_code" in
  200)
    echo "FeatBit API authentication preflight passed (HTTP 200)."
    ;;
  401)
    echo "FeatBit returned HTTP 401. Create a fresh personal or service token under Integrations / Access tokens, copy its one-time raw secret, and verify that the token belongs to this API origin." >&2
    exit 1
    ;;
  403)
    echo "FeatBit returned HTTP 403. The token is recognized but cannot list Projects; create a token with all permissions required by this IAM tutorial." >&2
    exit 1
    ;;
  *)
    echo "FeatBit API authentication preflight failed with HTTP ${featbit_status_code}." >&2
    exit 1
    ;;
esac
```

Continue only after this preflight prints HTTP `200`.

This is a per-session preflight, not a one-time setup step. Repeat the token
replacement, API-origin selection, and read-only authentication check whenever
you open a new shell, resume the tutorial after a time gap, or begin another
day of testing. A non-empty `FEATBIT_ACCESS_TOKEN` can still be expired,
revoked, or associated with another deployment.

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

If any later `terraform plan` or `terraform apply` reports HTTP `401`, the API
origin rejected the supplied token before that operation could confirm its
resource graph. Do not add an Organization key and do not reuse the failed or
stale saved plan. Delete `tfplan`, [replace the token](#replace-api-access-token),
[select its matching API origin](#select-api-origin), require the
[read-only preflight](#verify-api-authentication) to return HTTP `200`, and
generate a new saved plan from the same shell.

The first plan should report:

```text
Plan: 7 to add, 0 to change, 0 to destroy.
```

It adds one `featbit_project` and six `featbit_feature_flag` resources. Dev
and Prod do not appear as separate Environment resources because FeatBit
creates them with the Project. The verification plan must report `No changes`.

Confirm in FeatBit that both Environments contain the same three disabled
Feature Flags.

## Step 2: Create two custom Policies

Create two more files in `mypractice/iam/`.

**PowerShell**

```powershell
@(
  "lookups.tf",
  "policies.tf"
) | ForEach-Object {
  New-Item -ItemType File -Force $_ | Out-Null
}
```

**Bash**

```bash
touch lookups.tf policies.tf
```

Put this in `lookups.tf`:

```hcl
data "featbit_project" "iam" {
  key = featbit_project.iam.key
}

data "featbit_environment" "dev" {
  project_id = data.featbit_project.iam.id
  key        = "dev"
}

data "featbit_environment" "prod" {
  project_id = data.featbit_project.iam.id
  key        = "prod"
}
```

These data sources deliberately exercise organization-scoped exact Project
key lookup and Project-scoped exact Environment key lookup. They do not adopt
another lifecycle.

Put this in `policies.tf`:

```hcl
resource "featbit_policy" "project_observer" {
  name        = "Terraform IAM Project Observers ${var.demo_suffix}"
  key         = "tf-iam-project-observer-${var.demo_suffix}"
  description = "Read-only Project and Environment visibility across Dev and Prod"

  statements = [
    {
      resource_type = "project"
      effect        = "allow"
      actions       = ["CanAccessProject"]
      resources     = ["project/${data.featbit_project.iam.key}"]
    },
    {
      resource_type = "env"
      effect        = "allow"
      actions       = ["CanAccessEnv"]
      resources     = ["project/${data.featbit_project.iam.key}:env/*"]
    }
  ]
}

resource "featbit_policy" "dev_operator" {
  name        = "Terraform IAM Dev Operators ${var.demo_suffix}"
  key         = "tf-iam-dev-operator-${var.demo_suffix}"
  description = "Dev access with every non-delete operation"

  statements = [
    {
      resource_type = "project"
      effect        = "allow"
      actions       = ["CanAccessProject"]
      resources     = ["project/${data.featbit_project.iam.key}"]
    },
    {
      resource_type = "env"
      effect        = "allow"
      actions = [
        "CanAccessEnv",
        "UpdateEnvSettings",
        "CreateEnvSecret",
        "UpdateEnvSecret"
      ]
      resources = ["project/${data.featbit_project.iam.key}:env/${data.featbit_environment.dev.key}"]
    },
    {
      resource_type = "flag"
      effect        = "allow"
      actions = [
        "CreateFlag",
        "ArchiveFlag",
        "RestoreFlag",
        "CloneFlag",
        "CopyFlagTo",
        "ToggleFlag",
        "UpdateFlagName",
        "UpdateFlagDescription",
        "UpdateFlagOffVariation",
        "UpdateFlagVariations",
        "UpdateFlagTags",
        "UpdateFlagDefaultRule",
        "UpdateFlagIndividualTargeting",
        "UpdateFlagTargetingRules"
      ]
      resources = ["project/${data.featbit_project.iam.key}:env/${data.featbit_environment.dev.key}:flag/*"]
    },
    {
      resource_type = "segment"
      effect        = "allow"
      actions = [
        "CreateSegment",
        "ArchiveSegment",
        "RestoreSegment",
        "UpdateSegmentName",
        "UpdateSegmentDescription",
        "UpdateSegmentTags",
        "UpdateSegmentTargetingUsers",
        "UpdateSegmentRules"
      ]
      resources = ["project/${data.featbit_project.iam.key}:env/${data.featbit_environment.dev.key}:segment/*"]
    },
  ]
}
```

Feature Flag and Segment permissions do not grant parent visibility by
themselves. The Project observer Policy grants only Project and Environment
access actions. The Dev operator Policy contains its own exact Project and Dev
access statements, so it remains usable when assigned directly to Member B.
Both are `CustomerManaged` Policies.

Format and validate the root, save and apply the reviewed plan, and then run a
second plan to verify idempotence.

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

The first saved plan should report:

```text
Plan: 2 to add, 0 to change, 0 to destroy.
```

The verification plan must report `No changes`. In FeatBit, confirm that both
Policies are `CustomerManaged` and that their statement sets match the HCL.

## Step 3: Create one Group

Create `groups.tf`.

**PowerShell**

```powershell
New-Item -ItemType File -Force groups.tf | Out-Null
```

**Bash**

```bash
touch groups.tf
```

Put this in `groups.tf`:

```hcl
resource "featbit_group" "project_operators" {
  name        = "Terraform IAM Project Operators ${var.demo_suffix}"
  description = "Combines Project visibility with non-delete Dev operations"
}
```

Format and validate the root, save and apply the reviewed plan, and then run a
second plan to verify idempotence.

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

The first saved plan should report:

```text
Plan: 1 to add, 0 to change, 0 to destroy.
```

The verification plan must report `No changes`. The Group should initially
have zero Policy and Member relationships.

<a id="assign-access"></a>

## Step 4: Assign both Policies to the Group

Create `bindings.tf`.

**PowerShell**

```powershell
New-Item -ItemType File -Force bindings.tf | Out-Null
```

**Bash**

```bash
touch bindings.tf
```

Put this in `bindings.tf`:

```hcl
resource "featbit_group_policy_binding" "project_observer" {
  group_id  = featbit_group.project_operators.id
  policy_id = featbit_policy.project_observer.id
}

resource "featbit_group_policy_binding" "dev_operator" {
  group_id  = featbit_group.project_operators.id
  policy_id = featbit_policy.dev_operator.id
}
```

Format and validate the root, save and apply the reviewed plan, and then run a
second plan to verify idempotence.

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

The first saved plan should report:

```text
Plan: 2 to add, 0 to change, 0 to destroy.
```

Each binding owns only its exact pair. The verification plan must report
`No changes`, and the Project operators Group should show exactly the Project
observer and Dev operator Policies.

## Step 5: Assign one Member through the Group and one directly

Before writing HCL, select exactly two dedicated existing test Members in
FeatBit:

1. Member A will use the stable alias `group-member`. This Member joins the
   Project operators Group, receives both Policies only through inheritance,
   and has no direct Policies.
2. Member B will use the stable alias `direct-member`. This Member stays
   outside the Group and has exactly the Dev operator Policy in its complete
   direct set.

For both Members, verify that they are not Owners or administrators and that
their existing effective permissions do not grant access to the newly created
tutorial Project. You do not need to list their current Policies. You do,
however, explicitly authorize Terraform to remove every current direct Policy
from Member A and every current direct Policy other than the Dev operator
Policy from Member B.

Create `members.tf`.

**PowerShell**

```powershell
New-Item -ItemType File -Force members.tf | Out-Null
```

**Bash**

```bash
touch members.tf
```

Put this in `members.tf`:

```hcl
locals {
  member_emails_by_alias = sensitive({
    "group-member"  = "first@example.com"
    "direct-member" = "second@example.com"
  })

  member_aliases = nonsensitive(toset(keys(local.member_emails_by_alias)))
}

data "featbit_member" "tester" {
  for_each = local.member_aliases

  email = local.member_emails_by_alias[each.key]
}

resource "featbit_member_direct_policies" "group_member" {
  member_id = data.featbit_member.tester["group-member"].id
  policy_ids = []

  depends_on = [featbit_group_member_binding.group_member]
}

resource "featbit_member_direct_policies" "direct_member" {
  member_id = data.featbit_member.tester["direct-member"].id
  policy_ids = [
    featbit_policy.dev_operator.id
  ]
}
```

Append the actual Member A-to-Group edge to `bindings.tf`:

```hcl
resource "featbit_group_member_binding" "group_member" {
  group_id  = featbit_group.project_operators.id
  member_id = data.featbit_member.tester["group-member"].id
}
```

The explicit `depends_on` makes Terraform establish or adopt Member A's Group
membership before clearing its direct Policies. If the Group binding fails,
Terraform must not remove those direct Policies.

After applying Step 5, the intended relationship graph is:

| Source | Relationship | Target |
|---|---|---|
| Project operators Group | Group-Policy binding | Project observer Policy |
| Project operators Group | Group-Policy binding | Dev operator Policy |
| Member A (`group-member`) | Group-Member binding | Project operators Group |
| Member A (`group-member`) | Complete direct Policy set | Empty |
| Member B (`direct-member`) | Complete direct Policy set | Dev operator Policy only |

Replace both `example.com` addresses with the two selected Members' complete
FeatBit emails before planning. Keep exactly the `group-member` and
`direct-member` entries: later resources refer to those aliases explicitly.
The aliases are non-identifying because they become Terraform instance keys
and therefore appear in plans and state addresses.

The aliases drive `for_each`, while `sensitive()` prevents the email map from
being displayed through ordinary Terraform expressions. It does not encrypt
the HCL source, so keep this tutorial in the ignored `mypractice/iam/` root and
never commit the real addresses. Member IDs remain Sensitive. Member A's
authoritative resource removes every current direct Policy because the desired
set is empty. Member B's authoritative resource adds the Dev operator Policy
if missing and removes every other direct Policy.

Format and validate the root, save and apply the reviewed plan, and then run a
second plan to verify idempotence.

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

The first saved plan should report:

```text
Plan: 3 to add, 0 to change, 0 to destroy.
```

Confirm with the separate Member sessions that:

1. Member A (`group-member`) belongs to the Project operators Group, has no direct Policies,
   and inherits both Group Policies;
2. Member A (`group-member`) can see Dev and Prod and can operate Dev Flags through inherited
   access, but cannot permanently delete a Dev Flag;
3. Member B (`direct-member`) remains outside the Group, can see the Project and operate Dev
   Flags through its direct Dev operator Policy, but cannot access Prod
   through the Dev operator Policy; and
4. Member B's direct Policy collection contains exactly the Dev operator
   Policy and no Organization-default or unrelated Policy.

Any previous direct Policy on either Member that is not in the configured
complete set is intentionally absent.

## Step 6: Verify inherited-only access versus direct access

Do not add another binding in this step. Step 5 already created the complete
intended topology. In FeatBit, inspect the two Members separately and confirm:

1. Member A (`group-member`) belongs to the Project operators Group;
2. Member A's direct Policy collection is empty;
3. Member A's effective Project observer and Dev operator Policies are both
   inherited from that Group;
4. Member B does not belong to the Project operators Group; and
5. Member B's complete direct Policy collection contains only the Dev operator
   Policy.

Then use the separate Member sessions to verify that Member A and Member B can
both operate Dev Flags and neither can permanently delete them. Member A can
see Prod through the inherited Project observer Policy but cannot toggle a
Prod Flag; Member B cannot access Prod through the direct Dev operator Policy.
The access paths must differ even though the Dev result is the same: Member A
is inherited-only, while Member B is direct-only for the tutorial-managed Dev
operator Policy.

Toggling is safe for this Terraform configuration because enabled state is
UI-owned. Do not rename or delete Terraform-owned Feature Flag definitions
during the access check.

<a id="update-policy"></a>

## Step 7: Add one exact Prod Feature Flag permission

Before continuing, delete any old `tfplan`, then repeat the required
[token replacement](#replace-api-access-token),
[API-origin selection](#select-api-origin), and
[read-only authentication preflight](#verify-api-authentication) if this is a
new shell or a later testing session. Do not proceed until it returns HTTP
`200`.

Append these two statement objects inside
`featbit_policy.dev_operator.statements` in `policies.tf`, immediately before
the list's closing bracket. The existing final Segment statement already ends
with the trailing comma required before these new list elements:

```hcl
    {
      resource_type = "env"
      effect        = "allow"
      actions       = ["CanAccessEnv"]
      resources     = ["project/${data.featbit_project.iam.key}:env/${data.featbit_environment.prod.key}"]
    },
    {
      resource_type = "flag"
      effect        = "allow"
      actions       = ["ToggleFlag"]
      resources     = ["project/${data.featbit_project.iam.key}:env/${data.featbit_environment.prod.key}:flag/checkout-enabled"]
    }
```

Format and validate the root, save and apply the reviewed plan, and then run a
second plan to verify idempotence.

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

The first saved plan should report:

```text
Plan: 0 to add, 1 to change, 0 to destroy.
```

Only `featbit_policy.dev_operator` should update in place. Its key and UUID
must remain unchanged, and the verification plan must report `No changes`.

Using both separate Member sessions, verify:

1. Dev access still behaves as before for both Members;
2. Member A (`group-member`) receives the new Prod Flag operation through the Group, while
   Member B (`direct-member`) receives it through the direct Policy;
3. Prod `checkout-enabled` can be toggled by both Members; and
4. Prod `payment-retry` and `fraud-review` cannot be toggled by either Member.

The final check proves that the new leaf selector is exact rather than a fuzzy
or Environment-wide grant.

<a id="prepare-cleanup"></a>

## Step 8: Remove Member B's direct Policy before cleanup

The Dev operator Policy is currently inherited by Member A through the Project
operators Group and is the sole direct Policy owned for Member B. First update
Member B's complete desired direct set to empty. This separates direct access
cleanup from the Group cleanup in the next step.

Change only `featbit_member_direct_policies.direct_member` in `members.tf`:

```hcl
resource "featbit_member_direct_policies" "direct_member" {
  member_id  = data.featbit_member.tester["direct-member"].id
  policy_ids = []
}
```

Keep the resource so Terraform continues to enforce that Member B has no
direct Policies through the remaining cleanup steps.

Format and validate the root, save and apply the reviewed plan, and then run a
second plan to verify idempotence.

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

The first saved plan should report:

```text
Plan: 0 to add, 1 to change, 0 to destroy.
```

The update removes the Dev operator Policy from Member B while retaining
authoritative ownership of its now-empty complete direct set. Confirm that
Member B no longer receives the tutorial Dev operator access, while Member A
still inherits it through the Project operators Group. The verification plan
must report `No changes`.

<a id="cleanup"></a>

## Step 9: Delete the Group and its exact bindings

Normal GitOps cleanup removes relationships before their endpoints. Delete
these blocks from `bindings.tf`:

- `featbit_group_policy_binding.project_observer`
- `featbit_group_policy_binding.dev_operator`
- `featbit_group_member_binding.group_member`

Delete the block from `groups.tf`:

- `featbit_group.project_operators`

Delete this block from `members.tf` in the same change:

- `featbit_member_direct_policies.group_member`

That resource already owns an empty set, so its Destroy leaves Member A with
no direct Policies while dropping the Terraform ownership record. Its stored
dependency makes Terraform remove it before the Group-Member binding.

Format and validate the root, save and apply the reviewed plan, and then run a
second plan to verify idempotence.

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

The first saved plan should report:

```text
Plan: 0 to add, 0 to change, 5 to destroy.
```

Those five objects are the empty authoritative direct-set record, two exact
Group-Policy pairs, one exact Group-Member pair, and one Group. Terraform
orders the dependency graph before the Group. The two existing Members, both
custom Policies, and all core Project resources remain. The verification plan
must report `No changes`.

Confirm in FeatBit that the tutorial Group is absent, neither Member was
removed or profile-mutated, both Members still have no direct Policies, and
both custom Policies still exist.

## Step 10: Delete the custom Policies

Delete both resource blocks from `policies.tf`:

- `featbit_policy.project_observer`
- `featbit_policy.dev_operator`

Delete the remaining managed resource from `members.tf`:

- `featbit_member_direct_policies.direct_member`

It already owns an empty set after Step 8, so Destroy keeps Member B's direct
Policies empty and only drops the Terraform ownership record.

Then delete both non-owning blocks from `members.tf`:

- `data.featbit_member.tester`
- the `locals` block defining `member_emails_by_alias` and `member_aliases`

Removing the data source and locals has no remote lifecycle effect.

Format and validate the root, save and apply the reviewed plan, and then run a
second plan to verify idempotence.

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

The first saved plan should report:

```text
Plan: 0 to add, 0 to change, 3 to destroy.
```

It removes the empty Member B direct-set ownership record and the two custom
Policies. The verification plan must report `No changes`.

Confirm that both tutorial Policy keys are absent and both Members still have
empty direct Policy collections. Neither selected Member is owned by the
remaining Terraform configuration.

## Final core-resource cleanup

The IAM scenario is now gone. Create and apply one destroy plan for the six
Feature Flags and their Project.

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

The destroy plan should report:

```text
Plan: 0 to add, 0 to change, 7 to destroy.
```

Review it before applying. Terraform removes the six Feature Flags before the
Project, and FeatBit removes the Project's default Dev and Prod Environments
with the Project.

Confirm that the exact tutorial Project, Policy keys, Group name, bindings,
and Feature Flags are absent. Confirm again that both existing Members and
every unrelated relationship remain unchanged.

Clear process-scoped values:

**PowerShell**

```powershell
@(
  "FEATBIT_ACCESS_TOKEN",
  "FEATBIT_API_URL"
) | ForEach-Object {
  Remove-Item "Env:$_" -ErrorAction SilentlyContinue
}
```

**Bash**

```bash
unset FEATBIT_ACCESS_TOKEN FEATBIT_API_URL
```

Local state backups may retain Sensitive Member identifiers from earlier
steps. After remote cleanup is independently confirmed, retain or securely
dispose of the entire practice root according to your organization's state
handling policy. Never commit it.

## Further reading

- [FeatBit Terraform Provider](https://registry.terraform.io/providers/featbit/featbit/latest/docs)
- [Policy resource](../resources/policy.md)
- [Group resource](../resources/group.md)
- [Group-Policy binding resource](../resources/group_policy_binding.md)
- [Group-Member binding resource](../resources/group_member_binding.md)
- [Member-Policy binding resource (additive alternative)](../resources/member_policy_binding.md)
- [Member direct Policies resource](../resources/member_direct_policies.md)
- [Core FeatBit Terraform GitOps Tutorial](GitOpsGettingStarted.md)

[Back to top](#top)
