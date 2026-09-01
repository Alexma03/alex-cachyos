package cli

import (
	"bytes"
	"errors"
	"testing"

	"alex-cachyos/internal/app"
	"alex-cachyos/internal/gitx"
	"alex-cachyos/internal/receipt"
)

func TestRenderCommandResultUsesTypedRollbackAndCheckpointStrings(t *testing.T) {
	var output bytes.Buffer
	value := receipt.Receipt{RunID: "rollback-run"}
	rollback := app.CommandResult{Command: app.CommandRollback, Receipt: &value, Rollback: &app.RollbackResult{
		ReceiptID: "rollback-run", RollbackOf: "source-run", InverseCount: 2, SystemRollback: "snapper-delegated",
	}}
	if err := RenderCommandResult(false, rollback, &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "rollback rollback-run rollback-of=source-run inverses=2 system=snapper-delegated\n" {
		t.Fatalf("rollback output = %q", output.String())
	}
	output.Reset()
	checkpoint := app.CommandResult{Command: app.CommandCheckpoint, Checkpoint: &app.CheckpointResult{Tag: "catalog-v1.2.3", Commit: "0123456789abcdef0123456789abcdef01234567", Annotated: true}}
	if err := RenderCommandResult(false, checkpoint, &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "checkpoint catalog-v1.2.3 0123456789abcdef0123456789abcdef01234567\n" {
		t.Fatalf("checkpoint output = %q", output.String())
	}
	output.Reset()
	adoption := app.CommandResult{Command: app.CommandAdopt, Adoption: &app.AdoptionResult{Target: "/etc/example", Backup: "/etc/example.bak.alex-cachyos", FirstReceiptID: "run-1"}}
	if err := RenderCommandResult(false, adoption, &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "adopted /etc/example backup=/etc/example.bak.alex-cachyos receipt=run-1\n" {
		t.Fatalf("adoption output = %q", output.String())
	}
}

func TestRenderCommandErrorMapsTypedCommandFailuresWithoutLeakingDetails(t *testing.T) {
	tests := []struct {
		name, wantCode, wantMessage string
		err                         error
		wantExit                    int
	}{
		{"receipt missing", "receipt-not-found", "receipt not found", receipt.ErrReceiptNotFound, 1},
		{"receipt ambiguous", "receipt-ambiguous", "receipt ID is ambiguous", receipt.ErrReceiptAmbiguous, 1},
		{"rollback conflict", "rollback-conflict", "rollback precondition conflict", &app.RollbackConflict{Kind: app.ConflictLiveMismatch, Target: "/private/target"}, 1},
		{"checkpoint", "checkpoint-invalid", "catalog or managed assets are not committed at HEAD", app.ErrCheckpointValidation, 1},
		{"tag", "invalid-catalog-tag", "catalog tag must match catalog-vX.Y.Z", gitx.ErrInvalidCatalogTag, ExitUsage},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := RenderCommandError(true, tc.err, &stdout, &stderr); got != tc.wantExit {
				t.Fatalf("exit = %d, want %d", got, tc.wantExit)
			}
			want := `{"error":{"code":"` + tc.wantCode + `","message":"` + tc.wantMessage + `"}}` + "\n"
			if stdout.String() != want || stderr.Len() != 0 {
				t.Fatalf("output = %q stderr=%q want=%q", stdout.String(), stderr.String(), want)
			}
			if errors.Is(tc.err, app.ErrRollbackConflict) && bytes.Contains(stdout.Bytes(), []byte("/private")) {
				t.Fatal("rollback target leaked")
			}
		})
	}
}
