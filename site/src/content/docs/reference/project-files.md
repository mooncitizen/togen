---
title: Project files
description: What togen/project.json, views.json, layout.json and simulation.json each hold.
---

`togen init` writes a `togen/` directory. Everything in it is JSON, and everything in it is written by either the CLI or the studio, never by hand in the normal case.

## project.json

The sketch itself: `version`, `name`, `provider`, `region`, `environment`, and the `nodes` and `edges` that make up the system. This is what `togen validate`, `togen generate` and `togen cost` all read, and what `PUT /api/project` writes from the studio.

`version` is the project schema version (currently `1`). A file with a higher version than the running Togen understands is refused rather than misread.

See [nodes and edges](/togen/guides/nodes-and-edges/) for the node types and what an edge between two of them means, and `schema/project.schema.json` in the repository for the exact shape.

## views.json

The named views a project is split into: `version` (currently `1`) and a list of views, each with an `id`, a `name`, and either every node (`"*"`) or a list of node ids. The overview is the view with id `overview` and always exists. The studio writes this file when you add, rename or scope a view; see [views](/togen/guides/views/).

## layout.json

Where the studio drew each node, and the pan and zoom it left the canvas at, per view: `version` (currently `2`) and a map of view id to `{ nodes, viewport }`, where `nodes` is a map of node id to an `{ x, y }` position and `viewport` is `{ x, y, zoom }`.

A project can ship with every position empty, `examples/aws-full/togen/layout.json` does:

```json
{ "version": 2, "views": { "overview": { "nodes": {}, "viewport": { "x": 0, "y": 0, "zoom": 1 } } } }
```

The studio lays out an empty view with dagre on first load and saves the result straight back, so the file fills in on its own the first time you open the project.

A version 1 file, one drawing of the whole project rather than one per view, still loads: it becomes the `overview` view under version 2.

## simulation.json

The traffic a sketch carries: sources, bursts and per-edge fan-out. It is optional, a project with no file has no simulation, and the studio writes it as you edit traffic on the canvas. See [simulation](/togen/guides/simulation/) for the shape and what `togen simulate` does with it.
