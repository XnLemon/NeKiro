import { Type, type Static } from "@sinclair/typebox";

import { AgentCardSchema } from "../agent-card/index.js";
import { JsonObjectSchema, SemverSchema } from "../common/index.js";
import { CallerSchema, InvocationEventSchema } from "../events/index.js";
import {
  AgentIdSchema,
  CapabilityIdSchema,
  InstallationIdSchema,
  InvocationIdSchema,
  PermissionIdSchema,
  TaskIdSchema,
  TraceIdSchema,
  WorkspaceIdSchema
} from "../identifiers/index.js";

export const ResolveAgentRequestSchema = Type.Object(
  { workspaceId: WorkspaceIdSchema, agentId: AgentIdSchema, version: SemverSchema, capability: CapabilityIdSchema },
  { additionalProperties: false }
);
export type ResolveAgentRequest = Static<typeof ResolveAgentRequestSchema>;

export const ResolvedInstallationSchema = Type.Object(
  {
    installationId: InstallationIdSchema,
    workspaceId: WorkspaceIdSchema,
    agentId: AgentIdSchema,
    installedVersion: SemverSchema,
    acceptedPermissions: Type.Array(PermissionIdSchema, { uniqueItems: true }),
    status: Type.Literal("enabled")
  },
  { additionalProperties: false }
);
export type ResolvedInstallation = Static<typeof ResolvedInstallationSchema>;

export const ResolveAgentResponseSchema = Type.Object(
  { card: AgentCardSchema, installation: ResolvedInstallationSchema },
  { additionalProperties: false }
);
export type ResolveAgentResponse = Static<typeof ResolveAgentResponseSchema>;

export const DispatchInvocationRequestSchema = Type.Object(
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
    input: JsonObjectSchema,
    stream: Type.Boolean()
  },
  { additionalProperties: false }
);
export type DispatchInvocationRequest = Static<typeof DispatchInvocationRequestSchema>;

export const DispatchInvocationAcceptedSchema = Type.Object(
  { invocationId: InvocationIdSchema, accepted: Type.Literal(true) },
  { additionalProperties: false }
);
export type DispatchInvocationAccepted = Static<typeof DispatchInvocationAcceptedSchema>;

export const RouterEventEnvelopeSchema = Type.Object(
  { event: InvocationEventSchema },
  { additionalProperties: false }
);
export type RouterEventEnvelope = Static<typeof RouterEventEnvelopeSchema>;
