# Agent Card v0.2 Semantic Rules

## Normative Language

The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**,
**SHOULD**, **SHOULD NOT**, **RECOMMENDED**, **NOT RECOMMENDED**, **MAY**, and
**OPTIONAL** in this document are to be interpreted as described in BCP 14,
RFC 2119 and RFC 8174, when, and only when, they appear in all capitals.

## Conformance

An Agent Card v0.2 document MUST pass both structural validation against
`contracts/schemas/agent-card.v0.2.schema.json` and every semantic rule in this
document. Passing structural validation alone MUST NOT be reported as Agent
Card v0.2 conformance.

`protocol.endpoint` MUST be an absolute HTTP(S) URI without URI userinfo. The
structural Schema MUST reject username, password, token-like, and empty
userinfo marker forms before semantic rules are evaluated.

Semantic evaluation operates on one complete Agent Card document. Identifier
comparisons MUST use exact, case-sensitive JSON string equality. Implementations
MUST NOT trim, normalize, case-fold, coerce, or resolve identifiers from another
Agent Card document or Agent version.

## Rules

### AC-SEM-001: Unique Skill Identifiers

Every `skills[*].id` value MUST be unique within the Agent Card. Distinct skill
objects that contain the same `id` violate this rule.

### AC-SEM-002: Unique Permission Identifiers

Every `permissions[*].id` value MUST be unique within the Agent Card. Distinct
permission declarations that contain the same `id` violate this rule.

### AC-SEM-003: Declared Required Permissions

Every `skills[*].requiredPermissions[*]` value MUST match a
`permissions[*].id` declared in the same Agent Card document. A permission
declared only by another version of the Agent Card does not satisfy this rule.

## Portable Corpus

`conformance/manifest.json` lists the normative raw JSON cases for these rules.
Each conforming implementation MUST produce the manifest's combined validity
decision and violated rule IDs. Rule evaluation order, error wording, and
implementation-specific object paths are not normative.

Every manifest case explicitly declares `contextFiles`. Context fixtures MUST
be valid Agent Card v0.2 documents, but they MUST NOT participate in semantic
evaluation of the primary `file`. They exist to demonstrate facts such as a
permission being declared by another Agent Card version. For validator output,
only `valid` and `violatedRules` are normative.

Every manifest object member name MUST occur exactly once. The `file` and every
`contextFiles` entry MUST be a nonempty canonical `/`-separated relative path
confined to the conformance corpus. Absolute paths, URI schemes, backslashes,
empty segments, `.` or `..` segments, and encoded or platform-equivalent
traversal forms are invalid before fixture filesystem access.
