package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"alex-cachyos/internal/adopt"
	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/gitx"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/receipt"
)

type rollbackReceiptStore struct {
	value       receipt.Receipt
	readID      string
	published   []receipt.Receipt
	publishPath string
}

func (s *rollbackReceiptStore) Read(id string) (receipt.Receipt, string, error) {
	s.readID = id
	data, err := receipt.CanonicalJSON(s.value)
	if err != nil {
		return receipt.Receipt{}, "", err
	}
	value, err := receipt.Parse(data)
	return value, "/state/source.json", err
}
func (s *rollbackReceiptStore) Publish(value receipt.Receipt) (string, error) {
	s.published = append(s.published, value)
	return s.publishPath, nil
}
func (s *rollbackReceiptStore) Current() (receipt.Receipt, string, error) {
	data, err := receipt.CanonicalJSON(s.value)
	if err != nil {
		return receipt.Receipt{}, "", err
	}
	value, err := receipt.Parse(data)
	return value, "/state/current.json", err
}

type rollbackFilesSpy struct {
	live       map[string]LiveFile
	observed   []string
	executed   []RollbackInverse
	observeErr map[string]error
}

func (s *rollbackFilesSpy) Observe(_ context.Context, path string) (LiveFile, error) {
	s.observed = append(s.observed, path)
	return s.live[path], s.observeErr[path]
}
func (s *rollbackFilesSpy) Execute(_ context.Context, inverse RollbackInverse) error {
	s.executed = append(s.executed, inverse)
	return nil
}

type mutationLockStub struct{ released int }

func (l *mutationLockStub) Release() error { l.released++; return nil }
func lockFactory(lock *mutationLockStub) MutationLockFactory {
	return func() (MutationLock, error) { return lock, nil }
}

type applyFactorySpy struct {
	request CommandRequest
	policy  catalog.ResolvedHostPolicy
}

func (s *applyFactorySpy) Build(_ context.Context, request CommandRequest, policy catalog.ResolvedHostPolicy) (ApplyInput, error) {
	s.request, s.policy = request, policy
	return ApplyInput{}, nil
}

type applyRunnerSpy struct {
	input ApplyInput
	value receipt.Receipt
}

func (s *applyRunnerSpy) Apply(_ context.Context, input ApplyInput) (ApplyResult, error) {
	s.input = input
	return ApplyResult{Receipt: s.value, ReceiptPath: "/state/apply.json"}, nil
}

func TestApplyCommandComposesResolvedPolicyAndParsedSelectionWithoutProductionCatalog(t *testing.T) {
	factory := &applyFactorySpy{}
	runner := &applyRunnerSpy{value: commandTestReceipt(t)}
	handler := NewApplyCommand(factory, runner)
	request := CommandRequest{Command: CommandApply, DryRun: true, Selection: planner.Selection{Only: []string{"desktop"}}, Remove: []string{"apps"}}
	policy := catalog.ResolvedHostPolicy{Name: "fixture-host", Roles: []string{"workstation"}}
	got, err := handler.Execute(context.Background(), request, policy)
	if err != nil {
		t.Fatal(err)
	}
	if factory.policy.Name != "fixture-host" || !reflect.DeepEqual(factory.request.Selection, request.Selection) || !reflect.DeepEqual(factory.request.Remove, request.Remove) || !runner.input.DryRun || got.ReceiptPath != "/state/apply.json" {
		t.Fatalf("factory=%#v runner=%#v result=%#v", factory, runner.input, got)
	}
}

type adopterSpy struct {
	target, receiptID string
}

func (s *adopterSpy) Adopt(target, receiptID string) (adopt.Record, error) {
	s.target, s.receiptID = target, receiptID
	return adopt.Record{Target: target, Backup: target + adopt.BackupSuffix, FirstReceiptID: receiptID}, nil
}

