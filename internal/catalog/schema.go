package catalog

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"alex-cachyos/internal/assets"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

const catalogSchemaAsset = "data/catalog/schema/catalog-v1.schema.json"

type SchemaValidator struct{ schema *jsonschema.Schema }

type ValidationError struct {
	Path    string
	Message string
}

func (e ValidationError) Error() string { return e.Path + ": " + e.Message }

type SchemaValidationError struct {
	Issues []ValidationError
	cause  error
}

func (e *SchemaValidationError) Error() string {
	if len(e.Issues) == 0 {
		return "catalog schema validation failed"
	}
	parts := make([]string, len(e.Issues))
	for i, issue := range e.Issues {
		parts[i] = issue.Error()
	}
	return "catalog schema validation: " + strings.Join(parts, "; ")
}
func (e *SchemaValidationError) Unwrap() error { return e.cause }
func (e *SchemaValidationError) Errors() []ValidationError {
	return append([]ValidationError(nil), e.Issues...)
}
func ValidationErrors(err error) []ValidationError {
	var validationErr *SchemaValidationError
	if errors.As(err, &validationErr) {
		return validationErr.Errors()
	}
	return nil
}

func NewSchemaValidator() (*SchemaValidator, error) {
	data, err := assets.FS.ReadFile(catalogSchemaAsset)
	if err != nil {
		return nil, fmt.Errorf("read catalog schema: %w", err)
	}
	return NewSchemaValidatorFromJSON(data)
}

func NewSchemaValidatorFromJSON(data []byte) (*SchemaValidator, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode catalog schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("catalog-v1.schema.json", doc); err != nil {
		return nil, fmt.Errorf("register catalog schema: %w", err)
	}
	compiled, err := compiler.Compile("catalog-v1.schema.json")
	if err != nil {
		return nil, fmt.Errorf("compile catalog schema: %w", err)
	}
	return &SchemaValidator{schema: compiled}, nil
}

func ValidateJSON(data []byte) error {
	validator, err := NewSchemaValidator()
	if err != nil {
		return err
	}
	return validator.ValidateJSON(data)
}

func (v *SchemaValidator) ValidateJSON(data []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode catalog JSON: %w", err)
	}
	return v.Validate(doc)
}

func (v *SchemaValidator) Validate(doc any) error {
	if v == nil || v.schema == nil {
		return errors.New("catalog schema validator is nil")
	}
	if err := v.schema.Validate(doc); err != nil {
		return mapSchemaError(err)
	}
	return nil
}

func mapSchemaError(err error) error {
	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		return fmt.Errorf("catalog schema validation: %w", err)
	}
	issues := make([]ValidationError, 0, 1)
	collectSchemaErrors(validationErr, &issues)
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Path == issues[j].Path {
			return issues[i].Message < issues[j].Message
		}
		return issues[i].Path < issues[j].Path
	})
	return &SchemaValidationError{Issues: issues, cause: err}
}

func collectSchemaErrors(err *jsonschema.ValidationError, issues *[]ValidationError) {
	if len(err.Causes) != 0 {
		for _, cause := range err.Causes {
			collectSchemaErrors(cause, issues)
		}
		return
	}
	path := jsonPointer(err.InstanceLocation)
	switch kindErr := err.ErrorKind.(type) {
	case *kind.AdditionalProperties:
		for _, property := range kindErr.Properties {
			*issues = append(*issues, ValidationError{Path: childPointer(path, property), Message: "additional property is not allowed"})
		}
		return
	case *kind.Required:
		for _, property := range kindErr.Missing {
			*issues = append(*issues, ValidationError{Path: childPointer(path, property), Message: "required property is missing"})
		}
		return
	}
	message := "schema validation failed"
	if output := err.BasicOutput(); output.Error != nil {
		message = output.Error.String()
	}
	*issues = append(*issues, ValidationError{Path: path, Message: message})
}

func jsonPointer(parts []string) string {
	if len(parts) == 0 {
		return "/"
	}
	var b strings.Builder
	for _, part := range parts {
		b.WriteByte('/')
		b.WriteString(escapePointer(part))
	}
	return b.String()
}
func childPointer(parent, part string) string {
	if parent == "/" {
		return "/" + escapePointer(part)
	}
	return parent + "/" + escapePointer(part)
}
func escapePointer(part string) string {
	part = strings.ReplaceAll(part, "~", "~0")
	return strings.ReplaceAll(part, "/", "~1")
}
