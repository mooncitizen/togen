export type NodeType = 'service' | 'function' | 'database' | 'gateway' | 'queue' | 'bucket' | 'cache';

export type Relation = 'routes' | 'calls' | 'reads' | 'writes' | 'publishes' | 'consumes';

export type Provider = 'aws' | 'gcp' | 'azure';

export type Node = {
  id: string;
  type: NodeType;
  name: string;
  properties?: Record<string, unknown>;
};

export type Edge = {
  id: string;
  from: string;
  to: string;
  relation: Relation;
  properties?: Record<string, unknown>;
};

export type Project = {
  version: number;
  name: string;
  provider: Provider;
  region: string;
  environment: string;
  nodes: Node[];
  edges: Edge[];
};

export type Position = { x: number; y: number };

export type Viewport = { x: number; y: number; zoom: number };

export type Layout = {
  version: number;
  nodes: Record<string, Position>;
  viewport: Viewport;
};

export type ValidationError = {
  path: string;
  nodeId?: string;
  edgeId?: string;
  message: string;
};
