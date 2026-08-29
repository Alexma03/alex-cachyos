package executor

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestEqualJSONAcceptsEquivalentDocuments(t *testing.T) {
	observed := []byte(`{"b":2,"a":{"x":1},"list":[1,2,3],"n":42}`)
	desired := []byte(`{ "a" : {"x": 1}, "n" : 42, "list" : [1, 2, 3], "b" : 2 }`)
	if err := EqualJSON(observed, desired); err != nil {
		t.Fatalf("EqualJSON = %v, want nil", err)
	}
}

func TestEqualJSONPreservesNumberLiterals(t *testing.T) {
	if err := EqualJSON([]byte(`1`), []byte(`1`)); err != nil {
		t.Fatalf("EqualJSON integers = %v, want nil", err)
	}
	if err := EqualJSON([]byte(`1`), []byte(`1.0`)); !errors.Is(err, ErrJSONMismatch) {
		t.Fatalf("EqualJSON(1, 1.0) = %v, want ErrJSONMismatch", err)
	}
	const large = `9007199254740993`
	if err := EqualJSON([]byte(large), []byte(large)); err != nil {
		t.Fatalf("EqualJSON large integer = %v, want nil", err)
	}
	if err := EqualJSON([]byte(large), []byte(`9007199254740994`)); !errors.Is(err, ErrJSONMismatch) {
		t.Fatalf("EqualJSON distinct large integers = %v, want ErrJSONMismatch", err)
	}
}

func TestEqualJSONRejectsDifferentValues(t *testing.T) {
	if err := EqualJSON([]byte(`{"a":1}`), []byte(`{"a":2}`)); !errors.Is(err, ErrJSONMismatch) {
		t.Fatalf("error = %v, want ErrJSONMismatch", err)
	}
}

func TestEqualJSONRejectsInvalidObserved(t *testing.T) {
	err := EqualJSON([]byte(`{broken`), []byte(`{}`))
	if !errors.Is(err, ErrInvalidObserved) {
		t.Fatalf("error = %v, want ErrInvalidObserved", err)
	}
	var comparison *ComparisonError
	if !errors.As(err, &comparison) || comparison.Side != SideObserved || comparison.Kind != FailureMalformed {
		t.Fatalf("error = %v, want malformed observed comparison error", err)
	}
}

func TestEqualJSONRejectsInvalidDesired(t *testing.T) {
	err := EqualJSON([]byte(`{}`), []byte(`{broken`))
	if !errors.Is(err, ErrInvalidDesired) {
		t.Fatalf("error = %v, want ErrInvalidDesired", err)
	}
	var comparison *ComparisonError
	if !errors.As(err, &comparison) || comparison.Side != SideDesired || comparison.Kind != FailureMalformed {
		t.Fatalf("error = %v, want malformed desired comparison error", err)
	}
}

func TestEqualJSONBothInvalidChoosesObserved(t *testing.T) {
	err := EqualJSON([]byte(`{observed-broken`), []byte(`{desired-broken`))
	if !errors.Is(err, ErrInvalidObserved) || errors.Is(err, ErrInvalidDesired) {
		t.Fatalf("error = %v, want only ErrInvalidObserved", err)
	}
	var comparison *ComparisonError
	if !errors.As(err, &comparison) || comparison.Side != SideObserved {
		t.Fatalf("error = %v, want observed side", err)
	}
}

func TestEqualJSONRejectsTrailingData(t *testing.T) {
	for _, test := range []struct {
		name     string
		observed string
		desired  string
		wantSide Side
	}{
		{"observed", `{} {}`, `{}`, SideObserved},
		{"observed garbage", `{} trailing`, `{}`, SideObserved},
		{"desired", `{}`, `{} {}`, SideDesired},
		{"desired garbage", `{}`, `{} trailing`, SideDesired},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := EqualJSON([]byte(test.observed), []byte(test.desired))
			var comparison *ComparisonError
			if !errors.As(err, &comparison) || comparison.Side != test.wantSide || comparison.Kind != FailureTrailing {
				t.Fatalf("error = %v, want trailing %s comparison error", err, test.wantSide)
			}
		})
	}
}

func TestEqualJSONRejectsWhitespaceEmpty(t *testing.T) {
	for _, test := range []struct {
		name     string
		observed string
		desired  string
		wantSide Side
	}{
		{"empty observed", ``, `{}`, SideObserved},
		{"whitespace observed", " \n\t ", `{}`, SideObserved},
		{"empty desired", `{}`, ``, SideDesired},
		{"whitespace desired", `{}`, " \n\t ", SideDesired},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := EqualJSON([]byte(test.observed), []byte(test.desired))
			var comparison *ComparisonError
			if !errors.As(err, &comparison) || comparison.Side != test.wantSide || comparison.Kind != FailureEmpty {
				t.Fatalf("error = %v, want empty %s comparison error", err, test.wantSide)
			}
		})
	}
}

func TestEqualJSONErrorsDoNotLeakPayloadOrCause(t *testing.T) {
	const secret = "s3cr3t-p4yl0ad-7f3a"
	err := EqualJSON([]byte(`{"token":"`+secret+`"`), []byte(`{}`))
	if err == nil {
		t.Fatal("EqualJSON = nil, want error")
	}
	if message := err.Error(); strings.Contains(message, secret) || strings.Contains(message, "unexpected end of JSON input") {
		t.Fatalf("Error() leaks payload or cause: %q", message)
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		t.Fatalf("error exposes underlying json cause: %v", syntaxErr)
	}
}
