import { expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

import App from './App.svelte';
import './app.css';
import type {
  Config,
  Cost,
  Example,
  InitRequest,
  Layout,
  Position,
  Project,
  Viewport,
  Views,
  Workspace,
} from './lib/types.ts';

export type Call = { method: string; path: string; body: unknown; rawBody: string | undefined };

type Served = {
  project: Project | null;
  layout: Layout | null;
  config: Config;
  views: Views;
  workspace: Workspace;
  examples: Example[];
};

let made: Call[] = [];
let refusal: ((call: Call) => Response | undefined) | undefined;
let opened: FakeSocket[] = [];
let estimate: Cost | undefined;

const browserFetch = window.fetch.bind(window);

class FakeSocket {
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;

  constructor(readonly url: string) {
    opened.push(this);
  }

  close() {}
}

export const defaults: Config = {
  version: 1,
  targets: ['hcl'],
  outDir: 'infra',
  style: { theme: 'dark' },
};

export const overviewOnly: Views = {
  version: 1,
  views: [{ id: 'overview', name: 'Overview', nodes: '*' }],
};

export const shopDir: Workspace = { dir: '/home/paul/code/shop', name: 'shop' };

export const bundled: Example[] = [
  { id: 'aws-basic', description: 'One of every node type, wired up with the defaults' },
  { id: 'aws-full', description: 'Every node type, relation and property this milestone supports' },
];

export function reset() {
  made = [];
  opened = [];
  refusal = undefined;
  estimate = undefined;
  vi.stubGlobal('WebSocket', FakeSocket);
  window.sessionStorage.clear();
}

export function serve(
  project: Project,
  layout: Layout,
  config: Config = defaults,
  views: Views = overviewOnly,
) {
  install({ project, layout, config, views, workspace: shopDir, examples: bundled });
}

// A directory with no togen/ yet: the project and the layout answer 404 until
// a POST to init creates them, as the studio does.
export function serveEmpty(
  workspace: Workspace = shopDir,
  examples: Example[] = bundled,
  config: Config = defaults,
) {
  install({ project: null, layout: null, config, views: overviewOnly, workspace, examples });
}

function install(served: Served) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string, init?: RequestInit) => {
      // Only the API is faked; the fonts an export embeds come from Vite as they would.
      if (!String(input).startsWith('/api')) {
        return browserFetch(input, init);
      }
      const call: Call = {
        method: init?.method ?? 'GET',
        path: String(input),
        body: init?.body === undefined ? undefined : JSON.parse(String(init.body)),
        rawBody: init?.body === undefined ? undefined : String(init.body),
      };
      made.push(call);
      const refused = refusal?.(call);
      if (refused !== undefined) {
        return refused;
      }
      if (call.method === 'PUT') {
        return new Response(null, { status: 204 });
      }
      if (call.method === 'POST' && call.path === '/api/project/init') {
        served.project = created(call.body as InitRequest);
        served.layout = overviewLayout({});
        return answer(201, { written: ['togen/project.json', 'togen/layout.json', 'togen.yml'] });
      }
      if (call.path === '/api/project') {
        return served.project === null ? missing('togen/project.json') : json(served.project);
      }
      if (call.path === '/api/layout') {
        return served.layout === null ? missing('togen/layout.json') : json(served.layout);
      }
      if (call.path === '/api/config') {
        return json(served.config);
      }
      if (call.path === '/api/views') {
        return json(served.views);
      }
      if (call.path === '/api/workspace') {
        return json(served.workspace);
      }
      if (call.path === '/api/examples') {
        return json({ examples: served.examples });
      }
      if (call.path === '/api/cost') {
        return served.project === null
          ? missing('togen/project.json')
          : json(estimate ?? unpriced(served.project));
      }
      return new Response(null, { status: 404 });
    }),
  );
}

export function overviewLayout(
  nodes: Record<string, Position>,
  viewport: Viewport = { x: 0, y: 0, zoom: 1 },
): Layout {
  return { version: 2, views: { overview: { nodes, viewport } } };
}

// What the studio writes for the request: the sketch as an empty project, or a
// small stand-in for the bundled example.
function created(request: InitRequest): Project {
  if ('example' in request) {
    return {
      version: 1,
      name: request.example,
      provider: 'aws',
      region: 'eu-west-2',
      environment: 'dev',
      nodes: [{ id: 'gateway-1', type: 'gateway', name: 'api' }],
      edges: [],
    };
  }
  return { version: 1, ...request, nodes: [], edges: [] };
}

function missing(name: string): Response {
  return answer(404, { error: `${name} not found. Run 'togen init' first.` });
}

export function refuse(answer: ((call: Call) => Response | undefined) | undefined) {
  refusal = answer;
}

export function serveCost(cost: Cost) {
  estimate = cost;
}

export function invalid(errors: { path: string; nodeId?: string; message: string }[]): Response {
  return new Response(JSON.stringify({ errors }), {
    status: 422,
    headers: { 'Content-Type': 'application/json' },
  });
}

export function answer(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

export function calls(): Call[] {
  return made;
}

export function puts(path: string): Call[] {
  return made.filter((call) => call.method === 'PUT' && call.path === path);
}

export function posts(path: string): Call[] {
  return made.filter((call) => call.method === 'POST' && call.path === path);
}

export function gets(path: string): Call[] {
  return made.filter((call) => call.method === 'GET' && call.path === path);
}

export function socket(): FakeSocket | undefined {
  return opened.at(-1);
}

export function settle(ms = 500): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export async function show() {
  const screen = await mount(900, 600);
  await vi.waitFor(() => expect(screen.container.querySelector('.svelte-flow')).toBeInTheDocument());
  return screen;
}

// Until a test says otherwise, nothing is priced: every node listed, nothing summed.
function unpriced(project: Project): Cost {
  return {
    provider: project.provider,
    region: project.region,
    currency: 'USD',
    items: [],
    notPriced: project.nodes.map((node) => ({
      name: node.name,
      kind: node.type,
      reason: `no ${project.provider} prices for this node type yet`,
    })),
    total: 0,
    snapshotDate: '2026-09-05',
    note: 'list prices from 2026-09-05, estimate not a quote',
  };
}

// Wide enough for the three choice cards to sit in a row, as designed.
export async function showFirstRun() {
  const screen = await mount(1200, 700);
  await vi.waitFor(() =>
    expect(screen.container.querySelector('[data-screen="first-run"]')).toBeInTheDocument(),
  );
  return screen;
}

async function mount(width: number, height: number) {
  const screen = await render(App);
  screen.container.style.width = `${width}px`;
  screen.container.style.height = `${height}px`;
  return screen;
}

function json(value: unknown): Response {
  return new Response(JSON.stringify(value), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });
}
