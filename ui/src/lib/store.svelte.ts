import { getContext, setContext } from 'svelte';
import type { Edge as FlowEdge, Node as FlowNode } from '@xyflow/svelte';

import {
  ApiError,
  generate as generateFiles,
  getConfig,
  getCost,
  getLayout,
  getProject,
  getViews,
  putLayout,
  putProject,
  putViews,
} from './api.ts';
import { defaultProperties } from './catalogue.ts';
import { subtotalOf } from './cost.ts';
import { autoLayout } from './layout.ts';
import { resolveStyle, type Resolved } from './style.ts';
import type {
  Config,
  Cost,
  Edge,
  Generated,
  Layout,
  Node,
  NodeType,
  Position,
  Project,
  Relation,
  ValidationError,
  View,
  ViewLayout,
  ViewNodes,
  Viewport,
  Views,
} from './types.ts';
import { errorLine, validateProject } from './validate.ts';

type Patch = { name?: string; properties?: Record<string, unknown> };

type Held<T> = { index: number; item: T }[];

type Taken = {
  nodes: Held<Node>;
  edges: Held<Edge>;
  positions: Record<string, Position>;
  memberships: string[];
};

type Selection = { node: string | null; edge: string | null };

// The project and the views as they were before a change, with the positions
// and view layouts that change took away so putting things back puts them
// where they were.
type Entry = {
  project: Project;
  views: View[];
  positions: Record<string, Position>;
  layouts: Record<string, ViewLayout>;
};

export const overviewId = 'overview';

const saveDelay = 300;
const noticeDelay = 6000;
const historyLimit = 100;
const origin: Position = { x: 0, y: 0 };
const overview: View = { id: overviewId, name: 'Overview', nodes: '*' };

// Positions and the viewport are separate state so that panning the canvas
// does not touch the nodes: Svelte Flow re-measures and briefly hides any node
// whose object it has not seen before, and a node being measured cannot be
// grabbed.
export class Store {
  project = $state.raw<Project | null>(null);
  config = $state.raw<Config | null>(null);
  configError = $state.raw<string | null>(null);
  views = $state.raw<View[]>([overview]);
  activeView = $state.raw<string>(overviewId);
  editing = $state(false);
  cost = $state.raw<Cost | null>(null);
  costErrors = $state.raw<ValidationError[]>([]);
  costOpen = $state(false);
  exporting = $state(false);
  positions = $state.raw<Record<string, Position>>({});
  viewport = $state.raw<Viewport>({ x: 0, y: 0, zoom: 1 });
  selectedNodeId = $state.raw<string | null>(null);
  selectedEdgeId = $state.raw<string | null>(null);
  error = $state.raw<ApiError | null>(null);
  notice = $state.raw<string | null>(null);
  ready = $state(false);
  generated = $state.raw<Generated[] | null>(null);
  generating = $state(false);
  saving = $state(false);
  undoable = $state.raw<Entry[]>([]);
  redoable = $state.raw<Entry[]>([]);

  // The same checks as the CLI, run on every change, separate from what the
  // studio said about the last save.
  readonly errors: ValidationError[] = $derived(
    this.project === null ? [] : validateProject(this.project),
  );

  // What the last generate refused, kept apart from the checks the canvas runs
  // itself but shown in the same places.
  apiErrors = $state.raw<ValidationError[]>([]);

  readonly problems: ValidationError[] = $derived([...this.errors, ...this.apiErrors]);

  // What togen.yml has to say for itself: why it could not be read, or that
  // the legacy file is still in use.
  readonly note: string | null = $derived(this.configError ?? this.config?.deprecated ?? null);

  readonly canUndo: boolean = $derived(this.undoable.length > 0);
  readonly canRedo: boolean = $derived(this.redoable.length > 0);

  readonly view: View = $derived(
    this.views.find((view) => view.id === this.activeView) ?? overview,
  );

  readonly visibleNodeIds: Set<string> = $derived(this.members(this.view));