func TestAdoptCommandUsesCurrentReceiptProvenanceUnderMutationLock(t *testing.T) {
	store := &rollbackReceiptStore{value: commandTestReceipt(t)}
	store.value.RunID = "first-run"
	adopter := &adopterSpy{}
	lock := &mutationLockStub{}
	got, err := NewAdoptCommand(store, adopter, lockFactory(lock)).Execute(context.Background(), CommandRequest{Command: CommandAdopt, Target: "/etc/example"}, catalog.ResolvedHostPolicy{Name: "fixture-host"})
	if err != nil {
		t.Fatal(err)
	}
	if lock.released != 1 || adopter.receiptID != "first-run" || got.Adoption == nil || got.Adoption.Backup != "/etc/example"+adopt.BackupSuffix {
		t.Fatalf("adopter=%#v result=%#v released=%d", adopter, got, lock.released)
	}
}

func TestRollbackReceiptPlansEveryPreconditionBeforeMutationAndPublishesNewReceipt(t *testing.T) {
	source := commandTestReceipt(t)
	createdHash := sha256Hex("created")
	adoptedHash := sha256Hex("managed")
	backupHash := sha256Hex("original")
	source.RunID = "source-run"
	source.ManagedFiles = []receipt.ManagedFile{
		{Path: "/etc/z-created", BeforeHash: sha256Hex("absent"), AfterHash: createdHash, Mode: "0644", Ownership: "created"},
		{Path: "/etc/a-adopted", BeforeHash: backupHash, AfterHash: adoptedHash, Mode: "0644", Backup: "/etc/a-adopted.bak.alex-cachyos", Ownership: "adopted"},
	}
	before, err := receipt.CanonicalJSON(source)
	if err != nil {
		t.Fatal(err)
	}
	store := &rollbackReceiptStore{value: source, publishPath: "/state/new.json"}
	lock := &mutationLockStub{}
	files := &rollbackFilesSpy{live: map[string]LiveFile{
		"/etc/a-adopted": {Exists: true, Hash: adoptedHash},
		"/etc/z-created": {Exists: true, Hash: createdHash},
	}, observeErr: map[string]error{}}
	handler := NewRollbackCommand(RollbackCommandConfig{
		Receipts: store, Publisher: store, Files: files,
		Now:      func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) },
		NewRunID: func(receipt.Receipt) string { return "rollback-run" },
		Lock:     lockFactory(lock),
	})
	got, err := handler.Execute(context.Background(), CommandRequest{Command: CommandRollback, ReceiptID: "source-run"}, catalog.ResolvedHostPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(files.observed, []string{"/etc/a-adopted", "/etc/z-created"}) || lock.released != 1 {
		t.Fatalf("observations = %#v", files.observed)
	}
	if len(files.executed) != 2 || files.executed[0].Action != InverseRestoreBackup || files.executed[1].Action != InverseRemoveFile {
		t.Fatalf("executed = %#v", files.executed)
	}
	if len(store.published) != 1 || store.published[0].RunID != "rollback-run" || store.published[0].RollbackOf != "source-run" || store.published[0].Command != "rollback" {
		t.Fatalf("published = %#v", store.published)
	}
	if got.Rollback == nil || got.Rollback.SystemRollback != "snapper-delegated" || got.Rollback.ReceiptPath != "/state/new.json" {
		t.Fatalf("result = %#v", got)
	}
	after, err := receipt.CanonicalJSON(source)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source receipt was mutated")
	}
}

func TestRollbackReceiptRefusesAllMutationWhenAnyPreconditionFails(t *testing.T) {
	source := commandTestReceipt(t)
	source.RunID = "source-run"
	source.ManagedFiles = []receipt.ManagedFile{
		{Path: "/etc/a", BeforeHash: sha256Hex("none"), AfterHash: sha256Hex("a"), Mode: "0644", Ownership: "created"},
		{Path: "/etc/b", BeforeHash: sha256Hex("none"), AfterHash: sha256Hex("b"), Mode: "0644", Ownership: "created"},
	}
	store := &rollbackReceiptStore{value: source}
	lock := &mutationLockStub{}
	files := &rollbackFilesSpy{live: map[string]LiveFile{
		"/etc/a": {Exists: true, Hash: sha256Hex("a")},
		"/etc/b": {Exists: true, Hash: sha256Hex("edited")},
	}, observeErr: map[string]error{}}
	handler := NewRollbackCommand(RollbackCommandConfig{Receipts: store, Publisher: store, Files: files, Now: time.Now, NewRunID: func(receipt.Receipt) string { return "unused" }, Lock: lockFactory(lock)})
	_, err := handler.Execute(context.Background(), CommandRequest{Command: CommandRollback, ReceiptID: "source-run"}, catalog.ResolvedHostPolicy{})
	if !errors.Is(err, ErrRollbackConflict) || len(files.executed) != 0 || len(store.published) != 0 || lock.released != 1 {
		t.Fatalf("error=%v executed=%#v published=%#v", err, files.executed, store.published)
	}
}

