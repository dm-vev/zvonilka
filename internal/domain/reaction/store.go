package reaction

import "context"

// Store persists the global reaction catalog.
type Store interface {
	WithinTx(context.Context, func(Store) error) error
	SaveDefinition(context.Context, Definition) (Definition, error)
	DefinitionByEmoji(context.Context, string) (Definition, error)
	Definitions(context.Context) ([]Definition, error)
}