  readonly flowNodes: FlowNode[] = $derived.by(() => {
    const project = this.project;
    if (project === null) {
      return [];
    }
    const counted = tally(this.problems, 'nodeId');
    const visible = this.visibleNodeIds;
    const styled = (node: Node) => resolveStyle(node, this.config, project.provider);
    return project.nodes.flatMap((node) => {
      const errors = counted[node.id] ?? 0;
      if (visible.has(node.id)) {
        return [
          this.#card(
            node,
            this.positions[node.id] ?? origin,
            errors,
            styled(node),
            subtotalOf(this.cost, node),
            false,
          ),
        ];
      }
      if (!this.editing) {
        return [];
      }
      return [
        this.#card(
          node,
          this.#shownAt(node.id),
          errors,
          styled(node),
          subtotalOf(this.cost, node),
          true,
        ),
      ];
    });
  });

  readonly flowEdges: FlowEdge[] = $derived.by(() => {
    const counted = tally(this.problems, 'edgeId');
    const visible = this.visibleNodeIds;
    return (this.project?.edges ?? [])
      .filter((edge) => visible.has(edge.from) && visible.has(edge.to))
      .map((edge) => ({
        id: edge.id,
        source: edge.from,
        target: edge.to,
        label: edge.relation,
        selected: edge.id === this.selectedEdgeId,
        class: edge.id in counted ? 'togen-edge-error' : undefined,
      }));
  });

  // The studio draws one view at a time; the entries of the others ride along
  // untouched so a save never drops them.
  get layout(): Layout {
    return {
      version: 2,
      views: {
        ...this.#layouts,
        [this.activeView]: { nodes: this.positions, viewport: this.viewport },
      },
    };
  }

  #layouts: Record<string, ViewLayout> = {};
  #cards = new Map<string, { key: string; card: FlowNode }>();
  #timer: ReturnType<typeof setTimeout> | undefined;
  #nodeTimer: ReturnType<typeof setTimeout> | undefined;
  #viewTimer: ReturnType<typeof setTimeout> | undefined;
  #costTimer: ReturnType<typeof setTimeout> | undefined;
  #costSeq = 0;
  #noticeTimer: ReturnType<typeof setTimeout> | undefined;
  #chain: Promise<void> = Promise.resolve();
  #running = 0;
  #stale = false;
  #saved: Project | null = null;
  #savedViews: View[] = [overview];
  #edited = new Set<string>();
  #editedEdges = new Set<string>();
  #coalescing: string | null = null;

  async load(): Promise<void> {
    await this.loadConfig();
    void this.loadCost();
    let project: Project;
    try {
      project = await getProject();
    } catch (failure) {
      this.#report(failure);
      this.ready = true;
      return;
    }
    this.project = project;
    this.#saved = project;
    this.#forget();
    const sound = await this.#readViews();
    const found = await this.#readLayout();
    this.ready = true;
    if (!found) {
      return;
    }
    if (sound) {
      this.error = null;
    }
    await this.#placeMissing();
  }

  // togen.yml is read, never written, and a broken one is not a broken
  // project: the studio runs on the defaults and says why until it is fixed.
  async loadConfig(): Promise<void> {
    try {
      this.config = await getConfig();
      this.configError = null;
    } catch (failure) {
      this.config = null;
      this.configError = describe(failure);
    }
  }

  // A 422 is the same list the checks produce, kept apart from them because it
  // is what the studio said, and the last good estimate goes with it.
  async loadCost(): Promise<void> {
    clearTimeout(this.#costTimer);
    this.#costTimer = undefined;
    const seq = ++this.#costSeq;
    let cost: Cost;
    try {
      cost = await getCost();
    } catch (failure) {
      if (seq === this.#costSeq) {
        this.cost = null;
        this.costErrors =
          failure instanceof ApiError && failure.errors.length > 0
            ? failure.errors
            : [{ path: '', message: describe(failure) }];
      }
      return;
    }
    // A reload and a save's refetch can overlap; only the later answer lands.
    if (seq === this.#costSeq) {
      this.cost = cost;
      this.costErrors = [];
    }
  }

  toggleCost(open: boolean = !this.costOpen): void {
    this.costOpen = open;
  }

  // A file event during a save would fetch back the version the save has not
  // reached yet, so it waits for the queue to drain and then reloads once.
  reload(): void {
    if (this.#saving()) {
      this.#stale = true;
      return;
    }
    void this.load();
  }

  // A node dropped while a view is open joins that view, or it would vanish.
  addNode(type: NodeType, position: Position): Promise<void> {
    const project = this.project;
    if (project === null) {
      return Promise.resolve();
    }
    this.#record();
    const name = `${type}-${nextIndex(project, type)}`;
    const node: Node = { id: name, type, name, properties: defaultProperties(type) };
    const next: Project = { ...project, nodes: [...project.nodes, node] };
    this.project = next;
    this.positions = { ...this.positions, [name]: round(position) };
    const views = this.#joined(this.views, this.activeView, name);
    const joined = views !== this.views;
    this.views = views;
    const layout = this.layout;

    return this.#queue(async () => {
      try {
        await putProject(next);
      } catch (failure) {
        this.#discard([name], []);
        throw failure;
      }
      this.#commit(next);
      if (joined) {
        await this.#putViews(views);
      }
      await putLayout(layout);
    });
  }

  addEdge(from: string, to: string, relation: Relation): Promise<boolean> {
    const project = this.project;
    if (project === null) {
      return Promise.resolve(false);
    }
    const twin = project.edges.find(
      (edge) => edge.from === from && edge.to === to && edge.relation === relation,
    );
    if (twin !== undefined) {
      this.notify(duplicate(project, from, to, relation));
      return Promise.resolve(false);
    }
    this.#record();
    const id = `edge-${nextEdgeIndex(project)}`;
    const next: Project = { ...project, edges: [...project.edges, { id, from, to, relation }] };
    this.project = next;

    let saved = true;
    return this.#queue(async () => {
      try {
        await putProject(next);
      } catch (failure) {
        this.#discard([], [id]);
        saved = false;
        throw failure;
      }
      this.#commit(next);
    }).then(() => saved);
  }

  // A view naming a node the project no longer has is refused by the studio,
  // so the node leaves every view along with the project.
  deleteNode(id: string): Promise<void> {
    const project = this.project;
    if (project === null || !project.nodes.some((node) => node.id === id)) {
      return Promise.resolve();
    }
    const taken: Taken = {
      nodes: held(project.nodes, (node) => node.id === id),
      edges: held(project.edges, (edge) => edge.from === id || edge.to === id),
      positions: id in this.positions ? { [id]: this.positions[id] } : {},
      memberships: this.views
        .filter((view) => view.nodes !== '*' && view.nodes.includes(id))
        .map((view) => view.id),
    };
    this.#record(null, taken.positions);
    const next: Project = {
      ...project,
      nodes: project.nodes.filter((node) => node.id !== id),
      edges: project.edges.filter((edge) => edge.from !== id && edge.to !== id),
    };
    this.project = next;
    const positions = { ...this.positions };
    delete positions[id];
    this.positions = positions;
    const views = this.#left(this.views, id);
    const left = views !== this.views;
    this.views = views;
    const selection = this.#takeSelection();
    const layout = this.layout;

    return this.#queue(async () => {
      try {
        await putProject(next);
      } catch (failure) {
        this.#putBack(taken);
        this.#putBackSelection(selection);
        throw failure;
      }
      this.#commit(next);
      if (left) {
        await this.#putViews(views);
      }
      await putLayout(layout);
    });
  }

  deleteEdge(id: string): Promise<void> {
    const project = this.project;
    if (project === null || !project.edges.some((edge) => edge.id === id)) {
      return Promise.resolve();
    }
    const taken: Taken = {
      nodes: [],
      edges: held(project.edges, (edge) => edge.id === id),
      positions: {},
      memberships: [],
    };
    this.#record();
    const next: Project = { ...project, edges: project.edges.filter((edge) => edge.id !== id) };
    this.project = next;
    const selection = this.#takeSelection();

    return this.#queue(async () => {
      try {
        await putProject(next);
      } catch (failure) {
        this.#putBack(taken);
        this.#putBackSelection(selection);
        throw failure;
      }
      this.#commit(next);
    });
  }

  // While the view editor is open the canvas only previews membership, so the
  // inspector is not brought back by a click on a card.
  select(id: string | null): void {
    if (this.editing && id !== null) {
      return;
    }
    this.selectedNodeId = id;
    if (id !== null) {
      this.selectedEdgeId = null;
    }
  }

  selectEdge(id: string | null): void {
    if (this.editing && id !== null) {
      return;
    }
    this.selectedEdgeId = id;
    if (id !== null) {
      this.selectedNodeId = null;
    }
  }

  clearSelection(): void {
    this.selectedNodeId = null;
    this.selectedEdgeId = null;
  }

  notify(message: string): void {
    this.notice = message;
    clearTimeout(this.#noticeTimer);
    this.#noticeTimer = setTimeout(() => {
      this.notice = null;
    }, noticeDelay);
  }

  // A property the user has cleared is removed rather than written as its
  // default: a project file records only what was set (ADR 0004).
  updateNode(id: string, patch: Patch): void {
    const project = this.project;
    if (project === null) {
      return;
    }
    this.#record(`node:${id}`);
    this.project = {
      ...project,
      nodes: project.nodes.map((node) => (node.id === id ? patchedNode(node, patch) : node)),
    };
    this.#edited.add(id);
    this.#saveProjectSoon();
  }

  updateEdge(id: string, properties: Record<string, unknown>): void {
    const project = this.project;
    if (project === null) {
      return;
    }
    this.#record(`edge:${id}`);
    this.project = {
      ...project,
      edges: project.edges.map((edge) => (edge.id === id ? patchedEdge(edge, properties) : edge)),
    };
    this.#editedEdges.add(id);
    this.#saveProjectSoon();
  }

  members(view: View): Set<string> {
    const nodes = this.project?.nodes ?? [];
    if (view.nodes === '*') {
      return new Set(nodes.map((node) => node.id));
    }
    const listed = new Set(view.nodes);
    return new Set(nodes.filter((node) => listed.has(node.id)).map((node) => node.id));
  }

  openView(id: string): void {
    if (id === this.activeView || !this.views.some((view) => view.id === id)) {
      return;
    }
    this.#flushLayout();
    this.#layouts = this.#stashed();
    this.activeView = id;
    const entry = this.#layouts[id];
    this.positions = entry?.nodes ?? {};
    this.viewport = entry?.viewport ?? { x: 0, y: 0, zoom: 1 };
    this.clearSelection();
    void this.#placeMissing();
  }

  openEditor(id: string): void {
    this.openView(id);
    if (this.activeView !== id) {
      return;
    }
    this.clearSelection();
    this.editing = true;
  }

  closeEditor(): void {
    this.editing = false;
  }

  // A new view starts with the selected node, if there is one, and opens in
  // the editor so the rest can be ticked.
  createView(name: string): string | null {
    const title = name.trim();
    if (title === '' || this.project === null) {
      return null;
    }
    this.#record();
    const id = freeId(kebab(title), this.views);
    const selected = this.selectedNodeId;
    const chosen = this.project.nodes.some((node) => node.id === selected);
    const nodes = selected !== null && chosen ? [selected] : [];
    this.views = [...this.views, { id, name: title, nodes }];
    this.#saveViewsSoon();
    this.openEditor(id);
    return id;
  }

  renameView(id: string, name: string): void {
    const title = name.trim();
    const view = this.views.find((candidate) => candidate.id === id);
    if (view === undefined || title === '' || title === view.name) {
      return;
    }
    this.#record(`view:${id}`);
    this.views = this.views.map((candidate) =>
      candidate.id === id ? { ...candidate, name: title } : candidate,
    );
    this.#saveViewsSoon();
  }

  // Every project has the overview (ADR 0008), so it cannot go.
  deleteView(id: string): void {
    if (id === overviewId || !this.views.some((view) => view.id === id)) {
      return;
    }
    if (id === this.activeView) {
      this.openView(overviewId);
      this.editing = false;
    }
    const layouts = { ...this.#layouts };
    const taken = id in layouts ? { [id]: layouts[id] } : {};
    delete layouts[id];
    this.#record(null, {}, taken);
    this.#layouts = layouts;
    this.views = this.views.filter((view) => view.id !== id);
    this.#saveViewsSoon();
    this.#saveLayoutSoon();
  }

  // The overview shows everything, whatever it is asked to show.
  setViewNodes(id: string, nodes: ViewNodes): void {
    if (id === overviewId || !this.views.some((view) => view.id === id)) {
      return;
    }
    this.#record();
    this.views = this.views.map((view) => (view.id === id ? { ...view, nodes } : view));
    this.#saveViewsSoon();
    if (id === this.activeView) {
      void this.#placeMissing();
    }
  }

  toggleViewNode(id: string, nodeId: string): void {
    const view = this.views.find((candidate) => candidate.id === id);
    if (view === undefined) {
      return;
    }
    const current = [...this.members(view)];
    const nodes = current.includes(nodeId)
      ? current.filter((candidate) => candidate !== nodeId)
      : [...current, nodeId];
    this.setViewNodes(id, nodes);
  }

  // A pending edit is written first, so what the studio generates from is what
  // the canvas shows, and the POST goes on the same chain behind it.
  async generate(): Promise<void> {
    this.#flush();
    this.generated = null;
    this.generating = true;
    await this.#queue(async () => {
      try {
        const written = await generateFiles();
        this.apiErrors = [];
        if (written.length === 0) {
          this.notify('nothing to generate');
          return;
        }
        this.generated = written;
      } catch (failure) {
        if (!(failure instanceof ApiError)) {
          throw failure;
        }
        if (failure.status === 422) {
          this.apiErrors = failure.errors;
          return;
        }
        if (failure.status === 409) {
          this.notify(forceHint(failure.message));
          return;
        }
        throw failure;
      }
    });
    this.generating = false;
  }

  dismissGenerated(): void {
    this.generated = null;
  }

  undo(): Promise<void> {
    return this.#travel('undo');
  }

  redo(): Promise<void> {
    return this.#travel('redo');
  }

  #travel(way: 'undo' | 'redo'): Promise<void> {
    const from = way === 'undo' ? this.undoable : this.redoable;
    const entry = from.at(-1);
    const project = this.project;
    if (entry === undefined || project === null) {
      return Promise.resolve();
    }
    this.#flushViews();
    const views = this.views;
    const positions = this.positions;
    const undoable = this.undoable;
    const redoable = this.redoable;
    const inverse = this.#swap(entry, project, views);
    this.#coalescing = null;
    this.apiErrors = [];
    if (way === 'undo') {
      this.undoable = from.slice(0, -1);
      this.redoable = capped([...redoable, inverse]);
    } else {
      this.redoable = from.slice(0, -1);
      this.undoable = capped([...undoable, inverse]);
    }
    const next = entry.project;
    this.#place();
    const layout = this.layout;
    const moved =
      positions !== this.positions ||
      Object.keys(inverse.layouts).length > 0 ||
      Object.keys(entry.layouts).length > 0;
    const changedViews = entry.views !== views;

    return this.#queue(async () => {
      try {
        if (next !== project) {
          await putProject(next);
        }
      } catch (failure) {
        this.project = project;
        this.views = views;
        this.positions = positions;
        this.undoable = undoable;
        this.redoable = redoable;
        throw failure;
      }
      this.#commit(next);
      if (changedViews) {
        await this.#putViews(entry.views);
      }
      if (moved) {
        await putLayout(layout);
      }
    });
  }

  // Puts the project and the views back to a snapshot and returns the one that
  // would undo that, holding the positions and view layouts this step takes away.
  #swap(entry: Entry, project: Project, views: View[]): Entry {
    const kept = new Set(entry.project.nodes.map((node) => node.id));
    const gone: Record<string, Position> = {};
    for (const node of project.nodes) {
      if (!kept.has(node.id) && node.id in this.positions) {
        gone[node.id] = this.positions[node.id];
      }
    }
    this.project = entry.project;
    if (Object.keys(gone).length > 0 || Object.keys(entry.positions).length > 0) {
      const positions = { ...this.positions };
      for (const id of Object.keys(gone)) {
        delete positions[id];
      }
      this.positions = { ...positions, ...entry.positions };
    }

    const keptViews = new Set(entry.views.map((view) => view.id));
    const goneLayouts: Record<string, ViewLayout> = {};
    const layouts = this.#stashed();
    for (const view of views) {
      if (!keptViews.has(view.id) && view.id in layouts) {
        goneLayouts[view.id] = layouts[view.id];
        delete layouts[view.id];
      }
    }
    this.#layouts = { ...layouts, ...entry.layouts };
    this.#setViews(entry.views);
    return { project, views, positions: gone, layouts: goneLayouts };
  }

  // The active view is the one thing the views list cannot lose from under
  // the canvas: when it goes, the overview takes its place.
  #setViews(views: View[]): void {
    this.views = views;
    if (!views.some((view) => view.id === this.activeView)) {
      this.editing = false;
      this.openView(overviewId);
    }
  }

  // The layout entries with the active view's live one folded in, unless that
  // view has just been removed, in which case nothing of it is kept.
  #stashed(): Record<string, ViewLayout> {
    if (!this.views.some((view) => view.id === this.activeView)) {
      return { ...this.#layouts };
    }
    return {
      ...this.#layouts,
      [this.activeView]: { nodes: this.positions, viewport: this.viewport },
    };
  }

  #record(
    key: string | null = null,
    positions: Record<string, Position> = {},
    layouts: Record<string, ViewLayout> = {},
  ): void {
    const project = this.project;
    if (project === null) {
      return;
    }
    this.apiErrors = [];
    this.redoable = [];
    if (key !== null && key === this.#coalescing) {
      return;
    }
    this.#coalescing = key;
    const views = this.views;
    this.undoable = capped([...this.undoable, { project, views, positions, layouts }]);
  }

  #forget(): void {
    this.undoable = [];
    this.redoable = [];
    this.#coalescing = null;
  }

  // Removes only the refused item by id, rather than the whole snapshot, so a
  // second change queued behind a refused one is not lost.
  #discard(nodes: string[], edges: string[]): void {
    const project = this.project;
    if (project === null) {
      return;
    }
    this.project = {
      ...project,
      nodes: project.nodes.filter((node) => !nodes.includes(node.id)),
      edges: project.edges.filter((edge) => !edges.includes(edge.id)),
    };
    const positions = { ...this.positions };
    for (const id of nodes) {
      delete positions[id];
    }
    this.positions = positions;
    for (const id of nodes) {
      this.views = this.#left(this.views, id);
    }
  }

  #takeSelection(): Selection {
    const selection: Selection = { node: this.selectedNodeId, edge: this.selectedEdgeId };
    this.selectedNodeId = null;
    this.selectedEdgeId = null;
    return selection;
  }

  #putBackSelection(selection: Selection): void {
    this.selectedNodeId = selection.node;
    this.selectedEdgeId = selection.edge;
  }

  #putBack(taken: Taken): void {
    const project = this.project;
    if (project === null) {
      return;
    }
    this.project = {
      ...project,
      nodes: reinsert(project.nodes, taken.nodes),
      edges: reinsert(project.edges, taken.edges),
    };
    this.positions = { ...this.positions, ...taken.positions };
    for (const { item } of taken.nodes) {
      for (const viewId of taken.memberships) {
        this.views = this.#joined(this.views, viewId, item.id);
      }
    }
  }

  #joined(views: View[], viewId: string, nodeId: string): View[] {
    const view = views.find((candidate) => candidate.id === viewId);
    if (view === undefined || view.nodes === '*' || view.nodes.includes(nodeId)) {
      return views;
    }
    const nodes = [...view.nodes, nodeId];
    return views.map((candidate) =>
      candidate.id === viewId ? { ...candidate, nodes } : candidate,
    );
  }

  #left(views: View[], nodeId: string): View[] {
    if (!views.some((view) => view.nodes !== '*' && view.nodes.includes(nodeId))) {
      return views;
    }
    return views.map((view) =>
      view.nodes === '*' || !view.nodes.includes(nodeId)
        ? view
        : { ...view, nodes: view.nodes.filter((id) => id !== nodeId) },
    );
  }

  moveNode(id: string, position: Position): void {
    this.moveNodes({ [id]: position });
  }

  moveNodes(positions: Record<string, Position>): void {
    const rounded: Record<string, Position> = {};
    for (const [id, position] of Object.entries(positions)) {
      rounded[id] = round(position);
    }
    this.positions = { ...this.positions, ...rounded };
    this.#saveLayoutSoon();
  }

  setViewport(viewport: Viewport): void {
    const next = round3(viewport);
    if (sameViewport(next, this.viewport)) {
      return;
    }
    this.viewport = next;
    this.#saveLayoutSoon();
  }

  // The store is the only owner of selection: Svelte Flow marks it by replacing
  // the node object, and that would otherwise be lost the next time this runs.
  #card(
    node: Node,
    position: Position,
    errors: number,
    style: Resolved,
    subtotal: number | null,
    dimmed: boolean,
  ): FlowNode {
    const selected = node.id === this.selectedNodeId;
    const editing = this.editing;
    const key = [
      node.name,
      node.type,
      position.x,
      position.y,
      selected,
      errors,
      style.color,
      style.icon,
      style.shape,
      subtotal,
      dimmed,
      editing,
    ]
      .map(String)
      .join('|');
    const held = this.#cards.get(node.id);
    if (held !== undefined && held.key === key) {
      return held.card;
    }
    const card: FlowNode = {
      id: node.id,
      type: 'togen',
      position,
      selected,
      selectable: !editing,
      draggable: !dimmed,
      connectable: !dimmed,
      data: { name: node.name, type: node.type, errors, style, subtotal, dimmed },
    };
    this.#cards.set(node.id, { key, card });
    return card;
  }

  // A node outside the view is drawn where the overview has it while the
  // editor is open, which is the map the user already knows.
  #shownAt(id: string): Position {
    return this.positions[id] ?? this.#layouts[overviewId]?.nodes[id] ?? origin;
  }

  // A views file the studio cannot read degrades to the overview and says why
  // (ADR 0008); the next views save writes a sound file over it.
  async #readViews(): Promise<boolean> {
    let views: View[];
    let sound = true;
    try {
      views = (await getViews()).views;
    } catch (failure) {
      this.#report(failure);
      views = [overview];
      sound = false;
    }
    this.#savedViews = views;
    this.views = views;
    if (!views.some((view) => view.id === this.activeView)) {
      this.activeView = overviewId;
      this.editing = false;
    }
    return sound;
  }

  // A missing or broken layout file (404, or 422 from a hand-edited file) is
  // normal and dagre supplies one; anything else is reported and left alone,
  // rather than being overwritten by an auto-layout the user never asked for.
  async #readLayout(): Promise<boolean> {
    let layout: Layout;
    try {
      layout = await getLayout();
    } catch (failure) {
      if (!isMissingLayout(failure)) {
        this.#report(failure);
        return false;
      }
      layout = freshLayout();
    }
    this.#layouts = layout.views ?? {};
    const entry = this.#layouts[this.activeView];
    this.positions = entry?.nodes ?? {};
    this.viewport = entry?.viewport ?? { x: 0, y: 0, zoom: 1 };
    return true;
  }

  async #placeMissing(): Promise<void> {
    if (!this.#place()) {
      return;
    }
    const layout = this.layout;
    await this.#queue(() => putLayout(layout));
  }

  // Dagre over the view's own nodes and edges places whichever of them has no
  // position in this view yet.
  #place(): boolean {
    const project = this.project;
    if (project === null) {
      return false;
    }
    const visible = this.visibleNodeIds;
    const nodes = project.nodes.filter((node) => visible.has(node.id));
    if (nodes.every((node) => node.id in this.positions)) {
      return false;
    }
    const edges = project.edges.filter((edge) => visible.has(edge.from) && visible.has(edge.to));
    this.positions = { ...autoLayout({ nodes, edges }), ...this.positions };
    return true;
  }

  // Every project save the studio accepted moves the estimate, on the same
  // debounce as the saves so a run of edits asks once.
  #commit(project: Project): void {
    this.#saved = project;
    clearTimeout(this.#costTimer);
    this.#costTimer = setTimeout(() => void this.loadCost(), saveDelay);
  }

  #saveProjectSoon(): void {
    clearTimeout(this.#nodeTimer);
    this.#nodeTimer = setTimeout(() => this.#saveProjectNow(), saveDelay);
  }

  #saveProjectNow(): void {
    this.#nodeTimer = undefined;
    this.#coalescing = null;
    const nodes = [...this.#edited];
    const edges = [...this.#editedEdges];
    this.#edited.clear();
    this.#editedEdges.clear();
    void this.#queue(async () => {
      const project = this.project;
      if (project === null) {
        return;
      }
      try {
        await putProject(project);
      } catch (failure) {
        this.#revert(nodes, edges);
        throw failure;
      }
      this.#commit(project);
    });
  }

  #saveViewsSoon(): void {
    clearTimeout(this.#viewTimer);
    this.#viewTimer = setTimeout(() => this.#saveViewsNow(), saveDelay);
  }

  #saveViewsNow(): void {
    this.#viewTimer = undefined;
    this.#coalescing = null;
    void this.#queue(() => this.#putViews(this.views));
  }

  // A refused views file goes back to the last one the studio accepted.
  async #putViews(views: View[]): Promise<void> {
    try {
      await putViews(file(views));
    } catch (failure) {
      this.#setViews(this.#savedViews);
      throw failure;
    }
    this.#savedViews = views;
  }

  // Generate reads the files on disk, so anything the debounce is still holding
  // goes now rather than after it.
  #flush(): void {
    if (this.#nodeTimer !== undefined) {
      clearTimeout(this.#nodeTimer);
      this.#saveProjectNow();
    }
    this.#flushViews();
    this.#flushLayout();
  }

  #flushViews(): void {
    if (this.#viewTimer !== undefined) {
      clearTimeout(this.#viewTimer);
      this.#saveViewsNow();
    }
  }

  #flushLayout(): void {
    if (this.#timer !== undefined) {
      clearTimeout(this.#timer);
      this.#saveLayoutNow();
    }
  }

  #revert(nodes: string[], edges: string[]): void {
    const saved = this.#saved;
    const project = this.project;
    if (saved === null || project === null) {
      return;
    }
    // A refused item that the last save never held is dropped, not kept: the
    // serial chain saves an addition before any edit to it, so this is only a
    // guard on that order.
    this.project = {
      ...project,
      nodes: restored(project.nodes, saved.nodes, nodes),
      edges: restored(project.edges, saved.edges, edges),
    };
  }

  #saveLayoutSoon(): void {
    clearTimeout(this.#timer);
    this.#timer = setTimeout(() => this.#saveLayoutNow(), saveDelay);
  }

  #saveLayoutNow(): void {
    this.#timer = undefined;
    const layout = this.layout;
    void this.#queue(() => putLayout(layout));
  }

  #queue(work: () => Promise<void>): Promise<void> {
    this.#running += 1;
    this.saving = true;
    const done = this.#chain.then(async () => {
      try {
        await work();
        this.error = null;
      } catch (failure) {
        this.#report(failure);
      }
    });
    this.#chain = done.then(() => {
      this.#running -= 1;
      this.saving = this.#running > 0;
      this.#settle();
    });
    return done;
  }

  #settle(): void {
    if (this.#saving() || !this.#stale) {
      return;
    }
    this.#stale = false;
    void this.load();
  }

  #saving(): boolean {
    return (
      this.#timer !== undefined ||
      this.#nodeTimer !== undefined ||
      this.#viewTimer !== undefined ||
      this.#running > 0
    );
  }

  #report(failure: unknown): void {
    if (failure instanceof ApiError) {
      this.error = failure;
      return;
    }
    this.error = new ApiError(0, [], failure instanceof Error ? failure.message : String(failure));
  }
}

