# Contributing to blink

Thanks for your interest in improving `blink`. This project uses an **issue-first**
workflow. Please read this before opening anything.

## The workflow (issue first, always)

1. **Open an issue** describing what you want to change and why. Use the bug or
   proposal template.
2. **Wait for a maintainer to approve the approach** on that issue. When we agree
   on scope and design we add the `approved` label. This step exists so you never
   spend time on code we can't take.
3. **Only then open a pull request**, linking the issue in the description with
   `Closes #<number>`.

> Pull requests that do not reference a maintainer-approved issue are flagged by a
> bot with a comment and a `needs-approved-issue` label, and a maintainer will
> usually close them. This is not personal and not a judgment of your work. It
> keeps review focused on changes we have already agreed to take. Open an issue
> and we will pick it up.

## Developer Certificate of Origin (DCO)

Every commit must be signed off. By signing off you certify the
[Developer Certificate of Origin 1.1](https://developercertificate.org/): that
you wrote the change, or otherwise have the right to submit it under the
project's license.

Sign off by adding `-s` to your commit:

```sh
git commit -s -m "your message"
```

This appends a `Signed-off-by: Your Name <your@email>` trailer to the commit
message. To add it to commits that are missing it:

```sh
git rebase --signoff main
```

A CI check enforces this on every commit in a pull request, and a PR cannot merge
until all commits are signed off. Sign-off is a text trailer and is independent
of any cryptographic commit signature (`-S`) you may also use.

To make sign-off automatic, drop this hook into `.git/hooks/prepare-commit-msg`
and `chmod +x` it:

```sh
#!/bin/sh
SOB=$(git var GIT_AUTHOR_IDENT | sed -n 's/^\(.*>\).*$/Signed-off-by: \1/p')
grep -qsF "$SOB" "$1" || printf '\n%s\n' "$SOB" >> "$1"
```

## Licensing

Contributions are accepted under the **Apache License 2.0**, the project's
license. Under Apache 2.0 section 5, anything you submit is licensed inbound
under the same terms as outbound, so there is **no separate CLA to sign**. The
DCO sign-off is all we need.

## Building and testing

`blink` is a CLI application.

```sh
go build -o bin/blink ./cmd/blink   # build the binary
./bin/blink --version              # confirm the build
go test ./...                     # run the tests
```

When you report a bug, please include the output of `blink --version`, your OS and
architecture, and the exact command you ran.

## Code of Conduct

By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).
