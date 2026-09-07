---
title: Views
description: Grouping nodes into named views of a project.
---

## What a view is

`togen/views.json` names the views of a project: each has an id, a name, and the node ids it shows, or `"*"` for all of them. Every project has `overview`. A project without the file has the overview alone, and neither `togen validate` nor `togen generate` reads it.

`togen/layout.json` is version 2, with the positions and viewport of each view under `views` by view id. A version 1 file is read as the overview and written back as version 2 on the studio's next save. The API serves the views at `/api/views` and the layout at `/api/layout`.

## Working with views in the studio

The rail lists the views with how many nodes each shows. Click one to open it.

![The views list in the rail, and the canvas showing the open view](/togen/images/views.png)

The plus button adds a view, and the pencil edits one in place of the inspector: rename it, tick the nodes it shows (grouped by type, with the rest dimmed on the canvas while you choose), or delete it. The overview cannot be edited this way; it always shows everything.

![The view editor open on the overview, its node list fixed at every node](/togen/images/view-editor.png)

The canvas names the open view and counts its nodes, draws an edge only when both ends are in the view, and saves positions and the viewport per view, placing a node that has none by auto-layout. Undo and redo cover creating, renaming, deleting and changing the membership of a view.