const key = Symbol('togen.store');

export function setStore(store: Store): void {
  setContext(key, store);
}

export function getStore(): Store {
  return getContext<Store>(key);
}

export function errorLines(error: ApiError): string[] {
  if (error.errors.length === 0) {
    return [error.message];
  }
  return error.errors.map(errorLine);
}

// The id the views file wants: lower case kebab, starting with a letter,
// at most 32 characters.
export function kebab(name: string): string {
  const slug = name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^[^a-z]+/, '')
    .replace(/-+$/, '')
    .slice(0, 32)
    .replace(/-+$/, '');
  return slug === '' ? 'view' : slug;
}

function freeId(slug: string, views: View[]): string {
  const taken = new Set(views.map((view) => view.id));
  if (!taken.has(slug)) {
    return slug;
  }
  let index = 2;
  while (taken.has(suffixed(slug, index))) {
    index += 1;
  }
  return suffixed(slug, index);
}

function suffixed(slug: string, index: number): string {
  const tail = `-${index}`;
  return `${slug.slice(0, 32 - tail.length).replace(/-+$/, '')}${tail}`;
}

function file(views: View[]): Views {
  return { version: 1, views };
}

function describe(failure: unknown): string {
  if (failure instanceof ApiError) {
    return errorLines(failure)[0];
  }
  return failure instanceof Error ? failure.message : String(failure);
}

