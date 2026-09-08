---
title: The studio
description: A tour of the studio's layout, from the top bar to the inspector.
---

`togen studio --port 3000` serves the canvas and its JSON API on 127.0.0.1 and opens a browser (`--no-open` if you would rather it did not). The canvas talks to the studio over `/api` and holds a websocket to it, so it reloads the moment a file changes on disk, not only the ones it wrote itself.

## Layout

The studio is four parts:

- a top bar naming the project, its provider, region and environment, with the cost pill, the scenario selector when the project has a simulation, the status pill and the Export and Generate buttons on the right
- a rail on the left with the views list and the palette
- the canvas
- an inspector on the right that opens on selection

## Themes

The studio is dark by default.

![The studio in its default dark theme](/togen/images/studio-dark.png)

`style.theme` in `togen.yml` picks `dark`, `light` or `system` (the OS setting); the toggle in the top bar overrides that for the session.

![The studio in its light theme](/togen/images/studio-light.png)

## The inspector

Click a node to open the inspector on the right and set its name and properties. Edits save shortly after you stop typing.

![The inspector open on a selected node](/togen/images/inspector.png)

The forms it builds come from the project schema; see [Nodes and edges](/togen/guides/nodes-and-edges/) for how that works.

Every edit is checked in the browser with ajv, against the same schema and the same rules the CLI applies, so the top bar's problem count and `togen validate`'s output always agree.

## Traffic

A project with a `togen/simulation.json` gets a scenario selector next to the cost pill, Baseline plus one entry per burst, and the canvas draws each node and edge's rate for whichever scenario is picked. Editing a source, a burst or an edge's fan-out from the simulation panel or the edge inspector writes straight back to that file. See [Simulation](/togen/guides/simulation/) for what a simulation is and how it feeds `togen cost`.

## The palette

The palette lists what the open project's provider offers, in groups (Compute, Storage, Databases, Messaging, Networking) that you can collapse. The search box above it filters on the type's name, its description, the service's own name and its aliases, so typing `lambda`, `object storage` or `redis` all find something; a search opens every group and shows the matches. Drag a tile onto the canvas to add a node.

Most tiles generate infrastructure. A tile marked `draws` is one the provider draws but does not generate yet: the node has a style, properties, relations and cost meters, and `togen generate` names it under `not generated` rather than writing anything for it. `togen generate --strict` fails instead of writing, for a pipeline that wants the whole sketch or nothing.

## The style tab

The inspector's Style tab shows the colour, icon and shape the node is drawn with, and where each comes from: a node override, a kind override, or the provider scheme. It names the service the node becomes on this provider, and says so when the node only draws. Underneath is the `togen.yml` snippet that would override each one, with a copy button.

![The style tab, showing where a node's colour, icon and shape come from](/togen/images/style-tab.png)

## Icons

Nodes and palette tiles carry the provider's own architecture icons (AWS, Google Cloud and Azure), bundled under each vendor's terms, which About in the rail footer lists.
