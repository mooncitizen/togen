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

export type ViewLayout = { nodes: Record<string, Position>; viewport: Viewport };

export type Layout = {
  version: number;
  views: Record<string, ViewLayout>;
};

export type ViewNodes = '*' | string[];

export type View = { id: string; name: string; nodes: ViewNodes };

export type Views = { version: number; views: View[] };

export type Generated = {
  dir: string;
  files: string[];
};

export type ThemeSetting = 'dark' | 'light' | 'system';

export type Shape = 'card' | 'cylinder' | 'hexagon' | 'circle';

export type NodeStyle = {
  color?: string;
  icon?: string;
  shape?: Shape;
};

export type Style = {
  theme?: ThemeSetting;
  kinds?: Partial<Record<NodeType, NodeStyle>>;
  nodes?: Record<string, NodeStyle>;
};

export type Config = {
  version: number;
  targets: string[];
  outDir: string;
  style?: Style;
  deprecated?: string;
};

export type ValidationError = {
  path: string;
  nodeId?: string;
  edgeId?: string;
  message: string;
};