function capped(entries: Entry[]): Entry[] {
  return entries.length > historyLimit ? entries.slice(-historyLimit) : entries;
}

// The studio cannot replace a directory it did not write on its own, so the
// notice says where the switch is.
function forceHint(message: string): string {
  const first = message.split('\n')[0];
  return `${first}. Run 'togen generate --force' in a terminal to replace the directory.`;
}

function tally(errors: ValidationError[], part: 'nodeId' | 'edgeId'): Record<string, number> {
  const counted: Record<string, number> = {};
  for (const error of errors) {
    const id = error[part];
    if (id !== undefined && id !== '') {
      counted[id] = (counted[id] ?? 0) + 1;
    }
  }
  return counted;
}

function duplicate(project: Project, from: string, to: string, relation: Relation): string {
  const name = (id: string) => project.nodes.find((node) => node.id === id)?.name ?? id;
  return `duplicate '${relation}' edge from '${name(from)}' to '${name(to)}'`;
}

function held<T>(items: T[], match: (item: T) => boolean): Held<T> {
  return items.flatMap((item, index) => (match(item) ? [{ index, item }] : []));
}

function reinsert<T>(items: T[], taken: Held<T>): T[] {
  const out = [...items];
  for (const { index, item } of taken) {
    out.splice(Math.min(index, out.length), 0, item);
  }
  return out;
}

