package workspace

import (
	"bytes"
	"errors"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/mooncitizen/togen/internal/ir"
)

type compiledSchema struct {
	url  string
	raw  []byte
	once sync.Once
	sch  *jsonschema.Schema
	err  error
}

func (c *compiledSchema) get() (*jsonschema.Schema, error) {
	c.once.Do(func() {
		var doc any
		doc, c.err = jsonschema.UnmarshalJSON(bytes.NewReader(c.raw))
		if c.err != nil {
			return
		}
		compiler := jsonschema.NewCompiler()
		if c.err = compiler.AddResource(c.url, doc); c.err != nil {
			return
		}
		c.sch, c.err = compiler.Compile(c.url)
	})
	return c.sch, c.err
}

// A whole-document error has no path, and the display default for an empty
// one is the project, so the file's own name stands in.
func (c *compiledSchema) check(instance any, file string) ir.Errors {
	sch, err := c.get()
	if err != nil {
		return ir.Errors{{Message: err.Error()}}
	}
	err = sch.Validate(instance)
	if err == nil {
		return nil
	}
	var invalid *jsonschema.ValidationError
	if !errors.As(err, &invalid) {
		return ir.Errors{{Message: err.Error()}}
	}
	errs := ir.SchemaErrors(invalid, instance)
	for i := range errs {
		if errs[i].Path == "" {
			errs[i].Path = file
		}
	}
	return errs
}
