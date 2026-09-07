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

export type Workspace = {
  dir: string;
  name: string;
};

export type Example = {
  id: string;
  description: string;
};

export type Sketch = {
  name: string;
  provider: Provider;
  region: string;
  environment: string;
};

export type InitRequest = Sketch | { example: string };

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

export type Source = {
  id: string;
  name: string;
  target: string;
  rate: string;
  bytesPerRequest?: number;
};

export type Burst = {
  id: string;
  name: string;
  source: string;
  multiplier: number;
  minutes: number;
  timesPerMonth: number;
};

export type Simulation = {
  version: number;
  sources: Source[];
  bursts?: Burst[];
  edges?: Record<string, number>;
};

// stillSettling is set when the sweep budget ran out before the loop reached a fixed
// point: the rates are the last iterate, not the settled answer, and read low.
export type SimResult = {
  nodes: Record<string, number>;
  edges: Record<string, number>;
  stillSettling?: boolean;
};

export type ValidationError = {
  path: string;
  nodeId?: string;
  edgeId?: string;
  message: string;
};

export type CostLine = {
  label: string;
  quantity: number;
  unit: string;
  unitPrice: number;
  amount: number;
  sku: string;
  note?: string;
};

export type CostItem = {
  name: string;
  kind: string;
  summary?: string;
  note?: string;
  lines: CostLine[];
  subtotal: number;
};

export type Omission = {
  name: string;
  kind?: string;
  reason: string;
};

export type Cost = {
  provider: Provider;
  region: string;
  currency: string;
  items: CostItem[];
  notPriced: Omission[];
  total: number;
  snapshotDate?: string;
  note: string;
  warning?: string;
};
