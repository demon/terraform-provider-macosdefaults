<!--
Thanks for the PR. Filling this out makes review faster.

If this is a security fix, please coordinate via SECURITY.md instead of opening a public PR.
-->

## What and why

<!-- One or two sentences. What changed, and why does it need to change? Link the issue, and the `defaults` behaviour or macOS version it depends on, if relevant. -->

## Checklist

- [ ] `make lint` is clean
- [ ] `make test` is clean
- [ ] `make generate` produces no `docs/` diff
- [ ] For new resources or schema-affecting changes, the acceptance test passes on macOS (`TF_ACC=1 go test -run TestAcc<Name> ./internal/provider/... -timeout 20m`); tested on macOS version: <!-- e.g. 26.1 -->
- [ ] Acceptance tests only touch `tf-acc.` scratch domains
- [ ] `CHANGELOG.md` updated under the unreleased section
- [ ] No personal preference values, usernames, or paths in committed examples / fixtures

## Notes for reviewers

<!-- Anything reviewers should know but that doesn't belong in the commit message: tradeoffs you considered, things you intentionally skipped, follow-ups you're deferring. -->
