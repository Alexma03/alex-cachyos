package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"alex-cachyos/internal/adopt"
	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/gitx"
	"alex-cachyos/internal/receipt"
	"alex-cachyos/internal/statepath"
)

var (
	ErrInvalidRollbackTarget = errors.New("invalid rollback target")
	ErrCheckpointValidation  = errors.New("checkpoint validation failed")
)

type ReceiptReader interface {
	Read(string) (receipt.Receipt, string, error)
}

type ReceiptPublisher interface {
	Publish(receipt.Receipt) (string, error)
}

type RollbackFilePort interface {
	Observe(context.Context, string) (LiveFile, error)
	Execute(context.Context, RollbackInverse) error
}

type MutationLock interface {
	Release() error
}

type MutationLockFactory func() (MutationLock, error)

func NewMutationLockFactory(paths statepath.Paths) MutationLockFactory {
	return func() (MutationLock, error) { return AcquireLock(paths) }
}

type ReceiptCommandStore interface {
	Current() (receipt.Receipt, string, error)
	Read(string) (receipt.Receipt, string, error)
}

type ApplyInputFactory interface {
	Build(context.Context, CommandRequest, catalog.ResolvedHostPolicy) (ApplyInput, error)
}

type ApplyRunner interface {
	Apply(context.Context, ApplyInput) (ApplyResult, error)
}

// NewApplyCommand binds resolved catalog policy and parsed selection to the
// existing convergent applier without embedding production catalog values.
func NewApplyCommand(factory ApplyInputFactory, runner ApplyRunner) CommandOperations {
	return CommandOperationsFunc(func(ctx context.Context, request CommandRequest, policy catalog.ResolvedHostPolicy) (CommandResult, error) {
		if factory == nil || runner == nil || request.Command != CommandApply {
			return CommandResult{}, ErrCommandUnavailable
		}
		input, err := factory.Build(ctx, request, policy)
		if err != nil {
			return CommandResult{}, err
		}
		input.DryRun = request.DryRun
		result, err := runner.Apply(ctx, input)
		if err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Command: CommandApply, Host: policy.Name, Receipt: &result.Receipt, ReceiptPath: result.ReceiptPath}, nil
	})
}

type AdoptionPort interface {
	Adopt(string, string) (adopt.Record, error)
}

type AdoptionResult struct {
	Target         string `json:"target"`
	Backup         string `json:"backup"`
	FirstReceiptID string `json:"firstReceiptId"`
}

// NewAdoptCommand records one-time ownership using the current immutable
// receipt as provenance. The mutating adoption operation shares the same
// injected command lock as apply, rollback, and checkpoint.
func NewAdoptCommand(receipts ReceiptCommandStore, adopter AdoptionPort, lockFactory MutationLockFactory) CommandOperations {
	return CommandOperationsFunc(func(_ context.Context, request CommandRequest, policy catalog.ResolvedHostPolicy) (result CommandResult, err error) {
		if receipts == nil || adopter == nil || lockFactory == nil || request.Command != CommandAdopt {
			return CommandResult{}, ErrCommandUnavailable
		}
		lock, err := lockFactory()
		if err != nil {
			return CommandResult{}, err
		}
		defer func() { err = errors.Join(err, lock.Release()) }()
		current, _, err := receipts.Current()
		if err != nil {
			return CommandResult{}, err
		}
		record, err := adopter.Adopt(request.Target, current.RunID)
		if err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Command: CommandAdopt, Host: policy.Name, Adoption: &AdoptionResult{Target: record.Target, Backup: record.Backup, FirstReceiptID: record.FirstReceiptID}}, nil
	})
}

// NewReceiptCommand exposes immutable receipt reads without catalog or host
// authority. An empty ID resolves the atomic current index; a non-empty ID
// resolves historical immutable evidence.
func NewReceiptCommand(store ReceiptCommandStore) CommandOperations {
	return CommandOperationsFunc(func(_ context.Context, request CommandRequest, _ catalog.ResolvedHostPolicy) (CommandResult, error) {
		if store == nil || (request.Command != CommandReceipt && request.Command != CommandStatus) {
			return CommandResult{}, ErrCommandUnavailable
		}
		var value receipt.Receipt
		var path string
		var err error
		if request.ReceiptID == "" {
			value, path, err = store.Current()
		} else {
			value, path, err = store.Read(request.ReceiptID)
		}
		if err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Command: request.Command, Host: value.Host.Resolved, Receipt: &value, ReceiptPath: path}, nil
	})
}