function restored<T extends { id: string }>(items: T[], saved: T[], ids: string[]): T[] {
  return items.flatMap((item) => {
    if (!ids.includes(item.id)) {
      return [item];
    }
    const previous = saved.find((candidate) => candidate.id === item.id);
    return previous === undefined ? [] : [previous];
  });
}

function patchedNode(node: Node, patch: Patch): Node {
  const next: Node = { ...node };
  if (patch.name !== undefined) {
    next.name = patch.name;
  }
  if (patch.properties !== undefined) {
    settle(next, merged(node.properties, patch.properties));
  }
  return next;
}

function patchedEdge(edge: Edge, patch: Record<string, unknown>): Edge {
  const next: Edge = { ...edge };
  settle(next, merged(edge.properties, patch));
  return next;
}

function merged(
  current: Record<string, unknown> | undefined,
  patch: Record<string, unknown>,
): Record<string, unknown> | undefined {
  const properties = { ...(current ?? {}) };
  for (const [key, value] of Object.entries(patch)) {
    if (value === undefined || value === '') {
      delete properties[key];
    } else {
      properties[key] = value;
    }
  }
  return Object.keys(properties).length === 0 ? undefined : properties;
}

function settle(
  item: { properties?: Record<string, unknown> },
  properties: Record<string, unknown> | undefined,
): void {
  if (properties === undefined) {
    delete item.properties;
  } else {
    item.properties = properties;
  }
}

