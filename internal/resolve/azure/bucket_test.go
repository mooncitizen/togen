package azure

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mooncitizen/togen/internal/emit/hcl"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve"
)

// Properties are given raw because versioning defaults to true and omitempty would drop a
// false written through BucketProps.
func bucketNode(id, name, properties string) ir.Node {
	return ir.Node{ID: id, Type: ir.NodeBucket, Name: name, Properties: json.RawMessage(properties)}
}

func setupBucket(t *testing.T, properties string) (*resolve.Context, *resolve.Handle) {
	t.Helper()
	ctx := newContext(t, []ir.Node{bucketNode("n7", "uploads", properties)})
	return ctx, resolveBucket(ctx, ctx.Project.Nodes[0])
}

var (
	accountID   = ir.ID{Type: "azurerm_storage_account", Name: "uploads"}
	containerID = ir.ID{Type: "azurerm_storage_container", Name: "uploads"}
)

func accountArgs(name string, public, versioning bool) ir.Attrs {
	return located(name,
		ir.A("account_tier", ir.Str("Standard")),
		ir.A("account_replication_type", ir.Str("LRS")),
		ir.A("min_tls_version", ir.Str("TLS1_2")),
		ir.A("allow_nested_items_to_be_public", ir.Bool(public)),
		ir.A("blob_properties", ir.B(ir.Attrs{ir.A("versioning_enabled", ir.Bool(versioning))})),
	)
}

func containerArgs(access string) ir.Attrs {
	return ir.Attrs{
		ir.A("name", ir.Str("shop-dev-uploads")),
		ir.A("storage_account_id", ir.R(accountID, ir.Field("id"))),
		ir.A("container_access_type", ir.Str(access)),
	}
}

func TestBucketEmitsAPrivateVersionedContainerInItsOwnAccount(t *testing.T) {
	ctx, handle := setupBucket(t, "{}")
	if len(ctx.Errors) != 0 {
		t.Fatalf("errors = %v", ctx.Errors)
	}

	account := firstOfType(t, ctx, "azurerm_storage_account")
	if diff := cmp.Diff(accountArgs("shopdevuploads", false, true), account.Args); diff != "" {
		t.Errorf("account args (-want +got):\n%s", diff)
	}
	container := firstOfType(t, ctx, "azurerm_storage_container")
	if diff := cmp.Diff(containerArgs("private"), container.Args); diff != "" {
		t.Errorf("container args (-want +got):\n%s", diff)
	}
	for _, r := range []ir.Resource{account, container} {
		if r.SourceNode != "n7" || r.SourceLabel != "uploads" {
			t.Errorf("%s source = %q/%q", r.Type, r.SourceNode, r.SourceLabel)
		}
	}
	// The group, the account and the container.
	if got := len(ctx.Resources()); got != 3 {
		t.Errorf("resources = %d", got)
	}
	if got := countOfType(ctx, "azurerm_virtual_network"); got != 0 {
		t.Error("a bucket pulled in the network")
	}

	wantExports := resolve.BucketExports{
		Account:   ir.R(accountID, ir.Field("name")),
		Container: ir.R(containerID, ir.Field("name")),
		Scope:     ir.R(accountID, ir.Field("id")),
	}
	if diff := cmp.Diff(resolve.Exports(wantExports), handle.Exports); diff != "" {
		t.Errorf("exports (-want +got):\n%s", diff)
	}
	if handle.Primary != containerID {
		t.Errorf("primary = %s", handle.Primary)
	}
	wantOutputs := []ir.Output{
		{
			Name:        "uploads_account",
			Description: "Storage account holding the uploads container",
			Value:       ir.R(accountID, ir.Field("name")),
		},
		{
			Name:        "uploads_container",
			Description: "Name of the uploads container",
			Value:       ir.R(containerID, ir.Field("name")),
		},
	}
	if diff := cmp.Diff(wantOutputs, ctx.Outputs); diff != "" {
		t.Errorf("outputs (-want +got):\n%s", diff)
	}
}

func TestBucketWithoutVersioningTurnsBlobVersioningOff(t *testing.T) {
	ctx, _ := setupBucket(t, `{"versioning":false}`)

	if diff := cmp.Diff(accountArgs("shopdevuploads", false, false), firstOfType(t, ctx, "azurerm_storage_account").Args); diff != "" {
		t.Errorf("account args (-want +got):\n%s", diff)
	}
}

func TestPublicBucketOpensTheContainerToAnonymousBlobReads(t *testing.T) {
	ctx, _ := setupBucket(t, `{"public":true}`)

	if diff := cmp.Diff(accountArgs("shopdevuploads", true, true), firstOfType(t, ctx, "azurerm_storage_account").Args); diff != "" {
		t.Errorf("account args (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(containerArgs("blob"), firstOfType(t, ctx, "azurerm_storage_container").Args); diff != "" {
		t.Errorf("container args (-want +got):\n%s", diff)
	}
}

func TestBucketAccountNameFitsUnderALongProjectName(t *testing.T) {
	p := newProject(t, []ir.Node{bucketNode("n7", "user-uploads", "{}")})
	p.Name = "a-project-name-that-is-long-too"
	ctx := resolve.NewContext(p)
	resolveBucket(ctx, p.Nodes[0])

	name, _ := firstOfType(t, ctx, "azurerm_storage_account").Args.Get("name")
	if diff := cmp.Diff(ir.Value(ir.Str("aprojectnadevuseruploads")), name); diff != "" {
		t.Errorf("account name (-want +got):\n%s", diff)
	}
	if !storageAccountNamePattern.MatchString(string(name.(ir.String))) {
		t.Errorf("name %v is not 3 to 24 lowercase alphanumerics", name)
	}
	container, _ := firstOfType(t, ctx, "azurerm_storage_container").Args.Get("name")
	if diff := cmp.Diff(ir.Value(ir.Str("a-project-name-that-is-long-too-dev-user-uploads")), container); diff != "" {
		t.Errorf("container name (-want +got):\n%s", diff)
	}
}

func TestBucketEmitsValidHCL(t *testing.T) {
	p := newProject(t, []ir.Node{bucketNode("n7", "uploads", `{"public":true}`)})
	g, err := resolve.Run(p, New())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if errs := ir.ValidateGraph(g); len(errs) > 0 {
		t.Errorf("graph is not valid:\n%s", errs.Error())
	}
	files, err := hcl.Emit(g)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	main := unaligned(files["main.tf"])
	for _, want := range []string{
		`resource "azurerm_storage_account" "uploads" { name = "shopdevuploads" resource_group_name = azurerm_resource_group.main.name location = azurerm_resource_group.main.location account_tier = "Standard" account_replication_type = "LRS" min_tls_version = "TLS1_2" allow_nested_items_to_be_public = true blob_properties { versioning_enabled = true } }`,
		`resource "azurerm_storage_container" "uploads" { name = "shop-dev-uploads" storage_account_id = azurerm_storage_account.uploads.id container_access_type = "blob" }`,
	} {
		if !strings.Contains(main, want) {
			t.Errorf("main.tf lacks %s:\n%s", want, files["main.tf"])
		}
	}
}
