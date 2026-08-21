<a id="top"></a>

# FeatBit Terraform IAM GitOps Tutorial

This hands-on tutorial shows how to use the
[FeatBit Terraform Provider](https://registry.terraform.io/providers/featbit/featbit/latest/docs)
to manage one isolated FeatBit IAM scenario as reviewable Terraform code. You
will evolve one Terraform root that creates a Project and Feature Flags,
defines two complementary custom Policies and one Group, exercises inherited
and direct access with exactly two existing Members, updates scoped
permissions, proves that a live Group association blocks Policy deletion, and
then removes everything in the required dependency order.

[Get started](#getting-started) · [Create the baseline](#create-resources) ·
[Assign access](#assign-access) · [Update a Policy](#update-policy) ·
[Prove delete protection](#delete-guard) · [Clean up](#cleanup)

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

One command in Step 8 intentionally uses Terraform targeting to exercise a
negative Provider lifecycle guard. Targeting is not the normal GitOps delete
workflow and must not be copied into routine automation.

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
| Existing Member A (`group-member`) | Step 5 temporarily receives the Project observer Policy directly; Step 6 joins the Group and then removes only that exact tutorial direct binding |
| Existing Member B (`direct-member`) | Stays outside the Group and receives the Dev operator Policy directly |

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

## IAM ownership and safety boundaries

The tutorial depends on these ownership boundaries:

- each `featbit_policy` resource owns one custom Policy and its complete
  statement set;
- each `featbit_group` resource owns only Group existence, name, and
  description;
- each binding resource owns one exact Group-to-Policy, Group-to-Member, or
  direct Member-to-Policy pair; and
- Members remain external: the Provider does not invite, create, update,
  remove, or delete them.

This tutorial deliberately uses `featbit_member_policy_binding`, which owns
only the configured direct Member-Policy pair. It neither asks for nor takes
ownership of a Member's Organization-default or pre-existing direct Policies.
Do not replace it with `featbit_member_direct_policies`: that separate resource
is for callers who intentionally manage one Member's complete direct Policy
set, and the two ownership models must not overlap for the same Member.

Use exactly two dedicated test Members whose unrelated default, direct, or
Group-derived permissions do not grant overlapping access to the tutorial
Project. Do not use a production administrator or your only organization
owner. The tutorial never clears a Member's complete direct Policy set; it
removes only binding resources that the tutorial added.

Member identifiers are Sensitive in Terraform. Treat local state, state
backups, plans, and terminal output as confidential even though the access
token and Member login credential are never placed in HCL.

<a id="getting-started"></a>

## Before you begin

You need:

| Tool or access | Requirement |
|---|---|
| Terraform | `>= 1.5.0, < 2.0.0` |
| Command shell | PowerShell `>= 7` or Bash `>= 3.2` |
| FeatBit API access token | A personal or service token from the target FeatBit Organization, with permission to manage the isolated Project, Feature Flags, custom Policies, Groups, and IAM relationships, and to read both selected Members |
| Existing FeatBit test Members | Exactly two Members with no unrelated effective access to the tutorial Project, safe to receive and remove tutorial-owned bindings; no complete direct Policy inventory is required |
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

This corrected manual exercise requires the exact `0.2.0-beta.2` prerelease,
which adds `featbit_member_policy_binding`. The published `0.2.0-beta.1` does
not contain that resource and cannot perform Steps 5 through 10 safely for
Members whose complete direct Policy baselines are unknown. Do not substitute
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

If `terraform apply` reports HTTP `401`, the API origin rejected the supplied
token. Do not add an Organization key. Return to the two required
authentication sections above, replace the token, select its matching API
origin, and then rerun `terraform apply tfplan` from the same shell. The
Project create preflight runs before mutation, so this specific failure does
not leave a partially created tutorial Project.

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

## Step 5: Give two Members different direct Policies

Before writing HCL, select exactly two dedicated existing test Members in
FeatBit:

1. Member A will use the stable alias `group-member`. This Member starts with
   the Project observer Policy directly and will move to Group-based access in
   Step 6.
2. Member B will use the stable alias `direct-member`. This Member stays
   outside the Group and receives the Dev operator Policy directly.

For both Members, verify that they are not Owners or administrators and that
their existing effective permissions do not grant access to the newly created
tutorial Project. You do not need to list their current Policies. Each binding
resource below reads a Member only to resolve its exact identity and owns only
its one configured pair.

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

The alias `group-member` names Member A's role in the finished scenario; it
does not assign the Group by itself. Step 5 intentionally creates a temporary,
tutorial-owned direct Policy edge for Member A so that Step 6 can demonstrate
a safe direct-to-Group migration. The actual Group membership uses
`featbit_group_member_binding` and is added in Step 6.

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

resource "featbit_member_policy_binding" "group_member_observer" {
  member_id = data.featbit_member.tester["group-member"].id
  policy_id = featbit_policy.project_observer.id
}

resource "featbit_member_policy_binding" "direct_member_dev_operator" {
  member_id = data.featbit_member.tester["direct-member"].id
  policy_id = featbit_policy.dev_operator.id
}
```

After applying Step 5, the expected temporary relationship graph is:

| Source | Relationship | Target |
|---|---|---|
| Project operators Group | Group-Policy binding | Project observer Policy |
| Project operators Group | Group-Policy binding | Dev operator Policy |
| Member A (`group-member`) | Direct Member-Policy binding | Project observer Policy |
| Member B (`direct-member`) | Direct Member-Policy binding | Dev operator Policy |

There is deliberately no Group-Member binding yet. Seeing Member A assigned
directly to the Project observer Policy at this point is expected; it is the
known pair that the tutorial will remove after Group access is established.

Replace both `example.com` addresses with the two selected Members' complete
FeatBit emails before planning. Keep exactly the `group-member` and
`direct-member` entries: later resources refer to those aliases explicitly.
The aliases are non-identifying because they become Terraform instance keys
and therefore appear in plans and state addresses.

The aliases drive `for_each`, while `sensitive()` prevents the email map from
being displayed through ordinary Terraform expressions. It does not encrypt
the HCL source, so keep this tutorial in the ignored `mypractice/iam/` root and
never commit the real addresses. Member IDs remain Sensitive. Every resource
owns exactly one direct Member-Policy pair. Create adopts an
already-present exact pair or adds it once, and it never removes or replaces
another direct Policy. Do not add `featbit_member_direct_policies` for these
Members.

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

The verification plan must report `No changes`. Confirm with the separate
Member sessions that:

1. Member A can see the tutorial Project plus Dev and Prod, but cannot toggle
   the tutorial Flags through the Project observer Policy;
2. Member B can see the Project and operate Dev Flags, but cannot access Prod
   through the Dev operator Policy; and
3. neither Member belongs to the tutorial Group yet.

Any Organization-default or unrelated direct Policies on either Member remain
unchanged.

## Step 6: Move Member A from direct access to Group access

Now create the actual Member A-to-Group relationship. First append this block
to `bindings.tf`:

```hcl
resource "featbit_group_member_binding" "group_member" {
  group_id  = featbit_group.project_operators.id
  member_id = data.featbit_member.tester["group-member"].id
}
```

Apply the Group membership before removing Member A's direct binding. This
ordering avoids an access gap. Run this review and apply loop now, and run it
again after the second edit later in this step.

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

The verification plan must report `No changes`. Confirm that Member A now
belongs to the Project operators Group and inherits both Policies. Member A
temporarily has two access paths to the Project observer Policy, while Member
B remains outside the Group with only its tutorial direct Dev operator
binding.

Now delete only this block from `members.tf`:

```hcl
resource "featbit_member_policy_binding" "group_member_observer" {
  member_id = data.featbit_member.tester["group-member"].id
  policy_id = featbit_policy.project_observer.id
}
```

Run the same PowerShell or Bash review and apply loop above a second time. The
second saved plan should report:

```text
Plan: 0 to add, 0 to change, 1 to destroy.
```

Only the exact direct Member A-to-Project-observer pair should be destroyed.
The verification plan must report `No changes`. Confirm that:

1. Member A still sees Dev and Prod and can operate Dev Flags, now through the
   Group's two Policies;
2. Member A cannot permanently delete a Dev Flag;
3. Member B still operates Dev through its direct Policy and remains outside
   the Group; and
4. neither Member's Organization-default or unrelated direct Policies were
   removed.

The resulting relationship graph is now the intended steady state: Member A
belongs to the Project operators Group and inherits both Group Policies;
Member B remains outside the Group with only the tutorial-owned direct Dev
operator binding. Member A no longer has a tutorial-owned direct Policy.

Toggling is safe for this Terraform configuration because enabled state is
UI-owned. Do not rename or delete Terraform-owned Feature Flag definitions
during the access check.

<a id="update-policy"></a>

## Step 7: Add one exact Prod Feature Flag permission

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
2. Member A receives the new Prod Flag operation through the Group, while
   Member B receives it through the direct Policy;
3. Prod `checkout-enabled` can be toggled by both Members; and
4. Prod `payment-retry` and `fraud-review` cannot be toggled by either Member.

The final check proves that the new leaf selector is exact rather than a fuzzy
or Environment-wide grant.

<a id="delete-guard"></a>

## Step 8: Prove that a Group association blocks Policy deletion

The Dev operator Policy is currently inherited by Member A through the Project
operators Group and assigned directly to Member B. First remove only Member
B's tutorial-owned direct pair so the negative deletion test has exactly one
remaining association path.

Delete the `featbit_member_policy_binding.direct_member_dev_operator` block
from `members.tf`. Keep the `locals` block and
`data.featbit_member.tester`; Step 6 still uses Member A for the Group-Member
binding.

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
Plan: 0 to add, 0 to change, 1 to destroy.
```

The destroy removes only Member B's exact tutorial pair. No baseline Policy
inventory or comparison is required. Confirm that Member B no longer receives
the tutorial Dev operator access, while Member A still inherits it through the
Project operators Group. The verification plan must report `No changes`.

Now create a targeted destroy plan for only the Dev operator Policy. This is a
controlled negative test, not a normal GitOps operation. Inspect the plan and
continue only if it shows exactly one Policy destroy and no binding or Group
destroy.

The targeted plan should report:

```text
Plan: 0 to add, 0 to change, 1 to destroy.
```

**PowerShell**

```powershell
terraform plan -destroy -target=featbit_policy.dev_operator -out=blocked.tfplan
if ($LASTEXITCODE -ne 0) { throw "the targeted destroy plan failed." }

terraform show -no-color blocked.tfplan

terraform apply blocked.tfplan
if ($LASTEXITCODE -eq 0) {
  throw "expected Policy deletion to be refused while the Group binding exists."
}

Remove-Item blocked.tfplan -ErrorAction SilentlyContinue
```

**Bash**

```bash
if ! terraform plan -destroy -target=featbit_policy.dev_operator -out=blocked.tfplan; then
  echo "the targeted destroy plan failed." >&2
  exit 1
fi

terraform show -no-color blocked.tfplan

if terraform apply blocked.tfplan; then
  echo "expected Policy deletion to be refused while the Group binding exists." >&2
  exit 1
fi

rm -f blocked.tfplan
```

The apply must fail with this Provider diagnostic title:

```text
FeatBit Policy Still Has Live Associations
```

The detail explains that Destroy refuses to cascade a Policy still assigned
to a Group or direct Member. No delete mutation is sent, the Policy remains in
FeatBit, and Terraform preserves its state.

Run a normal full plan after the expected failure:

```console
terraform plan
```

It must report `No changes`. If the targeted plan included a binding or Group
destroy, or if the apply unexpectedly succeeded, stop and investigate before
continuing.

<a id="cleanup"></a>

## Step 9: Delete the Group and its exact bindings

Normal GitOps cleanup removes relationships before their endpoints. Delete
these blocks from `bindings.tf`:

- `featbit_group_policy_binding.project_observer`
- `featbit_group_policy_binding.dev_operator`
- `featbit_group_member_binding.group_member`

Delete the block from `groups.tf`:

- `featbit_group.project_operators`

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
Plan: 0 to add, 0 to change, 4 to destroy.
```

Those four objects are two exact Group-Policy pairs, one exact Group-Member
pair, and one Group. Terraform orders the bindings before the Group. The two
existing Members, both custom Policies, and all core Project resources remain.
The verification plan must report `No changes`.

Confirm in FeatBit that the tutorial Group is absent, the Members are not
removed or changed, and both custom Policies still exist.

## Step 10: Delete the custom Policies

Delete both resource blocks from `policies.tf`:

- `featbit_policy.project_observer`
- `featbit_policy.dev_operator`

Delete both remaining blocks from `members.tf`:

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
Plan: 0 to add, 0 to change, 2 to destroy.
```

It removes only the two custom Policies. The verification plan must report
`No changes`.

Confirm that both tutorial Policy keys are absent. Neither selected Member nor
any Organization-default or unrelated direct Policy is owned by the remaining
Terraform configuration.

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
- [Policy resource](docs/resources/policy.md)
- [Group resource](docs/resources/group.md)
- [Group-Policy binding resource](docs/resources/group_policy_binding.md)
- [Group-Member binding resource](docs/resources/group_member_binding.md)
- [Member-Policy binding resource](docs/resources/member_policy_binding.md)
- [Member direct Policies resource](docs/resources/member_direct_policies.md)
- [Core FeatBit Terraform GitOps Tutorial](GitOpsGettingStarted.md)

[Back to top](#top)
