import { expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

import App from './App.svelte';
import './app.css';
import type { Layout, Project } from './lib/types.ts';

export type Call = { method: string; path: string; body: unknown; rawBody: string | undefined };

let made: Call[] = [];
let refusal: ((call: Call) => Response | undefined) | undefined;
let opened: FakeSocket[] = [];

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

export function reset() {
  made = [];
  opened = [];
  refusal = undefined;
  vi.stubGlobal('WebSocket', FakeSocket);
}

export function serve(project: Project, layout: Layout) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string, init?: RequestInit) => {
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
      if (call.path === '/api/project') {
        return json(project);
      }
      if (call.path === '/api/layout') {
        return json(layout);
      }
      return new Response(null, { status: 404 });
    }),
  );
}

export function refuse(answer: ((call: Call) => Response | undefined) | undefined) {
  refusal = answer;
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
  const screen = await render(App);
  screen.container.style.width = '900px';
  screen.container.style.height = '600px';
  await vi.waitFor(() => expect(screen.container.querySelector('.svelte-flow')).toBeInTheDocument());
  return screen;
}

function json(value: unknown): Response {
  return new Response(JSON.stringify(value), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });
}