func TestRollbackReceiptValidatesNewReceiptIdentityBeforeMutation(t *testing.T) {
	source := commandTestReceipt(t)
	source.RunID = "source-run"
	source.ManagedFiles = []receipt.ManagedFile{{Path: "/etc/a", BeforeHash: sha256Hex("none"), AfterHash: sha256Hex("a"), Mode: "0644", Ownership: "created"}}
	store := &rollbackReceiptStore{value: source}
	files := &rollbackFilesSpy{live: map[string]LiveFile{"/etc/a": {Exists: true, Hash: sha256Hex("a")}}, observeErr: map[string]error{}}
	lock := &mutationLockStub{}
	handler := NewRollbackCommand(RollbackCommandConfig{Receipts: store, Publisher: store, Files: files, Now: time.Now, NewRunID: func(receipt.Receipt) string { return "source-run" }, Lock: lockFactory(lock)})
	_, err := handler.Execute(context.Background(), CommandRequest{Command: CommandRollback, ReceiptID: "source-run"}, catalog.ResolvedHostPolicy{})
	if !errors.Is(err, receipt.ErrInvalid) || len(files.executed) != 0 || len(store.published) != 0 {
		t.Fatalf("error=%v executed=%#v published=%#v", err, files.executed, store.published)
	}
}

type objectReaderSpy struct {
	calls [][3]string
	files map[string][]byte
}

func (s *objectReaderSpy) Show(_ context.Context, cwd, revision, path string) ([]byte, error) {
	s.calls = append(s.calls, [3]string{cwd, revision, path})
	return append([]byte(nil), s.files[path]...), nil
}

