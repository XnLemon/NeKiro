import { Type, type Static, type TSchema } from "@sinclair/typebox";

export const IdentifierSchema = Type.String({ minLength: 1, maxLength: 128 });
export type Identifier = Static<typeof IdentifierSchema>;

export const SemverSchema = Type.String({
  pattern: "^(0|[1-9]\\d*)\\.(0|[1-9]\\d*)\\.(0|[1-9]\\d*)(?:-[0-9A-Za-z.-]+)?(?:\\+[0-9A-Za-z.-]+)?$"
});
export type Semver = Static<typeof SemverSchema>;

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

export const CallerSchema = Type.Object(
  {
    type: Type.Union([Type.Literal("user"), Type.Literal("agent"), Type.Literal("service")]),
    id: IdentifierSchema
  },
  { additionalProperties: false }
);
export type Caller = Static<typeof CallerSchema>;

export const InstallationStatusSchema = Type.Union([
  Type.Literal("enabled"),
  Type.Literal("disabled"),
  Type.Literal("uninstalled")
]);
export type InstallationStatus = Static<typeof InstallationStatusSchema>;

export const InvocationStatusSchema = Type.Union([
  Type.Literal("pending"),
  Type.Literal("routing"),
  Type.Literal("running"),
  Type.Literal("succeeded"),
  Type.Literal("failed"),
  Type.Literal("canceled"),
  Type.Literal("timed_out")
]);
export type InvocationStatus = Static<typeof InvocationStatusSchema>;

export const TaskStatusSchema = Type.Union([
  Type.Literal("submitted"),
  Type.Literal("working"),
  Type.Literal("input_required"),
  Type.Literal("auth_required"),
  Type.Literal("completed"),
  Type.Literal("failed"),
  Type.Literal("canceled"),
  Type.Literal("rejected")
]);
export type TaskStatus = Static<typeof TaskStatusSchema>;

export const PlatformErrorCodeSchema = Type.Union([
  Type.Literal("VALIDATION_ERROR"),
  Type.Literal("UNAUTHENTICATED"),
  Type.Literal("FORBIDDEN"),
  Type.Literal("NOT_FOUND"),
  Type.Literal("CONFLICT"),
  Type.Literal("AGENT_NOT_INSTALLED"),
  Type.Literal("AGENT_DISABLED"),
  Type.Literal("CAPABILITY_NOT_ALLOWED"),
  Type.Literal("ROUTE_NOT_FOUND"),
  Type.Literal("A2A_PROTOCOL_ERROR"),
  Type.Literal("AGENT_UNAVAILABLE"),
  Type.Literal("TIMEOUT"),
  Type.Literal("CANCELED"),
  Type.Literal("INTERNAL_ERROR")
]);
export type PlatformErrorCode = Static<typeof PlatformErrorCodeSchema>;

export const PlatformErrorSchema = Type.Object(
  {
    code: PlatformErrorCodeSchema,
    message: Type.String({ minLength: 1 }),
    traceId: Type.Optional(IdentifierSchema),
    details: Type.Optional(JsonObjectSchema)
  },
  { additionalProperties: false }
);
export type PlatformError = Static<typeof PlatformErrorSchema>;

export function CursorPageSchema<T extends TSchema>(item: T) {
  return Type.Object(
    {
      items: Type.Array(item),
      nextCursor: Type.Optional(Type.String({ minLength: 1 }))
    },
    { additionalProperties: false }
  );
}
