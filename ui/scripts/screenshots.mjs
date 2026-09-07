import { spawn } from 'node:child_process';
import { createServer } from 'node:net';
import { mkdir, mkdtemp, rm, cp, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const binary = resolve(root, 'bin/togen');
const out = resolve(root, process.env.TOGEN_SHOTS_OUT ?? 'docs/images');

const defaultViewport = { width: 1440, height: 900 };
const scale = 2;
const frameMargin = 80;

// Every shot names the project it opens, the theme it wears, and either a
// selector to clip to or nothing for the whole frame. `frame` asks for the
// viewport to be widened to fit every node before the shot is taken. `act`
// runs before the capture and gets the page. `viewport` overrides the
// browser window for shots whose graph does not suit the default aspect
// ratio; aws-full is wide and short, and the default viewport's height left
// most of studio-dark.png empty above and below the graph.
const shots = [
  {
    file: 'studio-dark.png',
    project: 'examples/aws-full',
    theme: 'dark',
    frame: true,
    viewport: { width: 1440, height: 680 },
  },
];

async function freePort() {
  return new Promise((ok, fail) => {
    const server = createServer();
    server.on('error', fail);
    server.listen(0, '127.0.0.1', () => {
      const { port } = server.address();
      server.close(() => ok(port));
    });
  });
}

async function studio(cwd) {
  const port = await freePort();
  const child = spawn(binary, ['studio', '--port', String(port), '--no-open'], {
    cwd,
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  let log = '';
  child.stdout.on('data', (d) => (log += d));
  child.stderr.on('data', (d) => (log += d));

  let stopped = false;
  const stop = () => {
    if (stopped) return;
    stopped = true;
    child.kill('SIGTERM');
  };

  const url = `http://127.0.0.1:${port}`;
  const deadline = Date.now() + 20_000;
  try {
    for (;;) {
      if (child.exitCode !== null) {
        throw new Error(`studio exited with ${child.exitCode} in ${cwd}:\n${log}`);
      }
      try {
        const answer = await fetch(`${url}/api/workspace`);
        if (answer.ok) break;
      } catch {
        // not listening yet
      }
      if (Date.now() > deadline) {
        throw new Error(`studio did not start in ${cwd} within 20s:\n${log}`);
      }
      await new Promise((r) => setTimeout(r, 100));
    }
  } catch (err) {
    // Nothing outside this function holds the child until stop is returned.
    stop();
    throw err;
  }

  return { url, stop };
}

async function pollUntil(check, timeoutMs, what) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await check();
    if (value) return value;
    if (Date.now() > deadline) {
      throw new Error(`timed out waiting for ${what}`);
    }
    await new Promise((r) => setTimeout(r, 100));
  }
}

// The example projects are read-only fixtures. The studio auto-lays-out with
// dagre on first load and persists that back to togen/layout.json, so it is
// driven against a scratch copy rather than the repository's own copy.
async function copyProject(project) {
  const dir = await mkdtemp(join(tmpdir(), 'togen-shots-'));
  await cp(resolve(root, project), dir, {
    recursive: true,
    filter: (source) => basename(source) !== 'infra',
  });
  return dir;
}

// Transitions and the canvas's entry animation make a capture depend on when it
// was taken. Nothing here is animated on purpose, so switching them off costs
// nothing and makes a regenerated image a real diff.
const still = `
  *, *::before, *::after {
    animation-duration: 0s !important;
    animation-delay: 0s !important;
    transition-duration: 0s !important;
    transition-delay: 0s !important;
  }
`;

async function settle(page, shot) {
  await page.addStyleTag({ content: still });
  await page.waitForSelector(`html[data-theme="${shot.theme}"]`);
  await page.waitForSelector(shot.empty ? '[aria-label="New sketch"]' : '.svelte-flow__node');
  if (!shot.empty) {
    await page.waitForFunction(() => {
      const nodes = document.querySelectorAll('.svelte-flow__node');
      return nodes.length > 0 && [...nodes].every((n) => n.getBoundingClientRect().width > 0);
    });
  }
}

// Frames the whole graph: reads the positions dagre just saved, measures the
// rendered node boxes, and writes back a viewport that fits them with a
// margin. The zoom is only ever pulled in, never past 1, so a small project
// is not magnified past its natural size.
async function frameProject(page, projectDir, shot) {
  const layoutPath = resolve(projectDir, 'togen/layout.json');
  const layout = await pollUntil(
    async () => {
      const raw = await readFile(layoutPath, 'utf8').catch(() => null);
      if (!raw) return null;
      const parsed = JSON.parse(raw);
      const nodes = parsed.views?.overview?.nodes ?? {};
      return Object.keys(nodes).length > 0 ? parsed : null;
    },
    20_000,
    `dagre positions in ${layoutPath}`,
  );

  const view = layout.views.overview;
  const zoomNow = view.viewport?.zoom || 1;
  const boxes = await page.evaluate(() =>
    Object.fromEntries(
      [...document.querySelectorAll('.svelte-flow__node')].map((el) => {
        const rect = el.getBoundingClientRect();
        return [el.getAttribute('data-id'), { width: rect.width, height: rect.height }];
      }),
    ),
  );
  // The canvas pane sits beside the rail and under the top bar, so it is
  // smaller than the browser viewport; fitting against the wrong rectangle
  // pushes content past the pane's own right and bottom edges. The minimap
  // and the zoom controls are then drawn on top of the pane's bottom corners,
  // so the fit also has to stay clear of whichever of the two reaches
  // highest, measured from the DOM rather than assumed, since either one can
  // change size.
  const { pane, usableHeight } = await page.evaluate(() => {
    const paneRect = document.querySelector('.svelte-flow').getBoundingClientRect();
    const overlayTops = ['.svelte-flow__minimap', '.svelte-flow__controls']
      .map((selector) => document.querySelector(selector)?.getBoundingClientRect())
      .filter(Boolean)
      .map((rect) => rect.top);
    const usableHeight = overlayTops.length
      ? Math.min(...overlayTops) - paneRect.top
      : paneRect.height;
    return {
      pane: { width: paneRect.width, height: paneRect.height },
      usableHeight,
    };
  });

  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const [id, position] of Object.entries(view.nodes)) {
    const box = boxes[id];
    if (!box) continue;
    const width = box.width / zoomNow;
    const height = box.height / zoomNow;
    minX = Math.min(minX, position.x);
    minY = Math.min(minY, position.y);
    maxX = Math.max(maxX, position.x + width);
    maxY = Math.max(maxY, position.y + height);
  }

  if (!Number.isFinite(minX) || !Number.isFinite(maxX)) {
    throw new Error(
      `no rendered node matched a layout id for ${shot.file}; ` +
        `${Object.keys(view.nodes).length} in ${layoutPath}, ${Object.keys(boxes).length} on the canvas`,
    );
  }

  const boundsWidth = maxX - minX + frameMargin * 2;
  const boundsHeight = maxY - minY + frameMargin * 2;
  const zoom = Math.min(pane.width / boundsWidth, usableHeight / boundsHeight, 1);
  const centerX = (minX + maxX) / 2;
  const centerY = (minY + maxY) / 2;

  // Centre within the usable band, not the whole pane, or the graph drifts
  // down towards the overlays that usableHeight was measured to avoid.
  view.viewport = {
    x: pane.width / 2 - centerX * zoom,
    y: usableHeight / 2 - centerY * zoom,
    zoom,
  };
  await writeFile(layoutPath, JSON.stringify(layout));
}