func TestObjectTaggedCatalogReaderUsesOnlySortedGitObjectReads(t *testing.T) {
	repo := t.TempDir()
	reader := &objectReaderSpy{files: map[string][]byte{"catalog/global.yaml": []byte("global"), "catalog/hosts/portable.yaml": []byte("host")}}
	snapshot, err := (ObjectTaggedCatalogReader{RepoRoot: repo, Paths: []string{"catalog/hosts/portable.yaml", "catalog/global.yaml"}, Reader: reader}).Read(context.Background(), "catalog-v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := [][3]string{{repo, "catalog-v1.2.3", "catalog/global.yaml"}, {repo, "catalog-v1.2.3", "catalog/hosts/portable.yaml"}}
	if !reflect.DeepEqual(reader.calls, wantCalls) || snapshot.Tag != "catalog-v1.2.3" || len(snapshot.Digest) != 64 {
		t.Fatalf("snapshot=%#v calls=%#v", snapshot, reader.calls)
	}
	if _, err := (ObjectTaggedCatalogReader{RepoRoot: repo, Paths: []string{"catalog/global.yaml"}, Reader: reader}).Read(context.Background(), "../main"); !errors.Is(err, gitx.ErrInvalidCatalogTag) {
		t.Fatalf("unsafe tag error = %v", err)
	}
}

type taggedReaderStub struct{ snapshot TaggedCatalogSnapshot }

func (s taggedReaderStub) Read(context.Context, string) (TaggedCatalogSnapshot, error) {
	return s.snapshot, nil
}

type taggedReapplierStub struct {
	value  receipt.Receipt
	calls  int
	remove []string
}

func (s *taggedReapplierStub) Reapply(_ context.Context, snapshot TaggedCatalogSnapshot, remove []string) (receipt.Receipt, error) {
	s.calls++
	s.remove = append([]string(nil), remove...)
	return s.value, snapshot.Validate()
}

func TestRollbackTagReappliesValidatedSnapshotAndRecordsTagInNewReceipt(t *testing.T) {
	store := &rollbackReceiptStore{publishPath: "/state/tag-rollback.json"}
	lock := &mutationLockStub{}
	reapplier := &taggedReapplierStub{value: commandTestReceipt(t)}
	files := map[string][]byte{"catalog/global.yaml": []byte("global")}
	handler := NewRollbackCommand(RollbackCommandConfig{
		Publisher: store, Tags: taggedReaderStub{snapshot: TaggedCatalogSnapshot{Tag: "catalog-v1.2.3", Digest: taggedCatalogDigest(files), Files: files}}, Reapplier: reapplier,
		Now: func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) }, NewRunID: func(receipt.Receipt) string { return "tag-rollback" },
		Lock: lockFactory(lock),
	})
	got, err := handler.Execute(context.Background(), CommandRequest{Command: CommandRollback, Tag: "catalog-v1.2.3", Remove: []string{"desktop"}}, catalog.ResolvedHostPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if reapplier.calls != 1 || !reflect.DeepEqual(reapplier.remove, []string{"desktop"}) || lock.released != 1 || len(store.published) != 1 || store.published[0].ReappliedCatalogTag != "catalog-v1.2.3" || got.Rollback == nil || got.Rollback.CatalogTag != "catalog-v1.2.3" {
		t.Fatalf("got=%#v published=%#v reapplies=%d", got, store.published, reapplier.calls)
	}
}

type checkpointValidatorStub struct {
	value CheckpointValidation
	err   error
}

func (s checkpointValidatorStub) Validate(context.Context) (CheckpointValidation, error) {
	return s.value, s.err
}

type checkpointTaggerSpy struct {
	calls int
	spec  gitx.CheckpointSpec
}

func (s *checkpointTaggerSpy) CreateCheckpoint(_ context.Context, _ string, spec gitx.CheckpointSpec) (gitx.TagResult, error) {
	s.calls++
	s.spec = spec
	return gitx.TagResult{Name: spec.Name, Commit: "0123456789abcdef0123456789abcdef01234567", DescribeVisible: true}, nil
}

func TestCheckpointValidatesCommittedCatalogAndAssetsBeforeAnnotatedTag(t *testing.T) {
	tagger := &checkpointTaggerSpy{}
	lock := &mutationLockStub{}
	config := CheckpointCommandConfig{Validator: checkpointValidatorStub{value: CheckpointValidation{CatalogCommitted: true, AssetsCommitted: false}}, Tagger: tagger, RepoRoot: t.TempDir(), TaggerName: "Test", TaggerEmail: "test@example.invalid", Lock: lockFactory(lock)}
	request := CommandRequest{Command: CommandCheckpoint, Tag: "catalog-v2.0.0", Message: "release"}
	if _, err := NewCheckpointCommand(config).Execute(context.Background(), request, catalog.ResolvedHostPolicy{}); !errors.Is(err, ErrCheckpointValidation) || tagger.calls != 0 || lock.released != 1 {
		t.Fatalf("invalid checkpoint error=%v calls=%d", err, tagger.calls)
	}
	config.Validator = checkpointValidatorStub{value: CheckpointValidation{CatalogCommitted: true, AssetsCommitted: true}}
	got, err := NewCheckpointCommand(config).Execute(context.Background(), request, catalog.ResolvedHostPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if tagger.calls != 1 || lock.released != 2 || tagger.spec.Name != request.Tag || got.Checkpoint == nil || !got.Checkpoint.Annotated {
		t.Fatalf("got=%#v calls=%d spec=%#v", got, tagger.calls, tagger.spec)
	}
}

func commandTestReceipt(t *testing.T) receipt.Receipt {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "receipts", "golden-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := receipt.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
