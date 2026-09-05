// The built-in look of a diagram per provider, published as schema/styles.json (ADR 0007).
package style

import (
	"slices"

	"github.com/mooncitizen/togen/internal/ir"
)

type Shape string

const (
	ShapeCard     Shape = "card"
	ShapeCylinder Shape = "cylinder"
	ShapeHexagon  Shape = "hexagon"
	ShapeCircle   Shape = "circle"
)

var Shapes = []Shape{ShapeCard, ShapeCylinder, ShapeHexagon, ShapeCircle}

// A box the canvas draws around nodes: the implicit network, and on Azure the resource group.
type Boundary struct {
	Kind  string `json:"kind"`
	Color string `json:"color"`
}

type Kind struct {
	Color    string `json:"color"`
	Icon     string `json:"icon"`
	Shape    Shape  `json:"shape"`
	Resource string `json:"resource"`
}

type Scheme struct {
	Accent        string               `json:"accent"`
	Network       Boundary             `json:"network"`
	ResourceGroup *Boundary            `json:"resourceGroup,omitempty"`
	Kinds         map[ir.NodeType]Kind `json:"kinds"`
}

// The colours are the category colours each vendor uses in its architecture icon
// set, and the icon ids name an icon in that set. Stores are cylinders, the rest cards.
var Schemes = map[ir.CloudProvider]Scheme{
	ir.ProviderAWS: {
		Accent:  "#ED7100",
		Network: Boundary{Kind: "VPC", Color: "#8C4FFF"},
		Kinds: map[ir.NodeType]Kind{
			ir.NodeGateway:  {Color: "#8C4FFF", Icon: "aws/api-gateway", Shape: ShapeCard, Resource: "API Gateway"},
			ir.NodeFunction: {Color: "#ED7100", Icon: "aws/lambda", Shape: ShapeCard, Resource: "Lambda"},
			ir.NodeService:  {Color: "#ED7100", Icon: "aws/ecs", Shape: ShapeCard, Resource: "ECS on Fargate"},
			ir.NodeDatabase: {Color: "#C925D1", Icon: "aws/rds", Shape: ShapeCylinder, Resource: "RDS"},
			ir.NodeQueue:    {Color: "#E7157B", Icon: "aws/sqs", Shape: ShapeCard, Resource: "SQS"},
			ir.NodeBucket:   {Color: "#7AA116", Icon: "aws/s3", Shape: ShapeCard, Resource: "S3"},
			ir.NodeCache:    {Color: "#C925D1", Icon: "aws/elasticache", Shape: ShapeCylinder, Resource: "ElastiCache"},
		},
	},
	ir.ProviderGCP: {
		Accent:  "#4285F4",
		Network: Boundary{Kind: "VPC network", Color: "#4285F4"},
		Kinds: map[ir.NodeType]Kind{
			ir.NodeGateway:  {Color: "#4285F4", Icon: "gcp/api-gateway", Shape: ShapeCard, Resource: "API Gateway"},
			ir.NodeFunction: {Color: "#34A853", Icon: "gcp/cloud-functions", Shape: ShapeCard, Resource: "Cloud Functions"},
			ir.NodeService:  {Color: "#34A853", Icon: "gcp/cloud-run", Shape: ShapeCard, Resource: "Cloud Run"},
			ir.NodeDatabase: {Color: "#4285F4", Icon: "gcp/cloud-sql", Shape: ShapeCylinder, Resource: "Cloud SQL"},
			ir.NodeQueue:    {Color: "#F9AB00", Icon: "gcp/pubsub", Shape: ShapeCard, Resource: "Pub/Sub"},
			ir.NodeBucket:   {Color: "#EA4335", Icon: "gcp/cloud-storage", Shape: ShapeCard, Resource: "Cloud Storage"},
			ir.NodeCache:    {Color: "#4285F4", Icon: "gcp/memorystore", Shape: ShapeCylinder, Resource: "Memorystore"},
		},
	},
	ir.ProviderAzure: {
		Accent:        "#0078D4",
		Network:       Boundary{Kind: "VNet", Color: "#0078D4"},
		ResourceGroup: &Boundary{Kind: "Resource group", Color: "#0078D4"},
		Kinds: map[ir.NodeType]Kind{
			ir.NodeGateway:  {Color: "#0078D4", Icon: "azure/api-management", Shape: ShapeCard, Resource: "API Management"},
			ir.NodeFunction: {Color: "#0078D4", Icon: "azure/functions", Shape: ShapeCard, Resource: "Functions"},
			ir.NodeService:  {Color: "#0078D4", Icon: "azure/container-apps", Shape: ShapeCard, Resource: "Container Apps"},
			ir.NodeDatabase: {Color: "#7B3FE4", Icon: "azure/database-postgresql", Shape: ShapeCylinder, Resource: "Database for PostgreSQL"},
			ir.NodeQueue:    {Color: "#FFB900", Icon: "azure/service-bus", Shape: ShapeCard, Resource: "Service Bus"},
			ir.NodeBucket:   {Color: "#6BB700", Icon: "azure/storage-blob", Shape: ShapeCard, Resource: "Blob Storage"},
			ir.NodeCache:    {Color: "#7B3FE4", Icon: "azure/cache-redis", Shape: ShapeCylinder, Resource: "Cache for Redis"},
		},
	},
}

func BundledIcons() []string {
	var ids []string
	for _, scheme := range Schemes {
		for _, kind := range scheme.Kinds {
			ids = append(ids, kind.Icon)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}