type TaggedCatalogSnapshot struct {
	Tag    string            `json:"tag"`
	Digest string            `json:"digest"`
	Files  map[string][]byte `json:"-"`
}

type TaggedCatalogReader interface {
	Read(context.Context, string) (TaggedCatalogSnapshot, error)
}

type TaggedCatalogReapplier interface {
	Reapply(context.Context, TaggedCatalogSnapshot, []string) (receipt.Receipt, error)
}

type RollbackResult struct {
	ReceiptID      string `json:"receiptId"`
	ReceiptPath    string `json:"receiptPath"`
	RollbackOf     string `json:"rollbackOf,omitempty"`
	CatalogTag     string `json:"catalogTag,omitempty"`
	InverseCount   int    `json:"inverseCount"`
	NoChange       bool   `json:"noChange"`
	SystemRollback string `json:"systemRollback"`
}

type RollbackCommandConfig struct {
	Receipts  ReceiptReader
	Publisher ReceiptPublisher
	Files     RollbackFilePort
	Tags      TaggedCatalogReader
	Reapplier TaggedCatalogReapplier
	Now       func() time.Time
	NewRunID  func(receipt.Receipt) string
	Lock      MutationLockFactory
}

func NewRollbackCommand(config RollbackCommandConfig) CommandOperations {
	return CommandOperationsFunc(func(ctx context.Context, request CommandRequest, _ catalog.ResolvedHostPolicy) (CommandResult, error) {
		if request.Command != CommandRollback || config.Publisher == nil || config.Now == nil || config.NewRunID == nil || config.Lock == nil {
			return CommandResult{}, ErrCommandUnavailable
		}
		if (request.ReceiptID == "") == (request.Tag == "") {
			return CommandResult{}, ErrInvalidRollbackTarget
		}
		if request.Tag != "" {
			return rollbackTag(ctx, request.Tag, request.Remove, config)
		}
		return rollbackReceipt(ctx, request.ReceiptID, config)
	})
}

