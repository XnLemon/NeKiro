import { Type, type Static } from "@sinclair/typebox";

import { IsoDateTimeSchema, SemverSchema } from "../common/index.js";
import { PlatformErrorCodeSchema, PlatformErrorSchema } from "../errors/index.js";
import {
  AgentIdSchema,
  CallerIdSchema,
  CapabilityIdSchema,
  EventIdSchema,
  InvocationIdSchema,
  TaskIdSchema,
  TraceIdSchema,
  WorkspaceIdSchema
} from "../identifiers/index.js";

export const InvocationEventSchemaVersion = "0.1" as const;

export const CallerSchema = Type.Object(
  {
    type: Type.Union([Type.Literal("user"), Type.Literal("agent"), Type.Literal("service")]),
    id: CallerIdSchema
  },
  { additionalProperties: false }
);
export type Caller = Static<typeof CallerSchema>;

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

const InvocationEventBaseSchema = Type.Object({
  schemaVersion: Type.Literal(InvocationEventSchemaVersion),
  eventId: EventIdSchema,
  sequence: Type.Integer({ minimum: 0 }),
  occurredAt: IsoDateTimeSchema,
  invocationId: InvocationIdSchema,
  rootTaskId: TaskIdSchema,
  parentInvocationId: Type.Optional(InvocationIdSchema),
  traceId: TraceIdSchema,
  caller: CallerSchema,
  workspaceId: WorkspaceIdSchema,
  targetAgentId: AgentIdSchema,
  agentCardVersion: SemverSchema,
  capability: CapabilityIdSchema
});

const CreatedEventSchema = Type.Composite(
  [InvocationEventBaseSchema, Type.Object({ type: Type.Literal("created"), status: Type.Literal("pending") })],
  { additionalProperties: false }
);
const RoutingEventSchema = Type.Composite(
  [InvocationEventBaseSchema, Type.Object({ type: Type.Literal("routing"), status: Type.Literal("routing") })],
  { additionalProperties: false }
);
const StartedEventSchema = Type.Composite(
  [InvocationEventBaseSchema, Type.Object({ type: Type.Literal("started"), status: Type.Literal("running") })],
  { additionalProperties: false }
);
const StreamEventSchema = Type.Composite(
  [
    InvocationEventBaseSchema,
    Type.Object({
      type: Type.Literal("stream"),
      status: Type.Literal("running"),
      chunkIndex: Type.Integer({ minimum: 0 }),
      chunkBytes: Type.Integer({ minimum: 0 })
    })
  ],
  { additionalProperties: false }
);
const SucceededEventSchema = Type.Composite(
  [
    InvocationEventBaseSchema,
    Type.Object({
      type: Type.Literal("succeeded"),
      status: Type.Literal("succeeded"),
      latencyMs: Type.Integer({ minimum: 0 })
    })
  ],
  { additionalProperties: false }
);
const FailedEventSchema = Type.Composite(
  [
    InvocationEventBaseSchema,
    Type.Object({
      type: Type.Literal("failed"),
      status: Type.Literal("failed"),
      latencyMs: Type.Integer({ minimum: 0 }),
      error: PlatformErrorSchema
    })
  ],
  { additionalProperties: false }
);
const CanceledEventSchema = Type.Composite(
  [
    InvocationEventBaseSchema,
    Type.Object({
      type: Type.Literal("canceled"),
      status: Type.Literal("canceled"),
      latencyMs: Type.Integer({ minimum: 0 }),
      error: Type.Object(
        { code: Type.Literal("CANCELED"), message: Type.String({ minLength: 1 }), traceId: Type.Optional(TraceIdSchema) },
        { additionalProperties: false }
      )
    })
  ],
  { additionalProperties: false }
);
const TimedOutEventSchema = Type.Composite(
  [
    InvocationEventBaseSchema,
    Type.Object({
      type: Type.Literal("timed_out"),
      status: Type.Literal("timed_out"),
      latencyMs: Type.Integer({ minimum: 0 }),
      error: Type.Object(
        { code: Type.Literal("TIMEOUT"), message: Type.String({ minLength: 1 }), traceId: Type.Optional(TraceIdSchema) },
        { additionalProperties: false }
      )
    })
  ],
  { additionalProperties: false }
);

export const InvocationEventSchema = Type.Union([
  CreatedEventSchema,
  RoutingEventSchema,
  StartedEventSchema,
  StreamEventSchema,
  SucceededEventSchema,
  FailedEventSchema,
  CanceledEventSchema,
  TimedOutEventSchema
]);
export type InvocationEvent = Static<typeof InvocationEventSchema>;

export const InvocationEventTypeSchema = Type.Index(InvocationEventSchema, ["type"]);
export type InvocationEventType = Static<typeof InvocationEventTypeSchema>;

export const InvocationRecordSchema = Type.Object(
  {
    invocationId: InvocationIdSchema,
    rootTaskId: TaskIdSchema,
    parentInvocationId: Type.Optional(InvocationIdSchema),
    traceId: TraceIdSchema,
    caller: CallerSchema,
    workspaceId: WorkspaceIdSchema,
    targetAgentId: AgentIdSchema,
    agentCardVersion: SemverSchema,
    capability: CapabilityIdSchema,
    status: InvocationStatusSchema,
    latencyMs: Type.Optional(Type.Integer({ minimum: 0 })),
    errorCode: Type.Optional(PlatformErrorCodeSchema),
    createdAt: IsoDateTimeSchema,
    updatedAt: IsoDateTimeSchema
  },
  { additionalProperties: false }
);
export type InvocationRecord = Static<typeof InvocationRecordSchema>;
