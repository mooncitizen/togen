import { getFontEmbedCSS, toSvg } from 'html-to-image';

import type { Rect } from './boundary.ts';

export type Format = 'png' | 'jpeg';

export type Scale = 1 | 2 | 3;

export type Background = 'theme' | 'transparent' | 'white';

export type Options = { format: Format; scale: Scale; background: Background; title: boolean };

export type Size = { width: number; height: number };

// The picture at 1x: the frame with a margin round it and, with the title on,
// a band above the content for it. `top` is where the content starts.
export type Page = Size & { top: number };

export type Subject = { name: string; nodes: Set<string> };

export const defaults: Options = { format: 'png', scale: 2, background: 'theme', title: true };

export const margin = 32;
export const titleBand = 32;
export const titleAt = { x: 24, y: 20 };

const titleFont = '500 13px "IBM Plex Mono", ui-monospace, Menlo, monospace';
const jpegQuality = 0.92;
const revokeDelay = 1000;
const flowClass = 'svelte-flow';
const viewportClass = 'svelte-flow__viewport';
const nodeClass = 'svelte-flow__node';
const editing = [
  'svelte-flow__handle',
  'svelte-flow__selection-wrapper',
  'svelte-flow__connectionline',
];

export function pageFor(frame: Rect, options: Options): Page {
  const top = margin + (options.title ? titleBand : 0);
  return {
    width: Math.ceil(frame.width) + margin * 2,
    height: Math.ceil(frame.height) + top + margin,
    top,
  };
}

export function sizeFor(frame: Rect, options: Options): Size {
  const page = pageFor(frame, options);
  return { width: page.width * options.scale, height: page.height * options.scale };
}

export function describeSize(size: Size): string {
  return `${size.width} × ${size.height} px`;
}

export function fileName(project: string, view: string, format: Format): string {
  return `${project}-${view}.${format === 'png' ? 'png' : 'jpg'}`;
}

// Transparent is a PNG thing; a JPEG asked for it gets the theme's ground.
export function settled(options: Options): Options {
  if (options.format === 'jpeg' && options.background === 'transparent') {
    return { ...options, background: 'theme' };
  }
  return options;
}

// Copies the flow's viewport as the canvas draws it, moved so the frame fills
// the page, then draws that on a bitmap of the page at the chosen scale with
// the ground and the title.
export async function renderView(
  pane: HTMLElement | undefined,
  frame: Rect,
  subject: Subject,
  asked: Options,
): Promise<Blob> {
  const options = settled(asked);
  const root = pane?.querySelector<HTMLElement>(`.${flowClass}`);
  const viewport = root?.querySelector<HTMLElement>(`.${viewportClass}`);
  if (root == null || viewport == null) {
    throw new Error('the canvas is not drawn');
  }
  const page = pageFor(frame, options);
  const fonts = await getFontEmbedCSS(viewport);
  await document.fonts.load(titleFont);

  // The exporting state sits on the live canvas only while its styles are
  // copied. With the fonts already in hand nothing below awaits a paint, so
  // the screen never shows the canvas without its selection ring or in the
  // light palette a white export borrows.
  root.dataset.exporting = options.background;
  let ground: string;
  let ink: string;
  let svg: string;
  try {
    const palette = getComputedStyle(root);
    ground = options.background === 'white' ? '#ffffff' : variable(palette, '--togen-canvas');
    ink = variable(palette, '--togen-text');
    svg = await toSvg(viewport, {
      width: page.width,
      height: page.height,
      fontEmbedCSS: `${fonts}\n${edgeRules(palette)}`,
      style: {
        transform: `translate(${margin - frame.x}px, ${page.top - frame.y}px) scale(1)`,
        transformOrigin: '0 0',
      },
      filter: (node) => keep(node, subject.nodes),
    });
  } finally {
    delete root.dataset.exporting;
  }

  const image = await load(svg);
  const canvas = document.createElement('canvas');
  canvas.width = page.width * options.scale;
  canvas.height = page.height * options.scale;
  const context = canvas.getContext('2d');
  if (context === null) {
    throw new Error('the browser has no canvas to draw on');
  }
  if (options.background !== 'transparent') {
    context.fillStyle = ground;
    context.fillRect(0, 0, canvas.width, canvas.height);
  }
  context.drawImage(image, 0, 0, canvas.width, canvas.height);
  if (options.title) {
    context.scale(options.scale, options.scale);
    context.font = titleFont;
    context.fillStyle = ink;
    context.textBaseline = 'top';
    context.fillText(subject.name, titleAt.x, titleAt.y);
  }
  return encode(canvas, options.format);
}

