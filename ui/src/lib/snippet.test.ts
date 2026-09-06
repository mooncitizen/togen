import { expect, test } from 'vitest';

import { kindsSnippet, nodesSnippet } from './snippet.ts';
import type { Resolved } from './style.ts';

const orders: Resolved = {
  color: '#DD344C',
  icon: './icons/orders.svg',
  shape: 'hexagon',
  source: { color: 'node', icon: 'node', shape: 'kind' },
};

test('a snippet is the block to paste into togen.yml, hex quoted, icon as written', () => {
  expect(nodesSnippet({ name: 'orders-db' }, orders)).toBe(
    [
      'style:',
      '  nodes:',
      '    orders-db:',
      '      color: "#DD344C"',
      '      icon: ./icons/orders.svg',
      '      shape: hexagon',
    ].join('\n'),
  );
  expect(kindsSnippet('database', { ...orders, icon: 'aws/rds', shape: 'cylinder' })).toBe(
    [
      'style:',
      '  kinds:',
      '    database:',
      '      color: "#DD344C"',
      '      icon: aws/rds',
      '      shape: cylinder',
    ].join('\n'),
  );
});
