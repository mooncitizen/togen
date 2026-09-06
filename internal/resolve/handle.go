package resolve

import (
	"reflect"
	"slices"

	"github.com/mooncitizen/togen/internal/ir"
)

type Exports interface{ isExports() }

type DatabaseExports struct{ Host, Port, Name, User, Password, SecretARN ir.Value }

// ARN and InvokeARN are AWS. URL and PrincipalID are Azure: the function app's hostname and the
// managed identity that edges grant to.
type FunctionExports struct{ ARN, InvokeARN, FunctionName, URL, PrincipalID ir.Value }

type GatewayExports struct{ APIID, ExecutionARN, URL ir.Value }

// Cloud Run backs a GCP function as well as a GCP service, so both export the same things: the
// service an invoker binding names, the URL a caller is given, and the account edges grant to.
type CloudRunExports struct{ Service, Location, URL, ServiceAccount ir.Value }

// ServiceExports carries a nil URL when the service is not reachable from the internet, and a
// nil ListenerARN until something puts a load balancer in front of it.
type ServiceExports struct {
	Port        ir.Value
	Public      bool
	URL         ir.Value
	ListenerARN ir.Value
}

type QueueExports struct {
	ARN, URL, Name ir.Value
	FIFO           bool
}

type BucketExports struct{ ARN, Name ir.Value }

type CacheExports struct{ Host, Port ir.Value }

func (DatabaseExports) isExports() {}
func (FunctionExports) isExports() {}
func (GatewayExports) isExports()  {}
func (CloudRunExports) isExports() {}
func (ServiceExports) isExports()  {}
func (QueueExports) isExports()    {}
func (BucketExports) isExports()   {}
func (CacheExports) isExports()    {}

type Handle struct {
	Node          ir.Node
	Primary       ir.ID
	SecurityGroup *ir.ID
	Env           ir.Attrs
	Statements    []ir.Value
	NeedsNetwork  bool
	Exports       Exports
	Finalise      func()
}

func (h *Handle) SetEnv(key string, v ir.Value) { h.Env.Set(key, v) }

func (h *Handle) AddStatement(s ir.Value) {
	for _, existing := range h.Statements {
		if reflect.DeepEqual(existing, s) {
			return
		}
	}
	h.Statements = append(h.Statements, s)
}

// Go maps do not keep insertion order, so env is emitted alphabetically to stay deterministic.
func SortedEnv(env map[string]string) ir.Attrs {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make(ir.Attrs, 0, len(keys))
	for _, k := range keys {
		out = append(out, ir.A(k, ir.Str(env[k])))
	}
	return out
}
