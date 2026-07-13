# Contract Design: Agent Card Semantic Conformance

## Contract Composition

Agent Card `0.2` conformance requires both:

1. Structural validation against
   `contracts/schemas/agent-card.v0.2.schema.json`.
2. Semantic validation against
   `contracts/agent-card/v0.2/semantic-rules.md`.

Passing only one layer is invalid.

The structural Schema also rejects `protocol.endpoint` URI userinfo. A username,
password, token-like userinfo, or empty userinfo marker is credential-bearing
Card data and is invalid even when the remainder is a valid HTTP(S) URI.

## Normative Rules

- `AC-SEM-001`: every `skills[*].id` MUST be unique within one Card.
- `AC-SEM-002`: every `permissions[*].id` MUST be unique within one Card.
- `AC-SEM-003`: every `skills[*].requiredPermissions[*]` MUST exactly match a
  `permissions[*].id` declared in the same Card version.

Comparison is case-sensitive JSON string equality. A declaration in another
Agent Card version cannot satisfy a reference.

## Portable Conformance Corpus

Directory: `contracts/agent-card/v0.2/conformance/`

Minimum raw fixtures:

- valid baseline;
- valid multiple skills sharing one declared permission;
- invalid duplicate skill ID on otherwise distinct skill objects;
- invalid duplicate permission ID on otherwise distinct declarations;
- invalid undeclared required permission;
- invalid permission declared only in another Card version;
- invalid endpoint containing URI userinfo.

`manifest.json` records stable case ID, fixture path, required related-context
fixture paths, expected validity, and violated rule IDs. Every field is present;
`contextFiles` and `violatedRules` use explicit empty arrays when empty.
Fixtures are authored independently of Go marshaling.

Manifest JSON rejects duplicate member names. Fixture paths are canonical `/`
separated relative paths confined beneath the conformance directory; absolute
paths, URI-like paths, backslashes, empty segments, and `.`/`..` traversal are
invalid before filesystem access.

## Go Mapping

Go retains structural-then-semantic validation. Semantic errors expose stable
rule IDs for conformance tests; wording and evaluation order are not public
contract. The Go implementation does not parse Markdown or execute a new rules
DSL.

The duplicate-member scanner is shared with strict Invocation DTO decoding. Its
syntax walk preserves JSON number tokens without applying a bounded native
numeric range; Agent Card field types and Schema remain responsible for any
Card-specific numeric constraints. This parser invariant does not change the
Agent Card semantic rule catalog.

## Migration

The stricter semantic contract is Agent Card `0.2`. Version `0.1` remains
historical and is not silently amended. The first Registry implementation
accepts only active `0.2`, because no published Registry data exists to justify
a dual-version compatibility path.
