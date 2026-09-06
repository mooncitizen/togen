import { expect, test } from 'vitest';

import type { Project } from './types.ts';
import { errorLine, validateProject } from './validate.ts';

function project(nodes: unknown[], edges: unknown[] = []): Project {
  return {
    version: 1,
    name: 'shop',
    provider: 'aws',
    region: 'eu-west-2',
    environment: 'dev',
    nodes,
    edges,
  } as Project;
}

function lines(nodes: unknown[], edges: unknown[] = []): string[] {
  return validateProject(project(nodes, edges)).map(errorLine);
}

const gateway = { id: 'gateway-1', type: 'gateway', name: 'api' };
const orders = { id: 'function-1', type: 'function', name: 'f' };
const db = { id: 'database-1', type: 'database', name: 'db' };

test('a sound project has nothing to say', () => {
  expect(
    lines(
      [gateway, orders, db],
      [
        { id: 'edge-1', from: 'gateway-1', to: 'function-1', relation: 'routes' },
        { id: 'edge-2', from: 'function-1', to: 'database-1', relation: 'reads' },
      ],
    ),
  ).toEqual([]);
});

// The wording is the CLI's, checked against `internal/ir` case by case.
test('the schema pass says what the CLI says', () => {
  expect(lines([{ id: 'service-1', type: 'service', name: 'web', properties: {} }])).toEqual([
    "nodes.0.properties (node service-1): missing property 'image'",
  ]);
  expect(
    lines([{ id: 'service-1', type: 'service', name: 'Bad Name', properties: { image: 'x' } }]),
  ).toEqual([
    "nodes.0.name (node service-1): 'Bad Name' does not match pattern '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'",
  ]);
  expect(lines([{ id: 'x-1', type: 'bananas', name: 'x' }])).toEqual([
    "nodes.0.type (node x-1): unknown node type 'bananas', use one of service, function, database, gateway, queue, bucket, cache",
  ]);
  expect(lines([{ ...db, properties: { storageGb: 5 } }])).toEqual([
    'nodes.0.properties.storageGb (node database-1): minimum: got 5, want 20',
  ]);
  expect(lines([{ ...db, properties: { storageGb: 'lots' } }])).toEqual([
    'nodes.0.properties.storageGb (node database-1): got string, want integer',
  ]);
  expect(lines([{ id: 'cache-1', type: 'cache', name: 'c', properties: { size: 'huge' } }])).toEqual(
    ["nodes.0.properties.size (node cache-1): value must be one of 'small', 'medium', 'large'"],
  );
  expect(
    lines(
      [gateway, orders],
      [
        {
          id: 'edge-1',
          from: 'gateway-1',
          to: 'function-1',
          relation: 'routes',
          properties: { path: 'users' },
        },
      ],
    ),
  ).toEqual(["edges.0.properties.path (edge edge-1): 'users' does not match pattern '^/'"]);
});

test('an env key the schema refuses is reported at the key', () => {
  expect(lines([{ ...orders, properties: { env: { 'api url': 'x' } } }])).toEqual([
    "nodes.0.properties.env.api url (node function-1): 'api url' does not match pattern '^[A-Z][A-Z0-9_]*$'",
  ]);
});

test('the semantic pass says what the CLI says', () => {
  expect(lines([gateway, { id: 'gateway-2', type: 'gateway', name: 'api2' }])).toEqual([
    'nodes (node gateway-2): a project can have at most one gateway',
  ]);
  expect(lines([gateway, { ...db, name: 'api' }])).toEqual([
    "nodes.1 (node database-1): duplicate node name 'api'",
  ]);
  expect(lines([orders, { ...orders, name: 'g' }])).toEqual([
    "nodes.1 (node function-1): duplicate node id 'function-1'",
  ]);
  expect(
    lines([gateway], [{ id: 'edge-1', from: 'gateway-1', to: 'ghost', relation: 'routes' }]),
  ).toEqual(["edges.0 (edge edge-1): edge refers to missing node 'ghost'"]);
  expect(
    lines([db, orders], [{ id: 'edge-1', from: 'database-1', to: 'function-1', relation: 'calls' }]),
  ).toEqual(["edges.0 (edge edge-1): a database cannot have a 'calls' edge to a function"]);
  expect(
    lines([orders], [{ id: 'edge-1', from: 'function-1', to: 'function-1', relation: 'calls' }]),
  ).toEqual(['edges.0 (edge edge-1): an edge cannot connect a node to itself']);
  expect(
    lines(
      [orders, db],
      [
        { id: 'edge-1', from: 'function-1', to: 'database-1', relation: 'reads' },
        { id: 'edge-2', from: 'function-1', to: 'database-1', relation: 'reads' },
      ],
    ),
  ).toEqual(["edges.1 (edge edge-2): duplicate 'reads' edge from 'f' to 'db'"]);
  expect(
    lines(
      [orders, db],
      [
        { id: 'edge-1', from: 'function-1', to: 'database-1', relation: 'reads' },
        { id: 'edge-1', from: 'function-1', to: 'database-1', relation: 'writes' },
      ],
    ),
  ).toEqual(["edges.1 (edge edge-1): duplicate edge id 'edge-1'"]);
  expect(lines([{ ...db, properties: { version: '9.6' } }])).toEqual([
    "nodes.0.properties.version (node database-1): database 'db': postgres version '9.6' is not supported on aws (use one of 17, 16, 15)",
  ]);
});

test('an error names the node or the edge it is about', () => {
  const errors = validateProject(
    project([gateway], [{ id: 'edge-1', from: 'gateway-1', to: 'ghost', relation: 'routes' }]),
  );
  expect(errors).toEqual([
    {
      path: 'edges.0',
      edgeId: 'edge-1',
      message: "edge refers to missing node 'ghost'",
    },
  ]);
});
