import { Type, type Static } from "@sinclair/typebox";

import { IsoDateTimeSchema, JsonObjectSchema } from "../common/index.js";

export const A2AProtocolVersion = "0.3.0" as const;
export const A2A_PROTOCOL_VERSION = A2AProtocolVersion;

export const A2A_CORE_METHODS = ["message/send", "message/stream", "tasks/get", "tasks/cancel"] as const;

export const A2AMethodSchema = Type.Union([
  Type.Literal("message/send"),
  Type.Literal("message/stream"),
  Type.Literal("tasks/get"),
  Type.Literal("tasks/cancel")
]);
export type A2AMethod = Static<typeof A2AMethodSchema>;

const JsonRpcIdSchema = Type.Union([Type.String({ minLength: 1 }), Type.Number()]);

export const A2APartSchema = Type.Union([
  Type.Object(
    { kind: Type.Literal("text"), text: Type.String() },
    { additionalProperties: false }
  ),
  Type.Object(
    { kind: Type.Literal("data"), data: JsonObjectSchema },
    { additionalProperties: false }
  )
]);

export const A2AMessageSchema = Type.Object(
  {
    kind: Type.Literal("message"),
    messageId: Type.String({ minLength: 1 }),
    role: Type.Union([Type.Literal("user"), Type.Literal("agent")]),
    parts: Type.Array(A2APartSchema, { minItems: 1 }),
    contextId: Type.Optional(Type.String({ minLength: 1 })),
    taskId: Type.Optional(Type.String({ minLength: 1 }))
  },
  { additionalProperties: false }
);

const MessageParamsSchema = Type.Object({ message: A2AMessageSchema }, { additionalProperties: false });

function JsonRpcRequestSchema<TMethod extends A2AMethod, TParams extends ReturnType<typeof Type.Object>>(
  method: TMethod,
  params: TParams
) {
  return Type.Object(
    {
      jsonrpc: Type.Literal("2.0"),
      id: JsonRpcIdSchema,
      method: Type.Literal(method),
      params
    },
    { additionalProperties: false }
  );
}

export const SendMessageRequestSchema = JsonRpcRequestSchema("message/send", MessageParamsSchema);
export const SendStreamingMessageRequestSchema = JsonRpcRequestSchema("message/stream", MessageParamsSchema);
export const GetTaskRequestSchema = JsonRpcRequestSchema(
  "tasks/get",
  Type.Object(
    {
      id: Type.String({ minLength: 1 }),
      historyLength: Type.Optional(Type.Integer({ minimum: 0 }))
    },
    { additionalProperties: false }
  )
);
export const CancelTaskRequestSchema = JsonRpcRequestSchema(
  "tasks/cancel",
  Type.Object({ id: Type.String({ minLength: 1 }) }, { additionalProperties: false })
);

export const A2ATaskStateSchema = Type.Union([
  Type.Literal("submitted"),
  Type.Literal("working"),
  Type.Literal("input-required"),
  Type.Literal("completed"),
  Type.Literal("canceled"),
  Type.Literal("failed"),
  Type.Literal("rejected"),
  Type.Literal("auth-required"),
  Type.Literal("unknown")
]);

const A2AStatusUpdateSchema = Type.Object(
  {
    kind: Type.Literal("status-update"),
    taskId: Type.String({ minLength: 1 }),
    contextId: Type.String({ minLength: 1 }),
    status: Type.Object(
      {
        state: A2ATaskStateSchema,
        timestamp: Type.Optional(IsoDateTimeSchema),
        message: Type.Optional(A2AMessageSchema)
      },
      { additionalProperties: false }
    ),
    final: Type.Boolean()
  },
  { additionalProperties: false }
);

const A2AArtifactUpdateSchema = Type.Object(
  {
    kind: Type.Literal("artifact-update"),
    taskId: Type.String({ minLength: 1 }),
    contextId: Type.String({ minLength: 1 }),
    artifact: Type.Object(
      {
        artifactId: Type.String({ minLength: 1 }),
        parts: Type.Array(A2APartSchema, { minItems: 1 })
      },
      { additionalProperties: false }
    ),
    append: Type.Boolean(),
    lastChunk: Type.Boolean()
  },
  { additionalProperties: false }
);

export const SendStreamingMessageResponseSchema = Type.Object(
  {
    jsonrpc: Type.Literal("2.0"),
    id: JsonRpcIdSchema,
    result: Type.Union([A2AStatusUpdateSchema, A2AArtifactUpdateSchema, A2AMessageSchema])
  },
  { additionalProperties: false }
);

export const A2APlatformProfileSchema = Type.Object(
  {
    protocolVersion: Type.Literal(A2AProtocolVersion),
    transport: Type.Literal("JSONRPC"),
    agentCardPath: Type.Literal("/.well-known/agent-card.json"),
    requiredMethods: Type.Array(A2AMethodSchema, { minItems: 4, uniqueItems: true }),
    contextHeaders: Type.Object(
      {
        traceId: Type.Literal("x-nek-trace-id"),
        invocationId: Type.Literal("x-nek-invocation-id"),
        rootTaskId: Type.Literal("x-nek-root-task-id"),
        parentInvocationId: Type.Literal("x-nek-parent-invocation-id"),
        workspaceId: Type.Literal("x-nek-workspace-id")
      },
      { additionalProperties: false }
    )
  },
  { additionalProperties: false }
);
export type A2APlatformProfile = Static<typeof A2APlatformProfileSchema>;

export const PhaseOneA2AProfile: A2APlatformProfile = {
  protocolVersion: A2AProtocolVersion,
  transport: "JSONRPC",
  agentCardPath: "/.well-known/agent-card.json",
  requiredMethods: [...A2A_CORE_METHODS],
  contextHeaders: {
    traceId: "x-nek-trace-id",
    invocationId: "x-nek-invocation-id",
    rootTaskId: "x-nek-root-task-id",
    parentInvocationId: "x-nek-parent-invocation-id",
    workspaceId: "x-nek-workspace-id"
  }
};