export function save(blob: Blob, name: string): void {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = name;
  link.click();
  // Firefox wants the URL alive until the download has begun.
  setTimeout(() => URL.revokeObjectURL(url), revokeDelay);
}

// The handles, the box selection and a connection being dragged are the
// editor's; a card outside the view is on the canvas only while the view
// editor is open; the particle overlay is a live animation with no still
// frame worth keeping, so a picture shows the rate labels only.
function keep(node: Node, nodes: Set<string>): boolean {
  if (!(node instanceof Element)) {
    return true;
  }
  if (editing.some((name) => node.classList.contains(name))) {
    return false;
  }
  if (node.hasAttribute('data-particle') || node.hasAttribute('data-particles')) {
    return false;
  }
  return !node.classList.contains(nodeClass) || nodes.has(node.getAttribute('data-id') ?? '');
}

function variable(palette: CSSStyleDeclaration, name: string): string {
  return palette.getPropertyValue(name).trim();
}

// html-to-image copies an svg subtree as it stands, without what the
// stylesheet gives it, so the edge paths get their rules in the picture with
// the fonts, with the palette's values where the clone cannot see the
// canvas's variables.
function edgeRules(palette: CSSStyleDeclaration): string {
  const stroke = variable(palette, '--xy-edge-stroke');
  const width = variable(palette, '--xy-edge-stroke-width');
  const strong = variable(palette, '--togen-edge-strong');
  const canvas = variable(palette, '--togen-canvas');
  const errorStroke = variable(palette, '--togen-edge-error-stroke');
  return [
    `.svelte-flow__edge-path { fill: none; stroke: ${stroke}; stroke-width: ${width}; }`,
    `.togen-edge-casing { fill: none; stroke: ${canvas}; stroke-linecap: round; }`,
    `.togen-edge-head { fill: ${stroke}; stroke: ${stroke}; stroke-width: 0.6; stroke-linejoin: round; }`,
    `.togen-edge-dashed .svelte-flow__edge-path, .togen-edge-dotted .svelte-flow__edge-path { stroke: ${strong}; }`,
    `.togen-edge-dashed .togen-edge-head, .togen-edge-dotted .togen-edge-head { fill: ${strong}; stroke: ${strong}; }`,
    `.svelte-flow__edge.togen-edge-error .svelte-flow__edge-path { stroke: ${errorStroke}; }`,
    `.svelte-flow__edge.togen-edge-error .togen-edge-head { fill: ${errorStroke}; stroke: ${errorStroke}; }`,
  ].join('\n');
}

async function load(url: string): Promise<HTMLImageElement> {
  const image = new Image();
  await new Promise<void>((resolve, reject) => {
    image.onload = () => resolve();
    image.onerror = () => reject(new Error('the browser could not draw the picture'));
    image.src = url;
  });
  await image.decode();
  return image;
}

function encode(canvas: HTMLCanvasElement, format: Format): Promise<Blob> {
  const type = format === 'png' ? 'image/png' : 'image/jpeg';
  return new Promise((resolve, reject) => {
    canvas.toBlob(
      (blob) => {
        if (blob === null) {
          reject(new Error('the browser could not encode the image'));
          return;
        }
        resolve(blob);
      },
      type,
      jpegQuality,
    );
  });
}
