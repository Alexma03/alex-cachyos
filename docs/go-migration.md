# Migration Guide: Bash to Go Configurator

This document describes the transition from the legacy Bash configurator (`./apply` and `modules/*.sh`) to the compiled, declarative Go configurator CLI (`alex-cachyos`).

## Overview

The Go configurator replaces imperative shell scripting with a declarative, typed architecture backed by a layered catalog, strict package pinning, and immutable audit receipts.

### Key Architectural Differences

| Feature | Legacy Bash Configurator | Go Configurator (`alex-cachyos`) |
| :--- | :--- | :--- |
| **Model** | Imperative shell script (`./apply`) | Declarative state machine |
| **Configuration** | JSON profile files (`profiles/`) | Layered YAML catalog (`catalog/`) |
| **Ordering** | Ad-hoc script execution order | Topological dependency-ordered plan |
| **Safety** | Best-effort script traps | Lock acquisition, preflight check, dry-run |
| **Traceability** | Plain terminal output | Cryptographically hashed receipts |
| **Rollback** | Manual backup restoration | Built-in receipt-driven rollback |

## Command Reference

### 1. Catalog Validation (`check`)
Validates catalog syntax, inheritance hierarchy, and package pin specifications without modifying the system or requiring root:
```bash
alex-cachyos check --host galaxy
```

### 2. Execution Planning (`plan`)
Generates the deterministic execution plan for the target host without performing changes:
```bash
alex-cachyos plan --host galaxy
```

### 3. Dry-Run Execution (`apply --dry-run`)
Executes the full pipeline against system state in read-only mode, validating preconditions and producing a dry-run receipt:
```bash
alex-cachyos apply --host galaxy --dry-run
```

### 4. Convergence (`apply`)
Applies the planned system changes idempotently:
```bash
alex-cachyos apply --host galaxy
```

### 5. Rollback (`rollback`)
Reverts modifications managed by a previous run's receipt:
```bash
alex-cachyos rollback --receipt <receipt-id>
```

## Catalog Structure

The Go configurator resolves desired machine state through layer inheritance:

```text
catalog/
├── global.yaml              # Base packages, common desktop, devtools
├── roles/
│   └── workstation.yaml     # Workstation-specific tools and GUI apps
└── hosts/
    └── galaxy.yaml          # Host-specific hardware (Samsung Galaxy Book)
```

Values cascade from `global` → `role` → `host`. Host-level settings take top precedence.

## Staged Migration and Safety Policy

1. **Non-Destructive Transition:** Legacy Bash scripts (`./apply`, `lib/`, `modules/`) are preserved in the repository alongside the Go configurator until live convergence is validated on the target machine.
2. **Deterministic Pins:** AUR and external Git dependencies require explicit 40-character commit hashes and integrity digests.
3. **Receipt Storage:** Run artifacts and receipts are stored under `$XDG_STATE_HOME/alex-cachyos/` (defaults to `~/.local/state/alex-cachyos/`).
