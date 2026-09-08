---
title: Upgrading
description: How togen tells you about a new release, and how to move to it.
---

## The daily check

Togen asks GitHub for the latest release at most once a day and caches the answer under your user cache directory. When there is a newer one it prints two lines to standard error after the command's own output, naming the right upgrade command for how you installed it.

Nothing about your project is sent, and no account is involved. The check does not run at all when:

- the binary was built locally and reports `dev` rather than a version;
- `TOGEN_NO_UPDATE_CHECK` is set to anything;
- `CI` is set, so pipelines never see it;
- standard error is not a terminal, so pipes, redirects and scripts never see it;
- you passed `--json`;
- you ran `togen version`, `togen upgrade`, `togen help` or `togen completion`.

## togen upgrade

```bash
togen upgrade
```

What it does depends on how togen was installed, which `togen version` reports.

**Managed (install script or direct download), which `togen version` reports as `managed`.** It downloads the archive for your platform, verifies it against `checksums.txt`, and replaces the running binary with the one inside. If anything fails before the final step, the binary you have is untouched.

**Homebrew.** It refuses, and prints `brew upgrade mooncitizen/tap/togen`. Overwriting a Cellar binary would leave it out of step with the formula.

**Nix.** It refuses. The store is read only; update the flake input instead.

**A path togen cannot write to.** This is what a manually downloaded binary sitting in a root-owned directory like `/usr/local/bin` looks like. Togen prints that it cannot write to that path and cannot replace itself, and tells you to reinstall it the way you installed it.

`--check` reports the installed and latest versions and does nothing else. `--version v0.1.0` installs a specific tag, so a bad release can be stepped back from. `--yes` skips the confirmation, and without a terminal togen refuses unless you pass it.

## togen version

```bash
togen version
```

Prints the version, the commit and date it was built from, the Go version and platform, and how it was installed with the path it resolved. `--json` prints the same as an object.
