import { Ajv2020, type ErrorObject } from 'ajv/dist/2020';

import schema from '../../../schema/project.schema.json';

import { engineVersions } from './form.ts';
import { legalRelations } from './relations.ts';
import type { Node, NodeType, Project, ValidationError } from './types.ts';

const check = new Ajv2020({ allErrors: true, strict: false, verbose: true }).compile(schema);
const branches = schema.properties.nodes.items.oneOf.map(
  (variant) => variant.properties.type.const as NodeType,
);
const inBranch = /^#\/properties\/nodes\/items\/oneOf\/(\d+)\//;

// The same two passes as the CLI, in the same order and the same words, so a
// project the canvas calls sound is one the studio will save (ADR 0003).
export function validateProject(project: Project): ValidationError[] {
  const shape = schemaErrors(project);
  return shape.length > 0 ? shape : semanticErrors(project);
}

export function errorLine(error: ValidationError): string {
  const path = error.path === '' ? 'project' : error.path;
  if (error.nodeId !== undefined && error.nodeId !== '') {
    return `${path} (node ${error.nodeId}): ${error.message}`;
  }
  if (error.edgeId !== undefined && error.edgeId !== '') {
    return `${path} (edge ${error.edgeId}): ${error.message}`;
  }
  return `${path}: ${error.message}`;
}

function schemaErrors(project: Project): ValidationError[] {
  if (check(project)) {
    return [];
  }
  const out: ValidationError[] = [];
  for (const error of check.errors ?? []) {
    const at = segments(error.instancePath);
    if (error.keyword === 'oneOf' && at.length === 2 && at[0] === 'nodes') {
      const mistyped = mistypedNode(project, at);
      if (mistyped !== undefined) {
        out.push(mistyped);
      }
      continue;
    }
    // Six of the seven branches failed on the type const alone, and only the branch
    // the user meant has anything to say about the node.
    const branch = inBranch.exec(error.schemaPath)?.[1];
    if (branch !== undefined && Number(branch) !== branchFor(project, at)) {
      continue;
    }
    if (error.keyword === 'propertyNames') {
      continue;
    }
    out.push(entry(project, at, message(error)));
  }
  return out;
}

// A node is a oneOf over the seven types, discriminated by a const on one property,
// so the branch the user meant has already said what is wrong with the node.
function mistypedNode(project: Project, at: string[]): ValidationError | undefined {
  const node = project.nodes[Number(at[1])] as Node | undefined;
  const given = String(node?.type ?? '');
  if ((branches as string[]).includes(given)) {
    return undefined;
  }
  if (given === '') {
    return entry(project, at, "'oneOf' failed, none matched");
  }
  return {
    path: `${at.join('.')}.type`,
    nodeId: node?.id ?? '',
    message: `unknown node type '${given}', use one of ${branches.join(', ')}`,
  };
}

function branchFor(project: Project, at: string[]): number {
  const type = project.nodes[Number(at[1])]?.type;
  return branches.findIndex((candidate) => candidate === type);
}

function entry(project: Project, at: string[], text: string): ValidationError {
  const error: ValidationError = { path: at.join('.'), message: text };
  if (at.length < 2) {
    return error;
  }
  const index = Number(at[1]);
  if (at[0] === 'nodes') {
    error.nodeId = project.nodes[index]?.id ?? '';
  }
  if (at[0] === 'edges') {
    error.edgeId = project.edges[index]?.id ?? '';
  }
  return error;
}

function segments(instancePath: string): string[] {
  return instancePath === '' ? [] : instancePath.slice(1).split('/');
}

function message(error: ErrorObject): string {
  const params = error.params as Record<string, unknown>;
  const data: unknown = error.data;
  switch (error.keyword) {
    case 'required':
      return `missing property ${quote(String(params.missingProperty))}`;
    case 'additionalProperties':
      return `additional properties ${quote(String(params.additionalProperty))} not allowed`;
    case 'type':
      return `got ${jsonType(data)}, want ${String(params.type)}`;
    case 'enum':
      return oneOfMessage(params.allowedValues);
    case 'const':
      return `value must be ${display(params.allowedValue)}`;
    case 'pattern':
      return `${quote(String(data))} does not match pattern ${quote(String(params.pattern))}`;
    case 'minLength':
      return `minLength: got ${String(data).length}, want ${String(params.limit)}`;
    case 'maxLength':
      return `maxLength: got ${String(data).length}, want ${String(params.limit)}`;
    case 'minimum':
      return `minimum: got ${String(data)}, want ${String(params.limit)}`;
    case 'maximum':
      return `maximum: got ${String(data)}, want ${String(params.limit)}`;
    case 'minItems':
      return `minItems: got ${count(data)}, want ${String(params.limit)}`;
    case 'maxItems':
      return `maxItems: got ${count(data)}, want ${String(params.limit)}`;
    default:
      return error.message ?? 'validation failed';
  }
}

