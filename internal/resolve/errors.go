package resolve

import "github.com/mooncitizen/togen/internal/ir"

type ResolveError struct{ Errors ir.Errors }

func (e *ResolveError) Error() string { return e.Errors.Error() }

// Raised by Context.Fail for broken internal invariants. Run recovers it; nothing else should.
type internalError struct{ err ir.ValidationError }
