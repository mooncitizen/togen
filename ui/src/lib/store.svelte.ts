import { getContext, setContext } from 'svelte';
import type { Edge as FlowEdge, Node as FlowNode } from '@xyflow/svelte';

import { ApiError, getLayout, getProject, putLayout, putProject } from './api.ts';
import { defaultProperties } from './catalogue.ts';
import { autoLayout } from './layout.ts';
import type {
  Edge,
  Layout,
  Node,
  NodeType,
  Position,
  Project,
  Relation,
  ValidationError,
  Viewport,
} from './types.ts';
import { errorLine, validateProject } from './validate.ts';

type Patch = { name?: string; properties?: Record<string, unknown> };

type Held<T> = { index: number; item: T }[];

type Taken = { nodes: Held<Node>; edges: Held<Edge>; positions: Record<string, Position> };

type Selection = { node: string | null; edge: string | null };

const saveDelay = 300;
const noticeDelay = 6000;
const origin: Position = { x: 0, y: 0 };

// Positions and the viewport are separate state so that panning the canvas
// does not touch the nodes: Svelte Flow re-measures and briefly hides any node
// whose object it has not seen before, and a node being measured cannot be
// grabbed.
export class Store {
  project = $state.raw<Project | null>(null);
  positions = $state.raw<Record<string, Position>>({});
  viewport = $state.raw<Viewport>({ x: 0, y: 0, zoom: 1 });
  selectedNodeId = $state.raw<string | null>(null);
  selectedEdgeId = $state.raw<string | null>(null);
  error = $state.raw<ApiError | null>(null);
  notice = $state.raw<string | null>(null);
  ready = $state(false);

  // The same checks as the CLI, run on every change, separate from what the
  // studio said about the last save.
  readonly errors: ValidationError[] = $derived(
    this.project === null ? [] : validateProject(this.project),
  );

  readonly flowNodes: FlowNode[] = $derived.by(() => {
    const counted = tally(this.errors, 'nodeId');
    return (this.project?.nodes ?? []).map((node) =>
      this.#card(node, this.positions[node.id] ?? origin, counted[node.id] ?? 0),
    );
  });

  readonly flowEdges: FlowEdge[] = $derived.by(() => {
    const counted = tally(this.errors, 'edgeId');
    return (this.project?.edges ?? []).map((edge) => ({
      id: edge.id,
      source: edge.from,
      target: edge.to,
      label: edge.relation,
      selected: edge.id === this.selectedEdgeId,
      class: edge.id in counted ? 'togen-edge-error' : undefined,
    }));
  });

  get layout(): Layout {
    return { version: this.#version, nodes: this.positions, viewport: this.viewport };
  }

  #version = 1;
  #cards = new Map<string, { key: string; card: FlowNode }>();
  #timer: ReturnType<typeof setTimeout> | undefined;
  #nodeTimer: ReturnType<typeof setTimeout> | undefined;
  #noticeTimer: ReturnType<typeof setTimeout> | undefined;
  #chain: Promise<void> = Promise.resolve();
  #running = 0;
  #stale = false;
  #saved: Project | null = null;
  #edited = new Set<string>();
  #editedEdges = new Set<string>();

  async load(): Promise<void> {
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
    const found = await this.#readLayout();
    this.ready = true;
    if (!found) {
      return;
    }
    this.error = null;
    await this.#placeMissing();
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

  addNode(type: NodeType, position: Position): Promise<void> {
    const project = this.project;
    if (project === null) {
      return Promise.resolve();
    }
    const name = `${type}-${nextIndex(project, type)}`;
    const node: Node = { id: name, type, name, properties: defaultProperties(type) };
    const next: Project = { ...project, nodes: [...project.nodes, node] };
    this.project = next;
    this.positions = { ...this.positions, [name]: round(position) };
    const layout = this.layout;

    return this.#queue(async () => {
      try {
        await putProject(next);
      } catch (failure) {
        this.#discard([name], []);
        throw failure;
      }
      this.#saved = next;
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
      this.#saved = next;
    }).then(() => saved);
  }

  deleteNode(id: string): Promise<void> {
    const project = this.project;
    if (project === null || !project.nodes.some((node) => node.id === id)) {
      return Promise.resolve();
    }
    const taken: Taken = {
      nodes: held(project.nodes, (node) => node.id === id),
      edges: held(project.edges, (edge) => edge.from === id || edge.to === id),
      positions: id in this.positions ? { [id]: this.positions[id] } : {},
    };
    const next: Project = {
      ...project,
      nodes: project.nodes.filter((node) => node.id !== id),
      edges: project.edges.filter((edge) => edge.from !== id && edge.to !== id),
    };
    this.project = next;
    const positions = { ...this.positions };
    delete positions[id];
    this.positions = positions;
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
      this.#saved = next;
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
    };
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
      this.#saved = next;
    });
  }

  select(id: string | null): void {
    this.selectedNodeId = id;
    if (id !== null) {
      this.selectedEdgeId = null;
    }
  }

  selectEdge(id: string | null): void {
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
    this.project = {
      ...project,
      edges: project.edges.map((edge) => (edge.id === id ? patchedEdge(edge, properties) : edge)),
    };
    this.#editedEdges.add(id);
    this.#saveProjectSoon();
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
    this.viewport = round3(viewport);
    this.#saveLayoutSoon();
  }

  // The store is the only owner of selection: Svelte Flow marks it by replacing
  // the node object, and that would otherwise be lost the next time this runs.
  #card(node: Node, position: Position, errors: number): FlowNode {
    const selected = node.id === this.selectedNodeId;
    const key = `${node.name}|${node.type}|${position.x}|${position.y}|${selected}|${errors}`;
    const held = this.#cards.get(node.id);
    if (held !== undefined && held.key === key) {
      return held.card;
    }
    const card: FlowNode = {
      id: node.id,
      type: 'togen',
      position,
      selected,
      data: { name: node.name, type: node.type, errors },
    };
    this.#cards.set(node.id, { key, card });
    return card;
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
    this.#version = layout.version ?? 1;
    this.positions = layout.nodes ?? {};
    this.viewport = layout.viewport ?? { x: 0, y: 0, zoom: 1 };
    return true;
  }

  async #placeMissing(): Promise<void> {
    const project = this.project;
    if (project === null || project.nodes.every((node) => node.id in this.positions)) {
      return;
    }
    this.positions = { ...autoLayout(project), ...this.positions };
    const layout = this.layout;
    await this.#queue(() => putLayout(layout));
  }

  #saveProjectSoon(): void {
    clearTimeout(this.#nodeTimer);
    this.#nodeTimer = setTimeout(() => {
      this.#nodeTimer = undefined;
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
        this.#saved = project;
      });
    }, saveDelay);
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
    this.#timer = setTimeout(() => {
      this.#timer = undefined;
      void this.#queue(() => putLayout(this.layout));
    }, saveDelay);
  }

  #queue(work: () => Promise<void>): Promise<void> {
    this.#running += 1;
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
    return this.#timer !== undefined || this.#nodeTimer !== undefined || this.#running > 0;
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
  return { version: 1, nodes: {}, viewport: { x: 0, y: 0, zoom: 1 } };
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
