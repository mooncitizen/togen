package resolve

import (
	"reflect"

	"github.com/mooncitizen/togen/internal/ir"
)

type Exports interface{ isExports() }

type DatabaseExports struct{ Host, Port, Name, SecretARN ir.Value }

type FunctionExports struct{ ARN, InvokeARN, FunctionName ir.Value }

type GatewayExports struct{ APIID, ExecutionARN, URL ir.Value }

func (DatabaseExports) isExports() {}
func (FunctionExports) isExports() {}
func (GatewayExports) isExports()  {}

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
