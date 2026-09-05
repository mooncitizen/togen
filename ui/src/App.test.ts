import { afterEach, expect, test, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

import App from './App.svelte';

afterEach(() => {
  vi.unstubAllGlobals();
});

test('the bar names the project and the canvas is on the page', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify({ version: 1, name: 'shop' }), { status: 200 })),
  );

  const screen = await render(App);

  await expect.element(screen.getByText('Togen studio')).toBeInTheDocument();
  await expect.element(screen.getByText('shop')).toBeInTheDocument();
  expect(screen.container.querySelector('.svelte-flow')).toBeInTheDocument();
});