function oneOfMessage(allowed: unknown): string {
  const values = Array.isArray(allowed) ? (allowed as unknown[]) : [];
  if (values.length === 1) {
    return `value must be ${display(values[0])}`;
  }
  return `value must be one of ${values.map(display).join(', ')}`;
}

function count(data: unknown): number {
  return Array.isArray(data) ? data.length : 0;
}

function jsonType(data: unknown): string {
  if (data === null) {
    return 'null';
  }
  if (Array.isArray(data)) {
    return 'array';
  }
  return typeof data;
}

function display(value: unknown): string {
  return typeof value === 'string' ? quote(value) : String(value);
}

function quote(value: string): string {
  return `'${value.replaceAll("'", "\\'")}'`;
}

function semanticErrors(project: Project): ValidationError[] {
  const out: ValidationError[] = [];
  const byId = new Map<string, Node>();
  const names = new Set<string>();

  project.nodes.forEach((node, index) => {
    const path = `nodes.${index}`;
    if (byId.has(node.id)) {
      out.push({ path, nodeId: node.id, message: `duplicate node id '${node.id}'` });
    }
    if (names.has(node.name)) {
      out.push({ path, nodeId: node.id, message: `duplicate node name '${node.name}'` });
    }
    const unsupported = engineVersionError(project, node, index);
    if (unsupported !== undefined) {
      out.push(unsupported);
    }
    byId.set(node.id, node);
    names.add(node.name);
  });

  let gateways = 0;
  for (const node of project.nodes) {
    if (node.type !== 'gateway') {
      continue;
    }
    gateways += 1;
    if (gateways > 1) {
      out.push({ path: 'nodes', nodeId: node.id, message: 'a project can have at most one gateway' });
    }
  }

  const seen = new Set<string>();
  const ids = new Set<string>();
  project.edges.forEach((edge, index) => {
    const path = `edges.${index}`;
    if (ids.has(edge.id)) {
      out.push({ path, edgeId: edge.id, message: `duplicate edge id '${edge.id}'` });
    }
    ids.add(edge.id);

    const from = byId.get(edge.from);
    const to = byId.get(edge.to);
    if (from === undefined) {
      out.push({ path, edgeId: edge.id, message: `edge refers to missing node '${edge.from}'` });
    }
    if (to === undefined) {
      out.push({ path, edgeId: edge.id, message: `edge refers to missing node '${edge.to}'` });
    }
    if (from === undefined || to === undefined) {
      return;
    }
    if (from.id === to.id) {
      out.push({ path, edgeId: edge.id, message: 'an edge cannot connect a node to itself' });
      return;
    }
    if (!legalRelations(from.type, to.type).includes(edge.relation)) {
      out.push({
        path,
        edgeId: edge.id,
        message: `a ${from.type} cannot have a '${edge.relation}' edge to a ${to.type}`,
      });
    }
    const key = `${edge.from}|${edge.to}|${edge.relation}`;
    if (seen.has(key)) {
      out.push({
        path,
        edgeId: edge.id,
        message: `duplicate '${edge.relation}' edge from '${from.name}' to '${to.name}'`,
      });
    }
    seen.add(key);
  });

  return out;
}

// A provider with no engine table has no opinion yet, so nothing is checked for it.
function engineVersionError(
  project: Project,
  node: Node,
  index: number,
): ValidationError | undefined {
  if (node.type !== 'database') {
    return undefined;
  }
  const version = node.properties?.version;
  if (typeof version !== 'string' || version === '') {
    return undefined;
  }
  const held = node.properties?.engine;
  const engine = typeof held === 'string' && held !== '' ? held : 'postgres';
  const versions = engineVersions(project.provider, engine);
  if (versions === undefined || versions.includes(version)) {
    return undefined;
  }
  return {
    path: `nodes.${index}.properties.version`,
    nodeId: node.id,
    message: `database '${node.name}': ${engine} version '${version}' is not supported on ${project.provider} (use one of ${versions.join(', ')})`,
  };
}