func rollbackReceipt(ctx context.Context, id string, config RollbackCommandConfig) (result CommandResult, err error) {
	if config.Receipts == nil || config.Files == nil {
		return CommandResult{}, ErrCommandUnavailable
	}
	source, _, err := config.Receipts.Read(id)
	if err != nil {
		return CommandResult{}, err
	}
	lock, err := config.Lock()
	if err != nil {
		return CommandResult{}, err
	}
	defer func() { err = errors.Join(err, lock.Release()) }()
	files := append([]receipt.ManagedFile(nil), source.ManagedFiles...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	inverses := make([]RollbackInverse, 0, len(files))
	for _, file := range files {
		live, err := config.Files.Observe(ctx, file.Path)
		if err != nil {
			return CommandResult{}, err
		}
		class, err := receiptManagedClass(file)
		if err != nil {
			return CommandResult{}, err
		}
		inverse, err := PlanRollbackInverse(ManagedFileRollback{
			Class: class, Target: file.Path, AfterHash: file.AfterHash,
			BackupPath: file.Backup, BackupHash: file.BeforeHash, Live: live,
		})
		if err != nil {
			return CommandResult{}, err
		}
		inverses = append(inverses, inverse)
	}
	next, err := rollbackReceiptValue(source, config, id, "", allNoOp(inverses))
	if err != nil {
		return CommandResult{}, err
	}
	for _, inverse := range inverses {
		if inverse.Action == InverseNoOp {
			continue
		}
		if err := config.Files.Execute(ctx, inverse); err != nil {
			return CommandResult{}, err
		}
	}
	path, err := config.Publisher.Publish(next)
	if err != nil {
		return CommandResult{}, err
	}
	return rollbackCommandResult(next, path, id, "", len(inverses), len(source.SystemTransactions) != 0), nil
}

func rollbackTag(ctx context.Context, tag string, remove []string, config RollbackCommandConfig) (result CommandResult, err error) {
	if config.Tags == nil || config.Reapplier == nil {
		return CommandResult{}, ErrCommandUnavailable
	}
	snapshot, err := config.Tags.Read(ctx, tag)
	if err != nil {
		return CommandResult{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return CommandResult{}, err
	}
	lock, err := config.Lock()
	if err != nil {
		return CommandResult{}, err
	}
	defer func() { err = errors.Join(err, lock.Release()) }()
	base, err := config.Reapplier.Reapply(ctx, snapshot, append([]string(nil), remove...))
	if err != nil {
		return CommandResult{}, err
	}
	next, err := rollbackReceiptValue(base, config, "", snapshot.Tag, base.NoChange)
	if err != nil {
		return CommandResult{}, err
	}
	path, err := config.Publisher.Publish(next)
	if err != nil {
		return CommandResult{}, err
	}
	return rollbackCommandResult(next, path, "", snapshot.Tag, len(next.ManagedFiles), len(next.SystemTransactions) != 0), nil
}

func receiptManagedClass(file receipt.ManagedFile) (ManagedClass, error) {
	switch file.Ownership {
	case "created", "configurator":
		if file.Backup != "" {
			return ManagedAdopted, nil
		}
		return ManagedCreated, nil
	case "adopted":
		return ManagedAdopted, nil
	case "package-owned":
		return ManagedPackageOwned, nil
	default:
		return "", &RollbackConflict{Kind: ConflictInvalidClass, Target: file.Path}
	}
}

func rollbackReceiptValue(base receipt.Receipt, config RollbackCommandConfig, rollbackOf, tag string, noChange bool) (receipt.Receipt, error) {
	data, err := receipt.CanonicalJSON(base)
	if err != nil {
		return receipt.Receipt{}, err
	}
	next, err := receipt.Parse(data)
	if err != nil {
		return receipt.Receipt{}, err
	}
	now := config.Now().UTC().Format(time.RFC3339Nano)
	next.RunID = config.NewRunID(base)
	if next.RunID == "" || next.RunID == base.RunID {
		return receipt.Receipt{}, fmt.Errorf("%w: rollback requires a new receipt ID", receipt.ErrInvalid)
	}
	next.Command = "rollback"
	next.StartedAt, next.FinishedAt = now, now
	next.Status = "success"
	next.NoChange = noChange
	next.RollbackOf = rollbackOf
	next.ReappliedCatalogTag = tag
	if tag != "" {
		next.Catalog.Tag, next.Catalog.Release = tag, tag
	}
	next.Warnings, next.Errors = []string{}, []string{}
	if err := next.Validate(); err != nil {
		return receipt.Receipt{}, err
	}
	return next, nil
}

func allNoOp(values []RollbackInverse) bool {
	for _, value := range values {
		if value.Action != InverseNoOp {
			return false
		}
	}
	return true
}

func rollbackCommandResult(value receipt.Receipt, path, rollbackOf, tag string, count int, system bool) CommandResult {
	boundary := "not-required"
	if system {
		boundary = "snapper-delegated"
	}
	return CommandResult{Command: CommandRollback, Host: value.Host.Resolved, Receipt: &value, ReceiptPath: path, Rollback: &RollbackResult{
		ReceiptID: value.RunID, ReceiptPath: path, RollbackOf: rollbackOf, CatalogTag: tag,
		InverseCount: count, NoChange: value.NoChange, SystemRollback: boundary,
	}}
}

// ObjectTaggedCatalogReader reads only caller-declared catalog paths from a
// validated Git tag. It never checks out, resets, stashes, or writes the active
// worktree.
type ObjectTaggedCatalogReader struct {
	RepoRoot string
	Paths    []string
	Reader   interface {
		Show(context.Context, string, string, string) ([]byte, error)
	}
}

func (r ObjectTaggedCatalogReader) Read(ctx context.Context, tag string) (TaggedCatalogSnapshot, error) {
	if err := gitx.ValidateCatalogTag(tag); err != nil {
		return TaggedCatalogSnapshot{}, err
	}
	if r.Reader == nil || !filepath.IsAbs(r.RepoRoot) || len(r.Paths) == 0 {
		return TaggedCatalogSnapshot{}, ErrCommandUnavailable
	}
	paths := append([]string(nil), r.Paths...)
	sort.Strings(paths)
	files := make(map[string][]byte, len(paths))
	for _, path := range paths {
		data, err := r.Reader.Show(ctx, r.RepoRoot, tag, path)
		if err != nil {
			return TaggedCatalogSnapshot{}, err
		}
		files[path] = append([]byte(nil), data...)
	}
	return TaggedCatalogSnapshot{Tag: tag, Digest: taggedCatalogDigest(files), Files: files}, nil
}

type CheckpointValidation struct {
	CatalogCommitted bool `json:"catalogCommitted"`
	AssetsCommitted  bool `json:"assetsCommitted"`
}

type CheckpointValidator interface {
	Validate(context.Context) (CheckpointValidation, error)
}

type CommittedPathsValidator interface {
	ValidateCommittedPaths(context.Context, string, []string) error
}

// GitCheckpointValidator keeps production path authority injected. It proves
// both declared catalog and managed-asset paths against HEAD while ignoring
// unrelated dirty files.
type GitCheckpointValidator struct {
	Git          CommittedPathsValidator
	RepoRoot     string
	CatalogPaths []string
	AssetPaths   []string
}

func (v GitCheckpointValidator) Validate(ctx context.Context) (CheckpointValidation, error) {
	if v.Git == nil || !filepath.IsAbs(v.RepoRoot) || len(v.CatalogPaths) == 0 || len(v.AssetPaths) == 0 {
		return CheckpointValidation{}, ErrCommandUnavailable
	}
	if err := v.Git.ValidateCommittedPaths(ctx, v.RepoRoot, v.CatalogPaths); err != nil {
		return CheckpointValidation{}, fmt.Errorf("%w: catalog", ErrCheckpointValidation)
	}
	if err := v.Git.ValidateCommittedPaths(ctx, v.RepoRoot, v.AssetPaths); err != nil {
		return CheckpointValidation{CatalogCommitted: true}, fmt.Errorf("%w: managed assets", ErrCheckpointValidation)
	}
	return CheckpointValidation{CatalogCommitted: true, AssetsCommitted: true}, nil
}

type CheckpointTagger interface {
	CreateCheckpoint(context.Context, string, gitx.CheckpointSpec) (gitx.TagResult, error)
}

type CheckpointResult struct {
	Tag              string `json:"tag"`
	Commit           string `json:"commit"`
	CatalogCommitted bool   `json:"catalogCommitted"`
	AssetsCommitted  bool   `json:"assetsCommitted"`
	Annotated        bool   `json:"annotated"`
}

type CheckpointCommandConfig struct {
	Validator   CheckpointValidator
	Tagger      CheckpointTagger
	RepoRoot    string
	TaggerName  string
	TaggerEmail string
	Lock        MutationLockFactory
}

func NewCheckpointCommand(config CheckpointCommandConfig) CommandOperations {
	return CommandOperationsFunc(func(ctx context.Context, request CommandRequest, _ catalog.ResolvedHostPolicy) (result CommandResult, err error) {
		if request.Command != CommandCheckpoint || config.Validator == nil || config.Tagger == nil || config.Lock == nil || !filepath.IsAbs(config.RepoRoot) {
			return CommandResult{}, ErrCommandUnavailable
		}
		if err := gitx.ValidateCatalogTag(request.Tag); err != nil {
			return CommandResult{}, err
		}
		lock, err := config.Lock()
		if err != nil {
			return CommandResult{}, err
		}
		defer func() { err = errors.Join(err, lock.Release()) }()
		validation, err := config.Validator.Validate(ctx)
		if err != nil {
			return CommandResult{}, err
		}
		if !validation.CatalogCommitted || !validation.AssetsCommitted {
			return CommandResult{}, ErrCheckpointValidation
		}
		tagResult, err := config.Tagger.CreateCheckpoint(ctx, config.RepoRoot, gitx.CheckpointSpec{Name: request.Tag, Message: request.Message, TaggerName: config.TaggerName, TaggerEmail: config.TaggerEmail})
		if err != nil {
			return CommandResult{}, err
		}
		return CommandResult{Command: CommandCheckpoint, Checkpoint: &CheckpointResult{
			Tag: tagResult.Name, Commit: tagResult.Commit, CatalogCommitted: true, AssetsCommitted: true, Annotated: tagResult.DescribeVisible,
		}}, nil
	})
}

func (r TaggedCatalogSnapshot) Validate() error {
	if err := gitx.ValidateCatalogTag(r.Tag); err != nil || len(r.Files) == 0 || len(r.Digest) != sha256.Size*2 || r.Digest != taggedCatalogDigest(r.Files) {
		return fmt.Errorf("invalid tagged catalog snapshot")
	}
	return nil
}

func taggedCatalogDigest(files map[string][]byte) string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	digest := sha256.New()
	for _, path := range paths {
		digest.Write([]byte(path))
		digest.Write([]byte{0})
		digest.Write(files[path])
		digest.Write([]byte{0})
	}
	return hex.EncodeToString(digest.Sum(nil))
}
