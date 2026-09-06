import type { Resolved } from './style.ts';
import type { NodeType } from './types.ts';

export function nodesSnippet(node: { name: string }, style: Resolved): string {
  return block('nodes', node.name, style);
}

export function kindsSnippet(type: NodeType, style: Resolved): string {
  return block('kinds', type, style);
}

// The YAML as it goes under style in togen.yml: the hex is quoted because a
// bare # starts a comment, the icon id or path is a plain scalar as written.
function block(section: 'nodes' | 'kinds', key: string, style: Resolved): string {
  return [
    'style:',
    `  ${section}:`,
    `    ${key}:`,
    `      color: "${style.color}"`,
    `      icon: ${style.icon}`,
    `      shape: ${style.shape}`,
  ].join('\n');
}
