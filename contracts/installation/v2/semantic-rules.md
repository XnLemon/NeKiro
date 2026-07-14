# Installation v2 Semantic Rules

The Installation v2 JSON Schema defines structural shape. A conforming
Installation v2 value MUST also satisfy every rule below.

## INST-SEM-001: Canonical Permission Order

`acceptedPermissions` MUST contain unique permission identifiers in ascending
bytewise order. Identifier comparison is exact and case-sensitive. Empty arrays
are valid.

## INST-SEM-002: Monotonic Update Time

`installedAt` MUST be less than or equal to `updatedAt` as RFC 3339 instants.

## INST-SEM-003: Terminal Timestamp Coherence

When `status` is `uninstalled`, `uninstalledAt` MUST be present and equal to
`updatedAt` as an RFC 3339 instant. When `status` is `enabled` or `disabled`,
`uninstalledAt` MUST be absent.

## INST-SEM-004: Immutable Pin Shape

`versionConstraint`, `installedVersion`, and `acceptedPermissions` are the
installation-time facts. Lifecycle responses MUST NOT change them.

Implementations MUST report semantic violations as validation failures. They
MUST NOT sort, trim, coerce, or fill values while validating a received
Installation response.

## INST-SEM-005: Pinned Version Satisfies Constraint

`installedVersion` MUST be a strict SemVer version accepted by the submitted
`versionConstraint`, including the constraint's normal pre-release rules. A
structurally valid Installation with an incompatible exact pin is not
conformant.
