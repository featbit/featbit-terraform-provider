# Contributing

Contributions should preserve the provider's documented public API, exact
identity behavior, state safety, and credential boundaries. Open a
[provider issue](https://github.com/featbit/featbit-terraform-provider/issues)
before a substantive change so its scope and compatibility can be agreed
without exposing private deployment details.

## Prerequisites

- Go 1.25.8, matching [`go.mod`](go.mod);
- Git;
- GNU Make for the documented convenience targets; and
- network access for the initial module and pinned documentation-tool download.

Do not configure a FeatBit credential for ordinary development. Unit, mock,
Protocol, documentation, and example checks are credential-free.

## Local workflow

Clone your fork, enter the repository, and download the pinned Go modules:

```shell
go mod download
```

Run the release-equivalent local checks before submitting a change:

```shell
make fmt
make lint
make test
go test -race ./...
make build
go mod tidy -diff
go mod verify
make docs-check
```

`make fmt` rewrites Go files. The remaining commands should not change tracked
files. Keep tests focused and deterministic; use table-driven coverage when the
same setup and assertions can exercise multiple parameter values.

## Documentation

Provider and object reference pages are generated from the production Protocol
schema plus reviewed templates and examples. Change schema descriptions,
`templates/`, or `examples/` as appropriate, then regenerate explicitly:

```shell
make docs
```

Review every generated change and run the non-writing drift check:

```shell
make docs-check
```

Never put a token, real API URL, tenant value, runtime ID or key, state, saved
plan, log, or copied API response in an example, fixture, test failure, or
documentation page.

## Optional live acceptance

Live acceptance is not a contribution or pull request requirement. Most
contributors should run only the credential-free checks above. The live suite
is an optional, destructive release gate for maintainers or contributors who
already have explicit authorization to use a dedicated non-production test
boundary. Do not seek or create remote-service credentials merely to
contribute.

For an authorized run, every remote object involved must be owned by that test
run. Do not run it against production or use a shared Segment fixture that the
run does not exclusively own.

The complete suite requires `TF_ACC=1`, `FEATBIT_ACCESS_TOKEN`, and, for Segment
coverage, `FEATBIT_TEST_ORGANIZATION_KEY`. Supply both FeatBit values out of
band through the protected acceptance environment provided by the maintainer.
Invoke:

```shell
make testacc
```

The harness creates cryptographically unique `tfacc-*` keys, registers exact
created identities in memory, removes children before parents, and verifies
exact absence during cleanup. Do not replace those safeguards with broad list,
fuzzy-match, or bulk-delete behavior. Avoid terminating the process before
cleanup completes. If forced termination leaves an object, stop and use only
the dedicated test boundary's normal administration path to identify objects
from that exact run, remove them child-first, and verify each exact key; never
scan or delete unrelated objects.

Never persist or publish the token, organization key, generated prefix,
runtime IDs, cleanup inventory, state, plans, logs, request paths, or response
bodies.

## Compatibility checklist

Before changing provider configuration, a resource or data-source schema,
identity, lifecycle ownership, canonical state, or Import parsing:

1. Read [UPGRADING.md](UPGRADING.md) and the frozen assertions in
   [`release_contract_test.go`](release_contract_test.go).
2. Search existing client, model, conversion, validation, lifecycle, Import,
   redaction, and test helpers before adding another implementation.
3. Classify the change as compatible or breaking. A breaking change cannot be
   hidden in a patch release.
4. Preserve old state directly or add and test an explicit schema-version
   migration before new state is written. Never require manual state editing.
5. Preserve all documented Import forms and exact-match behavior.
6. Add focused unit/mock coverage and Protocol lifecycle coverage, then update
   generated documentation when the public contract changes.

New endpoint code must use documented public FeatBit APIs, exact identifiers,
redaction-safe diagnostics, context cancellation, and the existing one-shot
mutation and reconciliation boundaries. Do not add Portal-private APIs or
direct database access.

## Pull request scope

Keep each pull request focused, explain user-visible and compatibility effects,
and include the checks you ran. Do not include unrelated formatting, generated
artifacts outside the documented workflow, credentials, state, plans, logs,
runtime values, or release/publication actions.
