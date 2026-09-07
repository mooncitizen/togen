---
title: Studio API
description: The HTTP API togen studio serves, for anyone scripting against it.
---

`togen studio` serves the canvas and a JSON API on the port it printed, bound to `127.0.0.1` only. There is no authentication: anything that can reach the port can read and write the project. Do not point `--port` at anything other than loopback.

Every error response is `{ "error": "..." }` or, for a validation failure, `{ "errors": [...] }` in the same shape `togen validate` prints.

## Project

`GET /api/project` returns `togen/project.json`. `404` if the file does not exist yet, `422` if it exists but is not valid JSON.

`PUT /api/project` replaces it. The body is checked the way `togen validate` checks it; a project that does not resolve is refused with `422` and the file is left as it was. `204` on success.

`POST /api/project/init` creates `togen/` and `togen.yml` from a sketch (name, provider, region, environment and nodes) or a bundled example (`{ "example": "aws-full" }`). `409` if `togen/` already exists. `422` if the example name is not one `GET /api/examples` lists, or the sketch does not resolve. `201` with `{ "written": [...] }` on success.

`GET /api/examples` lists the bundled examples as `{ "examples": [...] }`.

`GET /api/workspace` returns `{ "dir": "...", "name": "..." }`, the absolute directory the studio is running in and its base name. It answers the same way whether or not `togen/` exists yet, which is what the first-run screen uses to name the project it is about to create.

## Views, layout and simulation

`GET /api/views` and `PUT /api/views` read and write `togen/views.json`. A `PUT` is checked against the current project; `422` if the project itself does not load, or if a view names a node id that is not in it.

`GET /api/layout` and `PUT /api/layout` read and write `togen/layout.json`. `404` from the `GET` if the file does not exist. A `PUT` accepts a version 1 body and writes it back as version 2.

`GET /api/simulation` and `PUT /api/simulation` read and write `togen/simulation.json`. A `PUT` is checked against the current project the same way views are; a project with no simulation file answers `GET` with an empty simulation rather than `404`.

## Configuration, icons and cost

`GET /api/config` returns `togen.yml` merged with its defaults, plus a `deprecated` field when the project is still on `togen/togen.json`. `422` if the file does not parse, or if a style or usage entry names a node that is not in the current project.

`GET /api/icon?path=...` serves an icon referenced from `togen.yml` by a relative path, `.svg` or `.png` only, resolved under the project root. `400` for anything else, `404` if the file is not there.

`GET /api/cost` returns the same document `togen cost --json` prints. `404` if there is no `togen/project.json` yet.

## Generate and events

`POST /api/generate` runs generation with an optional `{ "target": "hcl" }` body, defaulting to `togen.yml`'s targets and `outDir`. Unlike `togen generate` it has no `--force`: `409` if the output directory has files Togen did not write. `200` with `{ "generated": [...] }` on success.

`GET /api/events` is a WebSocket. The studio uses it to learn when project files change on disk outside the browser.

Any other path under `/api/` answers `404`. Every other path serves the canvas itself.
