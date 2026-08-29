package executor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
)

// Side identifies which side of a comparison failed validation.
type Side string

const (
	SideObserved Side = "observed"
	SideDesired  Side = "desired"
)

// FailureKind categorizes why an input was not exactly one JSON value.
type FailureKind string

const (
	FailureEmpty     FailureKind = "empty"
	FailureMalformed FailureKind = "malformed"
	FailureTrailing  FailureKind = "trailing"
)

var (
	ErrInvalidObserved = errors.New("invalid observed JSON")
	ErrInvalidDesired  = errors.New("invalid desired JSON")
	ErrJSONMismatch    = errors.New("JSON values differ")
)

// ComparisonError reports which side failed validation and why. Error exposes
// neither the input payload nor the underlying parse cause.
type ComparisonError struct {
	Side Side
	Kind FailureKind
}

func (e *ComparisonError) Error() string {
	return fmt.Sprintf("invalid %s JSON: %s", e.Side, e.Kind)
}

func (e *ComparisonError) Unwrap() error {
	if e.Side == SideObserved {
		return ErrInvalidObserved
	}
	return ErrInvalidDesired
}

// EqualJSON compares observed and desired as exactly one complete JSON value
// each. Object key order and whitespace are insignificant; number literals are
// preserved exactly. Empty, malformed, or trailing data fail closed. Observed
// is validated first, and the input slices are never mutated.
func EqualJSON(observed, desired []byte) error {
	observedValue, ok, kind := parseSingleJSON(observed)
	if !ok {
		return &ComparisonError{Side: SideObserved, Kind: kind}
	}
	desiredValue, ok, kind := parseSingleJSON(desired)
	if !ok {
		return &ComparisonError{Side: SideDesired, Kind: kind}
	}
	if !reflect.DeepEqual(observedValue, desiredValue) {
		return ErrJSONMismatch
	}
	return nil
}

func parseSingleJSON(data []byte) (value any, ok bool, kind FailureKind) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, false, FailureEmpty
		}
		return nil, false, FailureMalformed
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, false, FailureTrailing
	}
	return decoded, true, ""
}
