---
title: Nodes and edges
description: Adding nodes to the canvas, connecting them, and what a refused pairing looks like.
---

## Adding a node

Drag a type from the palette onto the canvas to add a node, and drag nodes around to arrange them. Both save to `togen/`, and a project with no saved positions is laid out left to right on first load.

## Editing a node

Click a node to open the inspector on the right and set its name and properties. Edits save shortly after you stop typing.

The inspector's form for each node type comes from the project schema, not from anything hand-written: it lists whatever properties that type has, as toggles, selects, numbers, text, or a key-value editor for `env`, with the schema description as help text and the default as the placeholder. Only what you set is written, so a field left empty stays out of `togen/project.json`. Database versions come from the provider's supported engine list, and node and palette icons come from the provider's own style set, listed in the [node catalogue](/togen/reference/catalogue/).

Every change is validated in the browser against the same schema and the same rules as the CLI: the top bar counts the problems and expands to the lines the CLI would print, and the node each one names gets a red badge, or the edge a red stroke.

## Connecting nodes

Drag from the right of one node to the left of another to connect them. What happens next depends on what the two node types support:

- one legal relation is applied straight away
- several offer a small menu to choose from
- a pair the relation table has nothing for is refused, with the reason

Which relations exist between which node types, and what each one does per provider, is in the [node catalogue](/togen/reference/catalogue/) rather than here.

## Editing and deleting edges

Click an edge to set a route's path and methods, and, on a project with a simulation, its fan-out: how many calls it makes per call in. Delete the selected node or edge from the inspector, or with the delete key while the canvas has focus.

Edges are drawn with right-angled turns rather than curves, so a dense project stays readable. See [Simulation](/togen/guides/simulation/) for what fan-out changes.

## Boundaries

The canvas also draws the network the resolver will create, the VPC, VNet, or VPC network, and on Azure the resource group around everything, as a dashed boundary round the nodes that need it. It is labelled implicit because it is worked out from the view and never stored.
