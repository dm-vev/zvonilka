package reaction

import "errors"

var (
	// ErrInvalidInput indicates malformed catalog data.
	ErrInvalidInput = errors.New("invalid input")
	// ErrNotFound indicates that a reaction definition does not exist.
	ErrNotFound = errors.New("not found")
	// ErrEmptyCatalog indicates that no reaction definitions are configured.
	ErrEmptyCatalog = errors.New("reaction catalog is empty")
	// ErrInvalidCatalog indicates that one or more catalog assets are unusable.
	ErrInvalidCatalog = errors.New("invalid reaction catalog")
	// ErrInactive indicates that a reaction is no longer available for new messages.
	ErrInactive = errors.New("reaction is inactive")
)

// Definition describes one globally available emoji reaction.
type Definition struct {
	Emoji             string
	Title             string
	Active            bool
	SortOrder         uint32
	StaticIcon        string
	AppearAnimation   string
	SelectAnimation   string
	ActivateAnimation string
	EffectAnimation   string
	AroundAnimation   string
	CenterAnimation   string
}

// Catalog is the versioned reaction catalog exposed to clients.
type Catalog struct {
	Version      string
	DefaultEmoji string
	Reactions    []Definition
}
