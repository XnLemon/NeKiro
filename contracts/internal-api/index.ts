import { Type, type Static } from "@sinclair/typebox";

import { AgentCardSchema } from "../agent-card/index.js";
import { CallerSchema, IdentifierSchema, JsonObjectSchema, SemverSchema } from "../common/index.js";
import { InvocationEventSchema } from "../events/index.js";
import { InstallationSchema } from "../platform-api/index.js";

export const ResolveAgentRequestSchema = Type.Object(
  {
    workspaceId: IdentifierSchema,
    agentId: IdentifierSchema,
    version: SemverSchema,
    capability: IdentifierSchema
  },
  { additionalProperties: false }
);
export type ResolveAgentRequest = Static<typeof ResolveAgentRequestSchema>;

export const ResolveAgentResponseSchema = Type.Object(
  {
    card: AgentCardSchema,
    installation: InstallationSchema
  },
  { additionalProperties: false }
);
export type ResolveAgentResponse = Static<typeof ResolveAgentResponseSchema>;

export const DispatchInvocationRequestSchema = Type.Object(
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
    input: JsonObjectSchema,
    stream: Type.Boolean()
  },
  { additionalProperties: false }
);
export type DispatchInvocationRequest = Static<typeof DispatchInvocationRequestSchema>;

export const DispatchInvocationAcceptedSchema = Type.Object(
  {
    invocationId: IdentifierSchema,
    accepted: Type.Literal(true)
  },
  { additionalProperties: false }
);
export type DispatchInvocationAccepted = Static<typeof DispatchInvocationAcceptedSchema>;

export const RouterEventEnvelopeSchema = Type.Object(
  { event: InvocationEventSchema },
  { additionalProperties: false }
);
export type RouterEventEnvelope = Static<typeof RouterEventEnvelopeSchema>;
