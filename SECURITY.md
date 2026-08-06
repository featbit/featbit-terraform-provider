# Security policy

## Supported versions

| Version | Security status |
|---|---|
| Latest published release | Supported |
| Earlier releases | Upgrade to the latest release before reporting, unless the upgrade itself is affected. |
| `main` and local development builds | Development only; not a supported distribution. |

If the [releases page](https://github.com/featbit/featbit-terraform-provider/releases)
contains no version, there is not yet a supported public distribution. Security
fixes are published as new, immutable versions; an existing tag or release asset
is never replaced.

## Report a vulnerability

Do not disclose vulnerability details in a public issue. GitHub private
vulnerability reporting is not currently enabled for this repository, and the
project does not advertise another verified private reporting address.

Open a
[detail-free issue](https://github.com/featbit/featbit-terraform-provider/issues/new?title=Private%20security%20contact%20requested)
with the title `Private security contact requested`. The body must contain only
that request. Do not identify the affected component, version, weakness,
impact, reproduction, or possible exploit, and do not attach files. A
maintainer can then provide a private route whose recipient you can verify
before sharing details.

After a private route is established, a useful report contains:

- the affected provider version and platform;
- a concise impact assessment;
- reproduction steps using synthetic values;
- whether exploitation has been observed; and
- a suggested mitigation, if one is known.

Do not send a real access token, authorization header, Terraform state or plan,
debug log, crash file, raw API response, internal hostname, organization or
workspace value, FeatBit object identifier, or user identifier, even through a
private route. Agree on a safe transfer method first if a diagnostic artifact
is essential.

Vulnerabilities in the hosted FeatBit service or a self-hosted FeatBit
deployment are outside this provider repository. Use a verified security route
associated with that service or deployment instead.

## Protect sensitive data

- Supply `FEATBIT_ACCESS_TOKEN` through an approved secret manager or process
  environment. Never place it in Terraform configuration, variable defaults,
  committed environment files, command arguments, or issue text.
- Treat all Terraform state, saved plans, backend snapshots, crash output, and
  debug logs as confidential. Store them outside the repository with access
  controls and encryption appropriate to your environment.
- Reproduce a problem without debug logging first. If local logging is
  necessary, keep it outside the repository, inspect it manually, and remove
  credentials, request headers, URLs, paths, IDs, keys, user data, and response
  bodies before sharing any excerpt.
- Use `example.com`, synthetic keys, and minimal credential-free configuration
  in reports. Describe an error rather than attaching raw state, plan, log, or
  API output.

If a credential or sensitive artifact is exposed, revoke or rotate the
credential first, remove public access to the artifact, and follow the incident
process for the affected FeatBit deployment and Terraform backend. Do not post
the revoked value as evidence.

This policy does not promise a response or remediation SLA. Disclosure timing
must be coordinated through the verified private route.
