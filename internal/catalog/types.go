package catalog

// Kind identifies the layer represented by one catalog document.
type Kind string

const (
	KindGlobal Kind = "Global"
	KindRole   Kind = "Role"
	KindHost   Kind = "Host"
)

// ModuleSet retains the module names and their explicit boolean values. A
// missing map key is different from a key whose value is false; a nil map in
// a Catalog also means that the modules field was omitted.
type ModuleSet map[string]bool

// Document is the presence-aware YAML representation used while decoding a
// catalog layer. Required fields use pointers so malformed documents can be
// rejected without confusing omission with a zero value. Optional collections
// use pointers so an explicit empty collection remains distinct from omission.
type Document struct {
	CatalogVersion *int                            `yaml:"catalogVersion" json:"catalogVersion,omitempty"`
	Kind           *Kind                           `yaml:"kind" json:"kind,omitempty"`
	Modules        *ModuleSet                      `yaml:"modules" json:"modules,omitempty"`
	Templates      *[]string                       `yaml:"templates" json:"templates,omitempty"`
	Overlays       *[]string                       `yaml:"overlays" json:"overlays,omitempty"`
	CheckoutPins   *map[string]CheckoutPinDocument `yaml:"checkoutPins" json:"checkoutPins,omitempty"`
}

// CheckoutPinDocument is the presence-aware YAML representation of a checkout
// pin. The schema layer requires all three fields before a typed pin is made.
type CheckoutPinDocument struct {
	Remote *string `yaml:"remote" json:"remote,omitempty"`
	Branch *string `yaml:"branch" json:"branch,omitempty"`
	Commit *string `yaml:"commit" json:"commit,omitempty"`
}

// Catalog is the typed, schema-valid form of one catalog layer. Optional
// collections preserve omission with nil and explicit empty values with a
// non-nil empty collection.
type Catalog struct {
	CatalogVersion int                    `json:"catalogVersion"`
	Kind           Kind                   `json:"kind"`
	Modules        ModuleSet              `json:"modules,omitempty"`
	Templates      []string               `json:"templates,omitempty"`
	Overlays       []string               `json:"overlays,omitempty"`
	CheckoutPins   map[string]CheckoutPin `json:"checkoutPins,omitempty"`
}

// CheckoutPin is the typed form of a valid checkout pin.
type CheckoutPin struct {
	Remote string `json:"remote"`
	Branch string `json:"branch"`
	Commit string `json:"commit"`
}
