import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  // `pnpm dev` talks to a `togen studio` on the default port. `fs.allow` opens
  // the repository root so the palette can import `schema/project.schema.json`.
  server: {
    fs: { allow: ['..'] },
    proxy: {
      '/api': { target: 'http://127.0.0.1:3000', ws: true },
    },
  },
});
