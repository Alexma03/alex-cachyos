package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	ErrInvalidCatalogTag = errors.New("invalid catalog tag")
	ErrCatalogTagExists  = errors.New("catalog tag already exists")
	ErrTagNotAnnotated   = errors.New("catalog tag is not annotated")
	ErrTagNotAtHEAD      = errors.New("catalog tag does not point at HEAD")
	ErrTagNotDescribed   = errors.New("catalog tag is not visible to git describe")

	// Short aliases keep callers from depending on the catalog-specific names.
	ErrInvalidTag = ErrInvalidCatalogTag
	ErrTagExists  = ErrCatalogTagExists
)

var catalogTagPattern = regexp.MustCompile(`^catalog-v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)$`)

type TagSpec struct {
	Name        string
	Message     string
	Tagger      string
	Identity    string
	TaggerName  string
	TaggerEmail string
}

type CatalogTagSpec = TagSpec

type TagResult struct {
	Name            string
	TagName         string
	Commit          string
	Head            string
	TaggerName      string
	TaggerEmail     string
	Message         string
	DescribeVisible bool
}

type CatalogTagResult = TagResult

type CheckpointSpec = TagSpec

func ValidateCatalogTag(name string) error {
	if !catalogTagPattern.MatchString(name) {
		return fmt.Errorf("%w: %q must match catalog-vX.Y.Z", ErrInvalidCatalogTag, name)
	}
	return nil
}

func ValidateTagName(name string) error { return ValidateCatalogTag(name) }

func (c *Client) CreateTag(ctx context.Context, cwd string, spec TagSpec) (TagResult, error) {
	var result TagResult
	name, email, err := validateTagSpec(spec)
	if err != nil {
		return result, err
	}
	if cwd == "" || !filepath.IsAbs(cwd) {
		return result, errors.New("tag repository must be absolute")
	}
	if c == nil || c.runner == nil {
		return result, errors.New("nil git runner")
	}

	runner := NewSafeRunner(c.runner)
	trun := func(operation string, argv ...string) (CommandResult, error) {
		request := CommandRequest{Operation: operation, Executable: "git", Argv: append([]string(nil), argv...), Cwd: cwd}
		got, runErr := runner.Run(ctx, request)
		if runErr != nil || got.ExitCode != 0 {
			if runErr != nil {
				return got, fmt.Errorf("git %s: %w", operation, runErr)
			}
			return got, fmt.Errorf("git %s failed with exit code %d", operation, got.ExitCode)
		}
		return got, nil
	}

	headOutput, err := trun("head", "rev-parse", "--verify", "HEAD")
	if err != nil {
		return result, err
	}
	head := strings.TrimSpace(string(headOutput.Stdout))
	if head == "" || strings.ContainsAny(head, " \t\r\n") {
		return result, errors.New("git HEAD was empty or malformed")
	}

	identityOutput, err := trun("identity", "var", "GIT_COMMITTER_IDENT")
	if err != nil {
		return result, errors.New("explicit tagger identity is unavailable")
	}
	identity := name + " <" + email + ">"
	if !strings.HasPrefix(strings.TrimSpace(string(identityOutput.Stdout)), identity+" ") {
		return result, fmt.Errorf("configured Git tagger does not match explicit identity %q", identity)
	}

	existing, existingErr := runner.Run(ctx, CommandRequest{
		Operation:  "existing-tag",
		Executable: "git",
		Argv:       []string{"show-ref", "--verify", "--quiet", "refs/tags/" + spec.Name},
		Cwd:        cwd,
	})
	if existingErr == nil && existing.ExitCode == 0 {
		return result, fmt.Errorf("%w: %s", ErrCatalogTagExists, spec.Name)
	}
	if existing.ExitCode != 1 {
		if existingErr != nil {
			return result, fmt.Errorf("inspect existing tag: %w", existingErr)
		}
		return result, fmt.Errorf("inspect existing tag failed with exit code %d", existing.ExitCode)
	}

	if _, err := trun("create-tag", "tag", "--annotate", "--message", spec.Message, "--", spec.Name, "HEAD"); err != nil {
		return result, err
	}

	typ, err := trun("tag-type", "cat-file", "-t", "refs/tags/"+spec.Name)
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(string(typ.Stdout)) != "tag" {
		return result, ErrTagNotAnnotated
	}

	target, err := trun("tag-target", "rev-parse", "--verify", "refs/tags/"+spec.Name+"^{}")
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(string(target.Stdout)) != head {
		return result, fmt.Errorf("%w: got %q, want %q", ErrTagNotAtHEAD, strings.TrimSpace(string(target.Stdout)), head)
	}

	object, err := trun("tag-object", "cat-file", "-p", "refs/tags/"+spec.Name)
	if err != nil {
		return result, err
	}
	if err := validateTagObject(object.Stdout, spec.Name, identity, spec.Message); err != nil {
		return result, err
	}

	describe, err := trun("describe", "describe", "--exact-match", "--tags", "HEAD")
	if err != nil {
		return result, ErrTagNotDescribed
	}
	if strings.TrimSpace(string(describe.Stdout)) != spec.Name {
		return result, fmt.Errorf("%w: got %q, want %q", ErrTagNotDescribed, strings.TrimSpace(string(describe.Stdout)), spec.Name)
	}

	result = TagResult{
		Name:            spec.Name,
		TagName:         spec.Name,
		Commit:          head,
		Head:            head,
		TaggerName:      name,
		TaggerEmail:     email,
		Message:         spec.Message,
		DescribeVisible: true,
	}
	return result, nil
}

