import { Type, type Static } from "@sinclair/typebox";

import { TraceIdSchema } from "../identifiers/index.js";

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
  Type.Literal("AGENT_EXECUTION_FAILED"),
  Type.Literal("DEPENDENCY_ERROR"),
  Type.Literal("TIMEOUT"),
  Type.Literal("CANCELED"),
  Type.Literal("INTERNAL_ERROR")
]);
export type PlatformErrorCode = Static<typeof PlatformErrorCodeSchema>;

export const PlatformErrorSchema = Type.Object(
  {
    code: PlatformErrorCodeSchema,
    message: Type.String({ minLength: 1 }),
    traceId: Type.Optional(TraceIdSchema)
  },
  { additionalProperties: false }
);
export type PlatformError = Static<typeof PlatformErrorSchema>;