function freshLayout(): Layout {
  return { version: 2, views: {} };
}

function isMissingLayout(failure: unknown): boolean {
  return failure instanceof ApiError && (failure.status === 404 || failure.status === 422);
}

function nextIndex(project: Project, type: NodeType): number {
  const taken = new Set<number>();
  const pattern = new RegExp(`^${type}-(\\d+)$`);
  for (const node of project.nodes) {
    for (const value of [node.id, node.name]) {
      const match = pattern.exec(value);
      if (match !== null) {
        taken.add(Number(match[1]));
      }
    }
  }
  return free(taken);
}

function nextEdgeIndex(project: Project): number {
  const taken = new Set<number>();
  for (const edge of project.edges) {
    const match = /^edge-(\d+)$/.exec(edge.id);
    if (match !== null) {
      taken.add(Number(match[1]));
    }
  }
  return free(taken);
}

function free(taken: Set<number>): number {
  let index = 1;
  while (taken.has(index)) {
    index += 1;
  }
  return index;
}

function round(position: Position): Position {
  return { x: Math.round(position.x), y: Math.round(position.y) };
}

function round3(viewport: Viewport): Viewport {
  return {
    x: Math.round(viewport.x),
    y: Math.round(viewport.y),
    zoom: Math.round(viewport.zoom * 1000) / 1000,
  };
}

function sameViewport(a: Viewport, b: Viewport): boolean {
  return a.x === b.x && a.y === b.y && a.zoom === b.zoom;
}
