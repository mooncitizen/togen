import type { Cost, Node } from './types.ts';

export const dash = '–';

// The same shapes as the CLI table: money to the cent, unit prices to four
// places, quantities as short as they can be.
export function money(amount: number): string {
  return amount.toFixed(2);
}

export function unitPrice(price: number): string {
  return price.toFixed(4);
}

export function quantity(value: number): string {
  return String(value);
}

// Items are named after nodes, the implicit ones after nothing on the canvas,
// so the kind tells a node called network from the VPC.
export function belongs(entry: { name: string; kind?: string }, node: Node): boolean {
  return entry.name === node.name && entry.kind === node.type;
}

export function subtotalOf(cost: Cost | null, node: Node): number | null {
  return cost?.items.find((item) => belongs(item, node))?.subtotal ?? null;
}
