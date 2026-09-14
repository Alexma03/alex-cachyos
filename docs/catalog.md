# Declarative Catalog Guide

The `alex-cachyos` Go configurator uses a versioned, declarative catalog to define desired machine states.

## Inheritance and Precedence

The catalog resolves configuration layers in the following order:

```text
Global (catalog/global.yaml)
  ↳ Role (catalog/roles/<role>.yaml)
      ↳ Host (catalog/hosts/<host>.yaml)
```

- **Scalars & Objects**: Host values override role values; role values override global values.
- **Module activation**: Explicit booleans (`bootstrap`, `fingerprint`, `devtools`, `apps`, `vicinae`, `desktop`, `verify`) merge by key name.
- **Ordered collections**: Explicitly replaced or augmented by layer.

## Pinning Contract

To guarantee deterministic, reproducible convergence:
- **AUR / Local packages**: Pinned to exact 40-character hexadecimal git commit and SHA-256 patch digest.
- **Pacman repository packages**: Explicit transaction policy and version presence.
- **Remote artifacts**: Immutable HTTPS URLs with verified SHA-256 digests.

## Verification

The catalog is validated before any execution plan is generated:
```bash
alex-cachyos check
```
Purity: catalog validation performs no filesystem mutations and resolves hosts without reading sensitive hardware identifiers.
