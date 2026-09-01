// Package acceptance renders deterministic, hardware-free evidence for the
// portable-profile amendment. It does not claim the parent change's complete
// seven-module or live-host acceptance scope.
package acceptance

import (
	"encoding/json"
	"fmt"
	"sort"

	"alex-cachyos/internal/app"
	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/planner"
)

const fixtureScope = "portable-profile-amendment-three-module-fixture"

type FixtureInput struct {
	Policy    catalog.ResolvedHostPolicy
	Plan      planner.Plan
	Check     app.CheckReport
	Execution FixtureExecution
	Rollback  app.RollbackInverse
}

type FixtureReport struct {
	Schema            string               `json:"schema"`
	Scope             string               `json:"scope"`
	EvidenceKind      app.EvidenceKind     `json:"evidenceKind"`
	Host              string               `json:"host"`
	Roles             []string             `json:"roles"`
	MergeTrace        []string             `json:"mergeTrace"`
	CatalogDigest     string               `json:"catalogDigest"`
	Pins              []string             `json:"pins"`
	AuthorizedRisks   []string             `json:"authorizedRisks"`
	Templates         []string             `json:"templates"`
	Overlays          []string             `json:"overlays"`
	PlanDigest        string               `json:"planDigest"`
	StepOrder         []string             `json:"stepOrder"`
	OfflineCheck      OfflineCheckEvidence `json:"offlineCheck"`
	DryRun            DryRunEvidence       `json:"dryRun"`
	Convergence       ConvergenceEvidence  `json:"convergence"`
	Rollback          RollbackEvidence     `json:"rollback"`
	ProductionValues  string               `json:"productionValues"`
	LiveEvidenceState string               `json:"liveEvidenceState"`
}

type OfflineCheckEvidence struct {
	Drift      bool   `json:"drift"`
	ExitIntent string `json:"exitIntent"`
	HardwareIO int    `json:"hardwareIo"`
}

type DryRunEvidence struct {
	MutationCount         int  `json:"mutationCount"`
	NetworkMutationCount  int  `json:"networkMutationCount"`
	LockAcquisitions      int  `json:"lockAcquisitions"`
	StateUnchanged        bool `json:"stateUnchanged"`
	AuditReceiptPublished bool `json:"auditReceiptPublished"`
}

type ConvergenceEvidence struct {
	FirstNoChange       bool `json:"firstNoChange"`
	SecondNoChange      bool `json:"secondNoChange"`
	MutationCount       int  `json:"mutationCount"`
	SecondMutationCount int  `json:"secondMutationCount"`
}

type RollbackEvidence struct {
	Action string `json:"action"`
	Target string `json:"target"`
}

func BuildFixtureReport(input FixtureInput) (FixtureReport, error) {
	if !input.Execution.verified {
		return FixtureReport{}, fmt.Errorf("fixture execution evidence is not derived from the application path")
	}
	catalogDigest, err := catalog.Digest(input.Policy.Desired)
	if err != nil {
		return FixtureReport{}, fmt.Errorf("catalog digest: %w", err)
	}
	planDigest, err := planner.Digest(input.Plan)
	if err != nil {
		return FixtureReport{}, fmt.Errorf("plan digest: %w", err)
	}
	trace := []string{"global"}
	for _, role := range input.Policy.Roles {
		trace = append(trace, "role:"+role)
	}
	trace = append(trace, "host:"+input.Policy.Name)
	steps := make([]string, len(input.Plan.Steps))
	for i, step := range input.Plan.Steps {
		steps[i] = step.ID
	}
	risks := make([]string, 0)
	for _, capability := range catalog.RiskCapabilities() {
		if input.Policy.Allows(capability) {
			risks = append(risks, string(capability))
		}
	}
	return FixtureReport{
		Schema: "alex-cachyos.acceptance-fixture/v1", Scope: fixtureScope,
		EvidenceKind: app.EvidenceFixture, Host: input.Policy.Name,
		Roles: append([]string(nil), input.Policy.Roles...), MergeTrace: trace,
		CatalogDigest: catalogDigest, Pins: pinIdentities(input.Policy.Desired), AuthorizedRisks: risks,
		Templates: append([]string(nil), input.Policy.Desired.Templates...), Overlays: append([]string(nil), input.Policy.Desired.Overlays...),
		PlanDigest: planDigest, StepOrder: steps,
		OfflineCheck:     OfflineCheckEvidence{Drift: input.Check.Drift, ExitIntent: string(input.Check.ExitIntent), HardwareIO: 0},
		DryRun:           input.Execution.dryRun,
		Convergence:      input.Execution.convergence,
		Rollback:         RollbackEvidence{Action: string(input.Rollback.Action), Target: input.Rollback.Target},
		ProductionValues: "deferred", LiveEvidenceState: "deferred-to-parent-WU-20",
	}, nil
}

func RenderFixtureReport(report FixtureReport) ([]byte, error) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func pinIdentities(value catalog.Catalog) []string {
	var result []string
	if value.Pins != nil {
		for name, version := range value.Pins.NPM {
			result = append(result, "npm:"+name+"@"+version)
		}
	}
	for name, pin := range value.CheckoutPins {
		result = append(result, "checkout:"+name+"@"+pin.Commit)
	}
	sort.Strings(result)
	return result
}