async function capture(browser, shot) {
  const projectDir = await copyProject(shot.project);
  try {
    const server = await studio(projectDir);
    try {
      const context = await browser.newContext({
        viewport: shot.viewport ?? defaultViewport,
        deviceScaleFactor: scale,
      });
      try {
        // The studio remembers the toggle in session storage and mirrors the
        // resolved scheme onto data-theme, which is what we wait on below.
        await context.addInitScript((theme) => {
          try {
            window.sessionStorage.setItem('togen.theme', theme);
          } catch {
            // a context that refuses storage falls back to the project setting
          }
        }, shot.theme);

        const page = await context.newPage();
        await page.goto(server.url, { waitUntil: 'networkidle' });
        await settle(page, shot);

        if (shot.frame) {
          await frameProject(page, projectDir, shot);
          await page.reload({ waitUntil: 'networkidle' });
          await settle(page, shot);
        }

        if (shot.act) await shot.act(page);

        const target = shot.clip ? page.locator(shot.clip) : page;
        await target.screenshot({ path: resolve(out, shot.file) });
        console.log(`  ${shot.file}`);
      } finally {
        await context.close();
      }
    } finally {
      server.stop();
    }
  } finally {
    await rm(projectDir, { recursive: true, force: true });
  }
}

async function main() {
  for (const shot of shots) {
    // Framing waits for dagre to persist node positions, which an empty
    // project never gets, so the pair would sit out the whole timeout.
    if (shot.frame && shot.empty) {
      throw new Error(
        `${shot.file} sets both frame and empty; an empty project has nothing to frame`,
      );
    }
  }

  await mkdir(out, { recursive: true });
  const browser = await chromium.launch({
    args: ['--disable-gpu', '--disable-gpu-compositing', '--force-color-profile=srgb'],
  });
  try {
    for (const shot of shots) {
      await capture(browser, shot);
    }
  } finally {
    await browser.close();
  }
  console.log(`${shots.length} screenshots written to ${out}`);
}

await main();
