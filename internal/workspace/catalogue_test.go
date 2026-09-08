package workspace

import (
	"slices"
	"testing"

	"github.com/mooncitizen/togen/internal/catalogue"
	"github.com/mooncitizen/togen/internal/ir"
	"github.com/mooncitizen/togen/internal/resolve/aws"
	"github.com/mooncitizen/togen/internal/resolve/azure"
	"github.com/mooncitizen/togen/internal/resolve/gcp"
)

var registered = map[ir.CloudProvider][]ir.NodeType{
	ir.ProviderAWS:   aws.Resolvers(),
	ir.ProviderGCP:   gcp.Resolvers(),
	ir.ProviderAzure: azure.Resolvers(),
}

// The catalogue is what the studio and the schema believe. A tier that disagrees with the
// resolvers means a palette tile that lies, so the two are held together here.
func TestTiersMatchTheResolvers(t *testing.T) {
	for provider, types := range registered {
		for _, e := range catalogue.Embedded().For(string(provider)) {
			has := slices.Contains(types, ir.NodeType(e.ID))
			switch {
			case e.Tier == catalogue.TierGenerates && !has:
				t.Errorf("%s says it generates %s, but the resolver has no function for it", provider, e.ID)
			case e.Tier == catalogue.TierDraws && has:
				t.Errorf("%s says it only draws %s, but the resolver has a function for it", provider, e.ID)
			}
		}
		for _, nodeType := range types {
			if _, ok := catalogue.Embedded().Lookup(string(provider), string(nodeType)); !ok {
				t.Errorf("the %s resolver has a function for %s, which is not in its catalogue", provider, nodeType)
			}
		}
	}
}

func TestEveryUsageKeyBelongsToATypeThatTakesIt(t *testing.T) {
	for _, e := range catalogue.Embedded().All() {
		for _, key := range e.Usage {
			if !slices.Contains(allUsageKeys, key) {
				t.Errorf("%s takes usage key %q, which no node usage record has", e.ID, key)
			}
		}
	}
}

var allUsageKeys = []string{"requests", "invocations", "durationMs", "messages", "storageGb", "egressGb", "natGb"}
