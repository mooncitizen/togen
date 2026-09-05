import { getContext, setContext } from 'svelte';
import type { Edge as FlowEdge, Node as FlowNode } from '@xyflow/svelte';

import { ApiError, getLayout, getProject, putLayout, putProject } from './api.ts';
import { defaultProperties } from './catalogue.ts';
import { autoLayout } from './layout.ts';
import type { Layout, Node, NodeType, Position, Project, Viewport } from './types.ts';

const saveDelay = 300;
const origin: Position = { x: 0, y: 0 };

// Positions and the viewport are separate state so that panning the canvas
// does not touch the nodes: Svelte Flow re-measures and briefly hides any node
// whose object it has not seen before, and a node being measured cannot be
// grabbed.
export class Store {
  project = $state.raw<Project | null>(null);
  positions = $state.raw<Record<string, Position>>({});
  viewport = $state.raw<Viewport>({ x: 0, y: 0, zoom: 1 });
  error = $state.raw<ApiError | null>(null);
  ready = $state(false);

  readonly flowNodes: FlowNode[] = $derived(
    (this.project?.nodes ?? []).map((node) => this.#card(node, this.positions[node.id] ?? origin)),
  );

  readonly flowEdges: FlowEdge[] = $derived(
    (this.project?.edges ?? []).map((edge) => ({
      id: edge.id,
      source: edge.from,
      target: edge.to,
      label: edge.relation,
    })),
  );

  get layout(): Layout {
    return { version: this.#version, nodes: this.positions, viewport: this.viewport };
  }

  #version = 1;
  #cards = new Map<string, { key: string; card: FlowNode }>();
  #timer: ReturnType<typeof setTimeout> | undefined;
  #chain: Promise<void> = Promise.resolve();
  #running = 0;
  #stale = false;

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
        this.#discardNode(name);
        throw failure;
      }
      await putLayout(layout);
    });
  }

  // Removes only the refused node by id, rather than the whole snapshot, so a
  // second drop queued behind a refused one is not lost.
  #discardNode(id: string): void {
    if (this.project !== null) {
      this.project = { ...this.project, nodes: this.project.nodes.filter((node) => node.id !== id) };
    }
    const positions = { ...this.positions };
    delete positions[id];
    this.positions = positions;
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

  #card(node: Node, position: Position): FlowNode {
    const key = `${node.name}|${node.type}|${position.x}|${position.y}`;
    const held = this.#cards.get(node.id);
    if (held !== undefined && held.key === key) {
      return held.card;
    }
    const card: FlowNode = {
      id: node.id,
      type: 'togen',
      position,
      data: { name: node.name, type: node.type },
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
    return this.#timer !== undefined || this.#running > 0;
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
  return error.errors.map((entry) => {
    const path = entry.path === '' ? 'project' : entry.path;
    if (entry.nodeId !== undefined && entry.nodeId !== '') {
      return `${path} (node ${entry.nodeId}): ${entry.message}`;
    }
    if (entry.edgeId !== undefined && entry.edgeId !== '') {
      return `${path} (edge ${entry.edgeId}): ${entry.message}`;
    }
    return `${path}: ${entry.message}`;
  });
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
