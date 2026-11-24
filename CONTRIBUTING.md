# Contributing

PRs are welcome.

## Setup

You need Go (version in [`go.mod`](go.mod)), [`golangci-lint`](https://golangci-lint.run/), and
`terraform` on your `PATH`. Acceptance tests also need a Mac.

## Testing

```sh
make test       # unit tests, ~20s, any OS
make lint
make generate   # regenerate docs/
```

`make test` runs real Terraform plan/apply cycles against a fake `defaults`, so it never
touches your preferences. Most tests belong here.

Acceptance tests run the real `defaults` binary as you, so they need macOS:

```sh
make testacc
TF_ACC=1 go test -run TestAccPreference ./internal/provider/...   # just one
```

They only write to `tf-acc.<random>` domains and clean up after themselves. To double-check:

```sh
defaults domains | tr ',' '\n' | grep tf-acc
```

Two groups are opt-in:

- `MACOSDEFAULTS_TEST_CURRENT_HOST=1` runs the `current_host` tests (they write your ByHost
  preferences).
- `MACOSDEFAULTS_TEST_SYSTEM=1` runs the `system` tests. These need root, and the user-scope
  tests refuse to run as root, so run them separately:

  ```sh
  sudo -E env MACOSDEFAULTS_TEST_SYSTEM=1 TF_ACC=1 go test -run TestAccSystem ./internal/provider/...
  ```

To test against OpenTofu instead of Terraform:

```sh
TF_ACC_TERRAFORM_PATH="$(command -v tofu)" TF_ACC_PROVIDER_NAMESPACE=hashicorp \
  TF_ACC_PROVIDER_HOST=registry.opentofu.org make test
```

CI runs all of this on Linux and macOS, against both CLIs; see
[`.github/workflows/test.yml`](.github/workflows/test.yml).

## Pull requests

Run `make lint`, `make test`, and `make generate` (CI fails on any `docs/` diff), plus the
acceptance test for whatever you touched. Add a line to `CHANGELOG.md` under Unreleased. Keep
commit messages short and imperative.

Things reviewers care about:

- Running `terraform apply` twice gives no diff the second time.
- Changes made in System Settings show up as drift, not silently overwritten.
- Nothing touches a key the config didn't declare.
- New features respect `scope` and the root rule.

## Adding a resource

Copy the shape of `macosdefaults_preference`. New `defaults` operations go in `internal/defaults`
(with a matching fake), the resource goes in `internal/provider`, and it needs an example under
`examples/` and an acceptance test. [DEVELOPMENT.md](DEVELOPMENT.md) explains why things are laid
out the way they are.

## Releasing

Move the Unreleased section of `CHANGELOG.md` under a `## vX.Y.Z` heading, tag `main`, and push
the tag. GoReleaser builds and publishes the darwin binaries.
