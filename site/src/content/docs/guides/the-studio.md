---
title: The studio
description: A tour of the studio's layout, from the top bar to the inspector.
---

`togen studio --port 3000` serves the canvas and its JSON API on 127.0.0.1 and opens a browser (`--no-open` if you would rather it did not).

## Layout

The studio is four parts:

- a top bar naming the project, its provider, region and environment, with the status pill and the Generate button on the right
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

## The style tab

The inspector's Style tab shows the colour, icon and shape the node is drawn with, and where each comes from: a node override, a kind override, or the provider scheme. Underneath is the `togen.yml` snippet that would override each one, with a copy button.

![The style tab, showing where a node's colour, icon and shape come from](/togen/images/style-tab.png)

## Icons

Nodes and palette tiles carry the provider's own architecture icons (AWS, Google Cloud and Azure), bundled under each vendor's terms, which About in the rail footer lists.
