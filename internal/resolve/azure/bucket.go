package azure

import (
	"fmt"

	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

// One storage account per bucket, holding one container. Sharing an account across buckets
// would make every role assignment reach every bucket, since the roles are granted on the
// account.
func resolveBucket(ctx *resolve.Context, node ir.Node) *resolve.Handle {
	p := resolve.Props[ir.BucketProps](ctx, node)
	local := ctx.Local(node.Name)
	group := ensureGroup(ctx)

	account := ctx.Add(ir.Resource{
		Type:        "azurerm_storage_account",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: group.Located(storageAccountName(ctx, node.Name),
			ir.A("account_tier", ir.Str("Standard")),
			ir.A("account_replication_type", ir.Str("LRS")),
			ir.A("min_tls_version", ir.Str("TLS1_2")),
			ir.A("allow_nested_items_to_be_public", ir.Bool(p.Public)),
			ir.A("blob_properties", ir.B(ir.Attrs{
				ir.A("versioning_enabled", ir.Bool(p.Versioning)),
			})),
		),
	})
	accountID := ir.ID{Type: account.Type, Name: account.Name}

	access := "private"
	if p.Public {
		access = "blob"
	}
	container := ctx.Add(ir.Resource{
		Type:        "azurerm_storage_container",
		Name:        local,
		SourceNode:  node.ID,
		SourceLabel: node.Name,
		Args: ir.Attrs{
			ir.A("name", ir.Str(ctx.Named(node.Name))),
			ir.A("storage_account_id", ir.R(accountID, ir.Field("id"))),
			ir.A("container_access_type", ir.Str(access)),
		},
	})
	containerID := ir.ID{Type: container.Type, Name: container.Name}

	exports := resolve.BucketExports{
		Account:   ir.R(accountID, ir.Field("name")),
		Container: ir.R(containerID, ir.Field("name")),
		Scope:     ir.R(accountID, ir.Field("id")),
	}
	ctx.AddOutput(ir.Output{
		Name:        local + "_account",
		Description: fmt.Sprintf("Storage account holding the %s container", node.Name),
		Value:       exports.Account,
	})
	ctx.AddOutput(ir.Output{
		Name:        local + "_container",
		Description: fmt.Sprintf("Name of the %s container", node.Name),
		Value:       exports.Container,
	})

	return &resolve.Handle{
		Node:    node,
		Primary: containerID,
		Exports: exports,
	}
}
