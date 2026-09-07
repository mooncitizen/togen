# Security

## Supported versions

`main` and the most recent tagged release.

## Reporting a vulnerability

Report privately through GitHub's private vulnerability reporting, on the
Security tab of this repository. If that is unavailable to you, email
paul@gitglue.com.

Please do not open a public issue for a vulnerability.

Expect an acknowledgement within three working days and an assessment within
ten. If a fix is warranted it ships in the next release, and the advisory
credits you unless you would rather it did not.

## What the surface actually is

Togen generates code. It never deploys anything, holds no cloud credentials, and
makes no network calls when it generates. The price snapshot it estimates from
is bundled in the binary.

That leaves three real areas:

- `togen studio` runs an HTTP server. It binds 127.0.0.1 only and has no
  authentication, on the assumption that it is a local tool on a trusted
  machine. A way to make it reachable from another host, or a path traversal out
  of the project directory, is a vulnerability.
- The studio reads and writes files under the project directory. Writing outside
  it is a vulnerability.
- The dependency tree, Go and npm.

Generated Terraform is yours to review before you apply it. Togen aims to emit
sensible defaults, but an opinion you disagree with is an issue, not a
vulnerability.
