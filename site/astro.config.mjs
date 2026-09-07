import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://mooncitizen.github.io',
  base: '/togen',
  trailingSlash: 'always',
  integrations: [
    starlight({
      title: 'Togen',
      description:
        'Sketch a system as boxes and edges, get a starting infrastructure repository. Local, deterministic, no account needed.',
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/mooncitizen/togen' },
      ],
      editLink: {
        baseUrl: 'https://github.com/mooncitizen/togen/edit/main/site/',
      },
      customCss: ['./src/styles/togen.css'],
      head: [
        {
          tag: 'link',
          attrs: {
            rel: 'stylesheet',
            href: 'https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;500;600&family=IBM+Plex+Mono:wght@400;500&display=swap',
          },
        },
      ],
      sidebar: [
        {
          label: 'Getting started',
          items: [
            { label: 'Install', slug: 'getting-started/install' },
            { label: 'Quickstart', slug: 'getting-started/quickstart' },
            { label: 'Your first project', slug: 'getting-started/your-first-project' },
          ],
        },
        {
          label: 'Guides',
          items: [
            { label: 'The studio', slug: 'guides/the-studio' },
            { label: 'Nodes and edges', slug: 'guides/nodes-and-edges' },
            { label: 'Views', slug: 'guides/views' },
            { label: 'Exporting diagrams', slug: 'guides/exporting-diagrams' },
            { label: 'Cost estimates', slug: 'guides/cost-estimates' },
            { label: 'Simulation', slug: 'guides/simulation' },
            { label: 'Configuration', slug: 'guides/configuration' },
            { label: 'Providers', slug: 'guides/providers' },
          ],
        },
        {
          label: 'Reference',
          items: [
            { label: 'CLI', slug: 'reference/cli' },
            { label: 'Project files', slug: 'reference/project-files' },
            { label: 'togen.yml', slug: 'reference/togen-yml' },
            { label: 'Studio API', slug: 'reference/api' },
            {
              label: 'Node catalogue',
              items: [{ autogenerate: { directory: 'reference/catalogue' } }],
            },
          ],
        },
        {
          label: 'About',
          items: [
            { label: 'Why Togen', slug: 'about/why-togen' },
            { label: 'How it is built', slug: 'about/design' },
            { label: 'Roadmap', slug: 'about/roadmap' },
            { label: 'Licence', slug: 'about/licence' },
          ],
        },
      ],
    }),
  ],
});
