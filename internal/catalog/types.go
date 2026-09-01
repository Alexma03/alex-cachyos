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

// RiskCapability identifies one closed, host-owned authorization. Runtime
// observations may deny an authorized capability but are never authority to
// enable one.
type RiskCapability string

const (
	RiskFingerprintPAM          RiskCapability = "fingerprintPam"
	RiskFixedDisplays           RiskCapability = "fixedDisplays"
	RiskFixedInputDevices       RiskCapability = "fixedInputDevices"
	RiskLiteralHomePaths        RiskCapability = "literalHomePaths"
	RiskBootstrapSystemUpdate   RiskCapability = "bootstrapSystemUpdate"
	RiskBootstrapPackageRemoval RiskCapability = "bootstrapPackageRemoval"
	RiskBootstrapBootMutation   RiskCapability = "bootstrapBootMutation"
	RiskCosmicPrune             RiskCapability = "cosmicPrune"
)

var riskCapabilities = [...]RiskCapability{
	RiskFingerprintPAM,
	RiskFixedDisplays,
	RiskFixedInputDevices,
	RiskLiteralHomePaths,
	RiskBootstrapSystemUpdate,
	RiskBootstrapPackageRemoval,
	RiskBootstrapBootMutation,
	RiskCosmicPrune,
}

// RiskCapabilities returns the canonical capability order as a fresh slice.
func RiskCapabilities() []RiskCapability {
	return append([]RiskCapability(nil), riskCapabilities[:]...)
}

// RiskPolicy is a closed set of host-owned opt-ins. Zero values are denied.
type RiskPolicy struct {
	FingerprintPAM          bool `yaml:"fingerprintPam" json:"fingerprintPam"`
	FixedDisplays           bool `yaml:"fixedDisplays" json:"fixedDisplays"`
	FixedInputDevices       bool `yaml:"fixedInputDevices" json:"fixedInputDevices"`
	LiteralHomePaths        bool `yaml:"literalHomePaths" json:"literalHomePaths"`
	BootstrapSystemUpdate   bool `yaml:"bootstrapSystemUpdate" json:"bootstrapSystemUpdate"`
	BootstrapPackageRemoval bool `yaml:"bootstrapPackageRemoval" json:"bootstrapPackageRemoval"`
	BootstrapBootMutation   bool `yaml:"bootstrapBootMutation" json:"bootstrapBootMutation"`
	CosmicPrune             bool `yaml:"cosmicPrune" json:"cosmicPrune"`
}

// Allows reports catalog authority only. Unknown capabilities are denied.
func (policy RiskPolicy) Allows(capability RiskCapability) bool {
	switch capability {
	case RiskFingerprintPAM:
		return policy.FingerprintPAM
	case RiskFixedDisplays:
		return policy.FixedDisplays
	case RiskFixedInputDevices:
		return policy.FixedInputDevices
	case RiskLiteralHomePaths:
		return policy.LiteralHomePaths
	case RiskBootstrapSystemUpdate:
		return policy.BootstrapSystemUpdate
	case RiskBootstrapPackageRemoval:
		return policy.BootstrapPackageRemoval
	case RiskBootstrapBootMutation:
		return policy.BootstrapBootMutation
	case RiskCosmicPrune:
		return policy.CosmicPrune
	default:
		return false
	}
}

// PinsDocument is the presence-aware YAML representation of source-specific
// pins. Each source map is a pointer so an omitted source remains distinct from
// an explicitly empty source map.
type PinsDocument struct {
	NPM               *map[string]string                    `yaml:"npm" json:"npm,omitempty"`
	PacmanArtifacts   *map[string]PacmanArtifactPinDocument `yaml:"pacmanArtifacts" json:"pacmanArtifacts,omitempty"`
	AURLocal          *map[string]AURLocalPinDocument       `yaml:"aurLocal" json:"aurLocal,omitempty"`
	RemoteArtifacts   *map[string]RemoteArtifactPinDocument `yaml:"remoteArtifacts" json:"remoteArtifacts,omitempty"`
	LocalPathPackages *map[string]string                    `yaml:"localPathPackages" json:"localPathPackages,omitempty"`
}

// PacmanArtifactPinDocument retains field presence until the structural schema
// has accepted the pin and it can be converted to PacmanArtifactPin.
type PacmanArtifactPinDocument struct {
	Package *string `yaml:"package" json:"package,omitempty"`
	Version *string `yaml:"version" json:"version,omitempty"`
	Source  *string `yaml:"source" json:"source,omitempty"`
	SHA256  *string `yaml:"sha256" json:"sha256,omitempty"`
}

// AURLocalPinDocument retains field presence until the structural schema has
// accepted the pin and it can be converted to AURLocalPin.
type AURLocalPinDocument struct {
	SourceCommit *string `yaml:"sourceCommit" json:"sourceCommit,omitempty"`
	PatchSHA256  *string `yaml:"patchSHA256" json:"patchSHA256,omitempty"`
}

// RemoteArtifactPinDocument retains field presence until the structural schema
// has accepted the pin and it can be converted to RemoteArtifactPin.
type RemoteArtifactPinDocument struct {
	URL    *string `yaml:"url" json:"url,omitempty"`
	SHA256 *string `yaml:"sha256" json:"sha256,omitempty"`
}

// Pins is the typed form of source-specific pins. Nil source maps mean that the
// source was omitted; non-nil empty maps mean that it was explicitly present.
type Pins struct {
	NPM               map[string]string            `json:"npm,omitempty"`
	PacmanArtifacts   map[string]PacmanArtifactPin `json:"pacmanArtifacts,omitempty"`
	AURLocal          map[string]AURLocalPin       `json:"aurLocal,omitempty"`
	RemoteArtifacts   map[string]RemoteArtifactPin `json:"remoteArtifacts,omitempty"`
	LocalPathPackages map[string]string            `json:"localPathPackages,omitempty"`
}

// Document is the presence-aware YAML representation used while decoding a
// catalog layer. Required fields use pointers so malformed documents can be
// rejected without confusing omission with a zero value. Optional collections
// use pointers so an explicit empty collection remains distinct from omission.
type Document struct {
	CatalogVersion *int                            `yaml:"catalogVersion" json:"catalogVersion,omitempty"`
	Kind           *Kind                           `yaml:"kind" json:"kind,omitempty"`
	Roles          *[]string                       `yaml:"roles" json:"roles,omitempty"`
	RiskPolicy     *RiskPolicy                     `yaml:"riskPolicy" json:"riskPolicy,omitempty"`
	Modules        *ModuleSet                      `yaml:"modules" json:"modules,omitempty"`
	Templates      *[]string                       `yaml:"templates" json:"templates,omitempty"`
	Overlays       *[]string                       `yaml:"overlays" json:"overlays,omitempty"`
	Pins           *PinsDocument                   `yaml:"pins" json:"pins,omitempty"`
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
	Roles          []string               `json:"roles,omitempty"`
	RiskPolicy     *RiskPolicy            `json:"riskPolicy,omitempty"`
	Modules        ModuleSet              `json:"modules,omitempty"`
	Templates      []string               `json:"templates,omitempty"`
	Overlays       []string               `json:"overlays,omitempty"`
	Pins           *Pins                  `json:"pins,omitempty"`
	CheckoutPins   map[string]CheckoutPin `json:"checkoutPins,omitempty"`
}

// CheckoutPin is the typed form of a valid checkout pin.
type CheckoutPin struct {
	Remote string `json:"remote"`
	Branch string `json:"branch"`
	Commit string `json:"commit"`
}
