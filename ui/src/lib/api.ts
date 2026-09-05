import type { Layout, Project, ValidationError } from './types.ts';

export class ApiError extends Error {
  status: number;
  errors: ValidationError[];

  constructor(status: number, errors: ValidationError[], message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.errors = errors;
  }
}

export function getProject(): Promise<Project> {
  return read<Project>('/api/project');
}

export function putProject(project: Project): Promise<void> {
  return write('/api/project', project);
}

export function getLayout(): Promise<Layout> {
  return read<Layout>('/api/layout');
}

export function putLayout(layout: Layout): Promise<void> {
  return write('/api/layout', layout);
}

const firstRetry = 500;
const longestRetry = 10_000;

export function events(onEvent: (name: string) => void): () => void {
  let socket: WebSocket | undefined;
  let retry: ReturnType<typeof setTimeout> | undefined;
  let wait = firstRetry;
  let stopped = false;

  const open = () => {
    socket = new WebSocket(socketURL());
    socket.onopen = () => {
      wait = firstRetry;
    };
    socket.onmessage = (message: MessageEvent) => {
      const name = eventName(message.data);
      if (name !== undefined) {
        onEvent(name);
      }
    };
    socket.onclose = () => {
      if (stopped) {
        return;
      }
      retry = setTimeout(open, wait);
      wait = Math.min(wait * 2, longestRetry);
    };
  };
  open();

  return () => {
    stopped = true;
    if (retry !== undefined) {
      clearTimeout(retry);
    }
    socket?.close();
  };
}

function socketURL(): string {
  const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${scheme}//${window.location.host}/api/events`;
}

function eventName(data: unknown): string | undefined {
  if (typeof data !== 'string') {
    return undefined;
  }
  try {
    const message: unknown = JSON.parse(data);
    if (message !== null && typeof message === 'object' && 'event' in message) {
      const name = (message as { event: unknown }).event;
      return typeof name === 'string' ? name : undefined;
    }
  } catch {
    return undefined;
  }
  return undefined;
}

async function read<T>(path: string): Promise<T> {
  const response = await fetch(path, { headers: { Accept: 'application/json' } });
  if (!response.ok) {
    throw await failure(response);
  }
  return (await response.json()) as T;
}

// The studio stores the request body verbatim, so it is formatted the way the
// CLI writes these files: project.json is reviewed in diffs (ADR 0004).
async function write(path: string, body: unknown): Promise<void> {
  const response = await fetch(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: `${JSON.stringify(body, null, 2)}\n`,
  });
  if (!response.ok) {
    throw await failure(response);
  }
}

async function failure(response: Response): Promise<ApiError> {
  const errors = await validationErrors(response);
  if (errors.length > 0) {
    return new ApiError(response.status, errors, errors[0].message);
  }
  return new ApiError(response.status, [], `the studio answered ${response.status}`);
}

async function validationErrors(response: Response): Promise<ValidationError[]> {
  let body: unknown;
  try {
    body = await response.json();
  } catch {
    return [];
  }
  if (body === null || typeof body !== 'object') {
    return [];
  }
  const { errors, error } = body as { errors?: unknown; error?: unknown };
  if (Array.isArray(errors)) {
    return errors as ValidationError[];
  }
  if (typeof error === 'string') {
    return [{ path: '', message: error }];
  }
  return [];
}
