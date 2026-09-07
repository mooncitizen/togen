---
title: Your first project
description: What the studio shows the first time you open it in a new directory.
---

The studio starts with or without a project. Point it at an empty directory and it walks you through creating one before it shows you a canvas.

## First run

In a directory with no `togen/`, the studio opens on a first-run screen that names the directory and offers three choices:

- a new sketch: project name, environment, provider with its palette, and region, then an empty canvas
- one of the bundled examples
- importing existing Terraform, which is drawn but comes later

![The first-run screen, offering a new sketch, a bundled example, or importing Terraform](/togen/images/first-run.png)

Underneath, this is `GET /api/project` answering 404 until `POST /api/project/init` creates one, from `{name, provider, region, environment}` as `togen init` would, or from a bundled example (`{example: "aws-basic"}`; `GET /api/examples` lists them, and `GET /api/workspace` names the directory). It refuses with 409 when `togen/` is already there, and leaves an existing `togen.yml` alone. Full route details are in the [Studio API](/togen/reference/api/) reference.

## The canvas

Once a project exists, or once you have picked one of the three choices above, the studio opens the canvas: dark by default.

![The studio in its default dark theme, with an example project open](/togen/images/studio-dark.png)

From here, dragging node types onto the canvas, connecting them, and reading the inspector is the whole workflow. [The studio](/togen/guides/the-studio/) covers the layout, and [Nodes and edges](/togen/guides/nodes-and-edges/) covers adding and wiring nodes.
