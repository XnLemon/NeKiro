# Invocation Correlation Semantic Rules v1

## Normative Language

The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**,
**SHOULD**, **SHOULD NOT**, **RECOMMENDED**, **NOT RECOMMENDED**, **MAY**, and
**OPTIONAL** in this document are to be interpreted as described in BCP 14,
RFC 2119 and RFC 8174, when, and only when, they appear in all capitals.

## Conformance

An Invocation Event v0.2 or Invocation Result Stream Event v1 document MUST
first pass its versioned JSON Schema. A conforming implementation MUST then
apply every semantic rule in this document. Structural validity alone MUST NOT
be reported as Invocation correlation conformance.

Identifier comparisons MUST use exact, case-sensitive JSON string equality.
Implementations MUST NOT trim, normalize, case-fold, coerce, omit, replace, or
synthesize correlation identifiers.

## Rules

### INV-CORR-001: Nested Platform Error Correlation

When an Invocation Event v0.2 or Invocation Result Stream Event v1 contains a
nested Platform Error v2, the nested error's `invocationId`, `rootTaskId`, and
`traceId` values MUST exactly equal the corresponding values on the enclosing
event.

A mismatch in any one or more of these identifiers violates `INV-CORR-001`.
The rule does not apply when the event contract forbids or omits `error`.

## Portable Corpus

`conformance/manifest.json` lists the normative raw JSON cases. Each conforming
implementation MUST produce the manifest's `expectedValid` decision and exact
`violatedRules` set for every case. Error wording and evaluation order are not
normative.

The manifest's `schemaVersion` and `cases` fields are required and non-null.
Every case requires exactly one `id`, `contractKind`, `file`, `expectedValid`,
and `violatedRules` member. Unknown or duplicate JSON members are invalid.

Case IDs and fixture files MUST be unique. Fixture paths MUST be non-empty,
canonical `/`-separated relative paths confined to the conformance corpus.
Absolute paths, URI schemes, backslashes, empty segments, `.` or `..` segments,
control characters, and platform-equivalent or reserved path forms are invalid
before fixture access. A valid case declares no violated rules. An invalid case
declares exactly `INV-CORR-001`. No compatibility manifest or alternate decoder
is defined.
