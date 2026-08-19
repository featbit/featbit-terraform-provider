# Upgrading

## Version and compatibility policy

The provider follows Semantic Versioning with an explicit pre-1.0 boundary:

- prereleases such as `0.2.0-beta.1` are explicit validation candidates and
  may change before the corresponding stable version based on real-scenario
  findings;
- patch releases in a published minor line, such as `0.1.x`, contain
  backward-compatible bug or security fixes;
- additive resources, data sources, and optional capabilities use a minor
  release; and
- before `1.0.0`, a new minor release may contain a breaking change only when
  release notes call it out and provide an explicit migration. At `1.0.0` and
  later, breaking changes require a new major release.

Published tags and assets are immutable. A correction is released as a new
version.

A compatible release preserves:

- existing valid provider and resource configuration;
- provider attribute and object names, types, defaults, sensitivity, and
  Required/Optional/Computed behavior;
- resource identity, replacement rules, Terraform-owned fields, and canonical
  state meaning;
- refresh of existing state without manual editing; and
- every documented Import form and its exact interpretation.

New state fields must be additive and safe for old state, or be introduced with
an explicit, tested schema-version migration. A change that removes or renames
an attribute or object, changes its type/default/ownership, rejects previously
valid configuration, or reinterprets identity, state, or Import is breaking.

The core Import contracts introduced in `0.1.x` are:

| Resource | Import ID |
|---|---|
| Project | `<project_uuid>` |
| Environment | `<project_uuid>/<environment_uuid>` |
| Feature Flag | `<environment_uuid>/<exact_key>` |
| Segment | `<environment_uuid>/<segment_uuid>` |

The IAM Import contracts introduced in `0.2.x` are:

| Resource | Import ID |
|---|---|
| Custom Policy | `<policy_uuid>` |
| Group | `<group_uuid>` |
| Group-Policy binding | `<group_uuid>/<policy_uuid>` |
| Group-Member binding | `<group_uuid>/<member_uuid>` |
| Member direct Policies | `<member_uuid>` |

An additional Import spelling may be additive, but none of these forms are
removed or reinterpreted in a compatible release.

## Pin the intended release line

Terraform does not select a prerelease through a broad stable constraint. For
the qualified IAM beta, opt in to that exact version:

```hcl
terraform {
  required_providers {
    featbit = {
      source  = "featbit/featbit"
      version = "= 0.2.0-beta.1"
    }
  }
}
```

After real-scenario validation, required fixes, and stable `v0.2.0`
qualification, use `~> 0.2.0` to stay within the IAM-enabled `0.2.x` line.
Core-only roots that intentionally remain on `0.1.x` can retain `~> 0.1.0`.
Commit `.terraform.lock.hcl` for deployed root configurations. Change an exact
prerelease, minor, or major constraint only after reviewing that release's
notes and any migration instructions.

## Safe upgrade workflow

1. Read the exact version's
   [release notes](https://github.com/featbit/featbit-terraform-provider/releases)
   and confirm its Terraform, platform, and FeatBit compatibility claims match
   your environment. A configurable `api_url` is not a self-hosted
   certification.
2. Bring the current provider line to its latest patch and require a clean plan
   before crossing a minor or major boundary.
3. Use your Terraform backend's supported, encrypted snapshot or versioning
   mechanism. Keep state backups outside the repository with restricted
   access; state can contain sensitive infrastructure and FeatBit data.
4. Test the change in a non-production workspace or equivalent isolated
   environment first. Update the version constraint deliberately.
5. Initialize and inspect a fresh plan in a controlled environment:

   ```shell
   terraform init -upgrade
   terraform plan
   ```

   Treat terminal output as sensitive and do not publish or attach it.
6. Stop if the plan proposes an unexplained replacement, destroy, identity
   change, or rewrite of UI-owned Feature Flag behavior. Do not apply merely to
   see whether the diff converges.
7. Apply through your normal reviewed workflow, then run a second plan and
   require no unexpected change.

If an upgrade produces an unexpected diff, keep the prior version constraint
and lock file, do not apply, and open a credential-free issue through
[SUPPORT.md](SUPPORT.md). Provide synthetic HCL and a manually summarized diff,
not state, saved plans, logs, raw responses, or real identifiers.

Do not downgrade blindly after a newer provider has applied changes or written
state; state migrations may be one-way. Review the release-specific migration
and use only your backend's supported restore workflow when a rollback is
required. Never edit state JSON by hand or change an Import ID to force an
upgrade.
