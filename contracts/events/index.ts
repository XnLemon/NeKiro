import { Type, type Static } from "@sinclair/typebox";

import {
  CallerSchema,
  IdentifierSchema,
  IsoDateTimeSchema,
  InvocationStatusSchema,
  JsonObjectSchema,
  PlatformErrorSchema,
  SemverSchema
} from "../common/index.js";

export const InvocationEventTypeSchema = Type.Union([
  Type.Literal("created"),
  Type.Literal("routing"),
  Type.Literal("started"),
  Type.Literal("stream"),
  Type.Literal("succeeded"),
  Type.Literal("failed"),
  Type.Literal("canceled"),
  Type.Literal("timed_out")
]);
export type InvocationEventType = Static<typeof InvocationEventTypeSchema>;

export const InvocationEventSchema = Type.Object(
  {
    eventId: IdentifierSchema,
    sequence: Type.Integer({ minimum: 0 }),
    occurredAt: IsoDateTimeSchema,
    type: InvocationEventTypeSchema,
    status: InvocationStatusSchema,
    invocationId: IdentifierSchema,
    rootTaskId: IdentifierSchema,
    parentInvocationId: Type.Optional(IdentifierSchema),
    traceId: IdentifierSchema,
    caller: CallerSchema,
    workspaceId: IdentifierSchema,
    targetAgentId: IdentifierSchema,
    agentCardVersion: SemverSchema,
    capability: IdentifierSchema,
    payload: Type.Optional(JsonObjectSchema),
    error: Type.Optional(PlatformErrorSchema)
  },
  { additionalProperties: false }
);
export type InvocationEvent = Static<typeof InvocationEventSchema>;

export const InvocationRecordSchema = Type.Object(
  {
    invocationId: IdentifierSchema,
    rootTaskId: IdentifierSchema,
    parentInvocationId: Type.Optional(IdentifierSchema),
    traceId: IdentifierSchema,
    caller: CallerSchema,
    workspaceId: IdentifierSchema,
    targetAgentId: IdentifierSchema,
    agentCardVersion: SemverSchema,
    capability: IdentifierSchema,
    status: InvocationStatusSchema,
    latencyMs: Type.Optional(Type.Integer({ minimum: 0 })),
    errorCode: Type.Optional(Type.String({ minLength: 1 })),
    createdAt: IsoDateTimeSchema,
    updatedAt: IsoDateTimeSchema
  },
  { additionalProperties: false }
);
export type InvocationRecord = Static<typeof InvocationRecordSchema>;