func (c *Client) CreateAnnotatedTag(ctx context.Context, cwd string, spec TagSpec) (TagResult, error) {
	return c.CreateTag(ctx, cwd, spec)
}

func (c *Client) CreateCheckpoint(ctx context.Context, cwd string, spec CheckpointSpec) (TagResult, error) {
	return c.CreateTag(ctx, cwd, spec)
}

func (c *Client) Checkpoint(ctx context.Context, cwd string, spec CheckpointSpec) (TagResult, error) {
	return c.CreateTag(ctx, cwd, spec)
}

func validateTagSpec(spec TagSpec) (string, string, error) {
	if err := ValidateCatalogTag(spec.Name); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(spec.Message) == "" || strings.IndexByte(spec.Message, 0) >= 0 {
		return "", "", fmt.Errorf("%w: tag message is required", ErrInvalidCatalogTag)
	}

	fullIdentity := spec.Tagger
	if spec.Identity != "" {
		if fullIdentity != "" && fullIdentity != spec.Identity {
			return "", "", fmt.Errorf("%w: tagger identities disagree", ErrInvalidCatalogTag)
		}
		fullIdentity = spec.Identity
	}
	name, email := strings.TrimSpace(spec.TaggerName), strings.TrimSpace(spec.TaggerEmail)
	if fullIdentity != "" {
		parsedName, parsedEmail, ok := splitTaggerIdentity(fullIdentity)
		if ok {
			if name != "" && name != parsedName || email != "" && email != parsedEmail {
				return "", "", fmt.Errorf("%w: tagger identities disagree", ErrInvalidCatalogTag)
			}
			name, email = parsedName, parsedEmail
		} else if name == "" && email != "" {
			name = strings.TrimSpace(fullIdentity)
		} else if name == "" && email == "" {
			name = strings.TrimSpace(fullIdentity)
		}
	}
	if name == "" || email == "" || invalidIdentityPart(name) || invalidIdentityPart(email) || strings.ContainsAny(email, "<>") {
		return "", "", fmt.Errorf("%w: explicit tagger name and email are required", ErrInvalidCatalogTag)
	}
	return name, email, nil
}

func splitTaggerIdentity(value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	open := strings.LastIndex(value, " <")
	if open <= 0 || !strings.HasSuffix(value, ">") {
		return "", "", false
	}
	name := strings.TrimSpace(value[:open])
	email := strings.TrimSuffix(strings.TrimSpace(value[open+2:]), ">")
	if name == "" || email == "" {
		return "", "", false
	}
	return name, email, true
}

func invalidIdentityPart(value string) bool {
	return strings.TrimSpace(value) != value || strings.IndexByte(value, 0) >= 0 || strings.ContainsAny(value, "\r\n")
}

func validateTagObject(data []byte, tag, identity, message string) error {
	separator := bytes.Index(data, []byte("\n\n"))
	if separator < 0 {
		return errors.New("annotated tag object has no message")
	}
	header := string(data[:separator])
	body := data[separator+2:]
	if !strings.Contains(header, "\ntag "+tag+"\n") && !strings.HasPrefix(header, "tag "+tag+"\n") {
		return errors.New("annotated tag object has an unexpected name")
	}
	taggerLine := "tagger " + identity + " "
	if !strings.Contains(header, "\n"+taggerLine) && !strings.HasPrefix(header, taggerLine) {
		return errors.New("annotated tag object has an unexpected tagger")
	}
	if !bytes.Equal(body, []byte(message+"\n")) {
		return errors.New("annotated tag object has an unexpected message")
	}
	return nil
}
