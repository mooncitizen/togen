import { cp, mkdir, readdir, readFile, rm, writeFile } from 'node:fs/promises';
import { basename, dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const site = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const root = resolve(site, '..');

const catalogueFrom = resolve(root, 'docs/catalogue');
const catalogueTo = resolve(site, 'src/content/docs/reference/catalogue');
const imagesFrom = resolve(root, 'docs/images');
const imagesTo = resolve(site, 'public/images');

const repo = 'https://github.com/mooncitizen/togen/blob/main/docs/catalogue';

function title(markdown, file) {
  const heading = markdown.split('\n').find((line) => line.startsWith('# '));
  if (!heading) {
    throw new Error(`${file} has no level one heading, so it has no title`);
  }
  return heading.slice(2).trim();
}

// Starlight resolves links against the page's own URL, and the catalogue's
// links are relative to docs/catalogue, so a bare styles.md would 404. The
// site is built with trailingSlash: 'always', so the rewritten link needs
// the trailing slash too, or it 404s again once deployed.
function relink(markdown) {
  return markdown.replace(/\]\((?!https?:|\/|#)([\w.-]+)\.md(#[\w-]*)?\)/g, '](./$1/$2)');
}

async function catalogue() {
  await rm(catalogueTo, { recursive: true, force: true });
  await mkdir(catalogueTo, { recursive: true });

  const files = (await readdir(catalogueFrom)).filter((f) => f.endsWith('.md'));
  if (files.length === 0) {
    throw new Error(`no catalogue files found in ${catalogueFrom}`);
  }

  for (const file of files) {
    const markdown = await readFile(resolve(catalogueFrom, file), 'utf8');
    const name = basename(file, '.md');
    const heading = title(markdown, file);
    const body = relink(markdown.replace(/^# .*\n/, ''));
    const frontmatter = [
      '---',
      `title: ${JSON.stringify(heading)}`,
      `editUrl: ${repo}/${file}`,
      'description: >-',
      '  Maintained beside the resolvers in docs/catalogue and published here',
      '  unchanged.',
      '---',
      '',
    ].join('\n');
    const out = name === 'README' ? 'index.md' : `${name}.md`;
    await writeFile(resolve(catalogueTo, out), frontmatter + body);
  }
  console.log(`catalogue: ${files.length} pages`);
}

async function images() {
  await rm(imagesTo, { recursive: true, force: true });
  await cp(imagesFrom, imagesTo, { recursive: true });
  const count = (await readdir(imagesTo)).length;
  if (count === 0) {
    throw new Error(`no screenshots found in ${imagesFrom}: run 'just screenshots'`);
  }
  console.log(`images: ${count} files`);
}

await catalogue();
await images();
