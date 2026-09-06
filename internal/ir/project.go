package ir

import "encoding/json"

type NodeType string

const (
	NodeService  NodeType = "service"
	NodeFunction NodeType = "function"
	NodeDatabase NodeType = "database"
	NodeGateway  NodeType = "gateway"
	NodeQueue    NodeType = "queue"
	NodeBucket   NodeType = "bucket"
	NodeCache    NodeType = "cache"
)

var NodeTypes = []NodeType{NodeService, NodeFunction, NodeDatabase, NodeGateway, NodeQueue, NodeBucket, NodeCache}

type Relation string

const (
	RelRoutes    Relation = "routes"
	RelCalls     Relation = "calls"
	RelReads     Relation = "reads"
	RelWrites    Relation = "writes"
	RelPublishes Relation = "publishes"
	RelConsumes  Relation = "consumes"
)

var Relations = []Relation{RelRoutes, RelCalls, RelReads, RelWrites, RelPublishes, RelConsumes}

type CloudProvider string

const (
	ProviderAWS   CloudProvider = "aws"
	ProviderGCP   CloudProvider = "gcp"
	ProviderAzure CloudProvider = "azure"
)

var Providers = []CloudProvider{ProviderAWS, ProviderGCP, ProviderAzure}

// Names, environments and region ids: lowercase words joined by single hyphens.
const KebabPattern = `^[a-z][a-z0-9]*(-[a-z0-9]+)*$`

type Size string

const (
	SizeSmall  Size = "small"
	SizeMedium Size = "medium"
	SizeLarge  Size = "large"
)

var Sizes = []Size{SizeSmall, SizeMedium, SizeLarge}

type Method string

const (
	MethodAny     Method = "ANY"
	MethodGet     Method = "GET"
	MethodPost    Method = "POST"
	MethodPut     Method = "PUT"
	MethodPatch   Method = "PATCH"
	MethodDelete  Method = "DELETE"
	MethodHead    Method = "HEAD"
	MethodOptions Method = "OPTIONS"
)

var Methods = []Method{MethodAny, MethodGet, MethodPost, MethodPut, MethodPatch, MethodDelete, MethodHead, MethodOptions}

type Runtime string

const (
	RuntimeNode   Runtime = "node"
	RuntimePython Runtime = "python"
	RuntimeGo     Runtime = "go"
)

var Runtimes = []Runtime{RuntimeNode, RuntimePython, RuntimeGo}

type Engine string

const (
	EnginePostgres Engine = "postgres"
	EngineMySQL    Engine = "mysql"
)

var EngineTypes = []Engine{EnginePostgres, EngineMySQL}

type Project struct {
	Version     int           `json:"version"`
	Name        string        `json:"name"`
	Provider    CloudProvider `json:"provider"`
	Region      string        `json:"region"`
	Environment string        `json:"environment"`
	Nodes       []Node        `json:"nodes"`
	Edges       []Edge        `json:"edges"`
}

type Node struct {
	ID         string          `json:"id"`
	Type       NodeType        `json:"type"`
	Name       string          `json:"name"`
	Properties json.RawMessage `json:"properties,omitempty"`
}

type Edge struct {
	ID         string         `json:"id"`
	From       string         `json:"from"`
	To         string         `json:"to"`
	Relation   Relation       `json:"relation"`
	Properties EdgeProperties `json:"properties,omitempty"`
}

type EdgeProperties struct {
	Path    string   `json:"path,omitempty"    jsonschema:"pattern=^/,default=/,description=Path the gateway routes to the target"`
	Methods []Method `json:"methods,omitempty" jsonschema:"description=HTTP methods the route accepts"`
}

type ServiceProps struct {
	Image       string            `json:"image"                 jsonschema:"description=Container image to run"`
	Port        int               `json:"port,omitempty"        jsonschema:"minimum=1,maximum=65535,default=8080,description=Port the container listens on"`
	Size        Size              `json:"size,omitempty"        jsonschema:"default=small,description=Memory and CPU tier"`
	MinReplicas int               `json:"minReplicas,omitempty" jsonschema:"minimum=0,default=1,description=Fewest tasks to keep running"`
	MaxReplicas int               `json:"maxReplicas,omitempty" jsonschema:"minimum=1,default=2,description=Most tasks to scale out to"`
	Public      bool              `json:"public,omitempty"      jsonschema:"default=false,description=Reachable from the internet"`
	Env         map[string]string `json:"env,omitempty"         jsonschema:"description=Environment variables to set on the container"`
}

type FunctionProps struct {
	Runtime        Runtime           `json:"runtime,omitempty"        jsonschema:"default=node,description=Language runtime the code needs"`
	Handler        string            `json:"handler,omitempty"        jsonschema:"minLength=1,default=index.handler,description=Entry point the runtime calls"`
	Size           Size              `json:"size,omitempty"           jsonschema:"default=small,description=Memory and CPU tier"`
	TimeoutSeconds int               `json:"timeoutSeconds,omitempty" jsonschema:"minimum=1,maximum=900,default=30,description=Seconds a single invocation may run for"`
	Env            map[string]string `json:"env,omitempty"            jsonschema:"description=Environment variables to set on the function"`
}

type DatabaseProps struct {
	Engine           Engine `json:"engine,omitempty"           jsonschema:"default=postgres,description=Database engine to run"`
	Version          string `json:"version,omitempty"          jsonschema:"minLength=1,description=Engine version. Defaults to the newest supported one"`
	Size             Size   `json:"size,omitempty"             jsonschema:"default=small,description=Memory and CPU tier"`
	StorageGB        int    `json:"storageGb,omitempty"        jsonschema:"minimum=20,default=20,description=Disk to allocate in gigabytes"`
	HighAvailability bool   `json:"highAvailability,omitempty" jsonschema:"default=false,description=Run a standby in a second availability zone"`
}

type GatewayProps struct{}

type QueueProps struct {
	FIFO          bool `json:"fifo,omitempty"          jsonschema:"default=false,description=Deliver messages in order exactly once"`
	DeadLetter    bool `json:"deadLetter,omitempty"    jsonschema:"default=true,description=Park messages that keep failing on a second queue"`
	RetentionDays int  `json:"retentionDays,omitempty" jsonschema:"minimum=1,maximum=14,default=4,description=Days an unread message is kept"`
}

type BucketProps struct {
	Versioning bool `json:"versioning,omitempty" jsonschema:"default=true,description=Keep previous versions of overwritten objects"`
	Public     bool `json:"public,omitempty"     jsonschema:"default=false,description=Readable from the internet"`
}

type CacheProps struct {
	Size Size `json:"size,omitempty" jsonschema:"default=small,description=Memory and CPU tier"`
}
