import { FormatRegistry, Type, type Static, type TSchema } from "@sinclair/typebox";
import { valid, validRange } from "semver";

if (!FormatRegistry.Has("semver")) {
  FormatRegistry.Set("semver", (value) => valid(value) !== null);
}

if (!FormatRegistry.Has("semver-range")) {
  FormatRegistry.Set("semver-range", (value) => validRange(value) !== null);
}

export const SemverSchema = Type.String({ format: "semver" });
export type Semver = Static<typeof SemverSchema>;

export const SemverRangeSchema = Type.String({ format: "semver-range" });
export type SemverRange = Static<typeof SemverRangeSchema>;

export const IsoDateTimeSchema = Type.String({
  pattern: "^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(?:\\.\\d+)?Z$"
});

export const JsonValueSchema = Type.Recursive((JsonValue) =>
  Type.Union([
    Type.Null(),
    Type.Boolean(),
    Type.Number(),
    Type.String(),
    Type.Array(JsonValue),
    Type.Record(Type.String(), JsonValue)
  ])
);
export type JsonValue = Static<typeof JsonValueSchema>;

export const JsonObjectSchema = Type.Record(Type.String(), JsonValueSchema);
export type JsonObject = Static<typeof JsonObjectSchema>;

export function CursorPageSchema<T extends TSchema>(item: T) {
  return Type.Object(
    {
      items: Type.Array(item),
      nextCursor: Type.Optional(Type.String({ minLength: 1 }))
    },
    { additionalProperties: false }
  );
}
