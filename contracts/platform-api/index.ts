import { Type, type Static } from "@sinclair/typebox";

import { AgentCardSchema, AgentCatalogEntrySchema } from "../agent-card/index.js";
import { CursorPageSchema, IsoDateTimeSchema, JsonObjectSchema, SemverRangeSchema, SemverSchema } from "../common/index.js";
import { InvocationEventSchema, InvocationRecordSchema, InvocationStatusSchema } from "../events/index.js";
import {
  AgentIdSchema,
  CapabilityIdSchema,
  InstallationIdSchema,
  InvocationIdSchema,
  OwnerIdSchema,
  PermissionIdSchema,
  TaskIdSchema,
  TraceIdSchema,
  WorkspaceIdSchema
} from "../identifiers/index.js";

export const RegisterAgentRequestSchema = Type.Object({ card: AgentCardSchema }, { additionalProperties: false });
export type RegisterAgentRequest = Static<typeof RegisterAgentRequestSchema>;
export const RegisterAgentResponseSchema = AgentCatalogEntrySchema;
export type RegisterAgentResponse = Static<typeof RegisterAgentResponseSchema>;

export const PublishAgentRequestSchema = Type.Object(
  { agentId: AgentIdSchema, version: SemverSchema },
  { additionalProperties: false }
);
export type PublishAgentRequest = Static<typeof PublishAgentRequestSchema>;

export const SearchAgentsQuerySchema = Type.Object(
  {
    query: Type.Optional(Type.String({ minLength: 1, maxLength: 256, pattern: "\\S" })),
    capability: Type.Optional(CapabilityIdSchema),
    ownerId: Type.Optional(OwnerIdSchema),
    limit: Type.Optional(Type.Integer({ minimum: 1, maximum: 100 })),
    cursor: Type.Optional(Type.String({ minLength: 1 }))
  },
  { additionalProperties: false }
);
export type SearchAgentsQuery = Static<typeof SearchAgentsQuerySchema>;
export const SearchAgentsResponseSchema = CursorPageSchema(AgentCatalogEntrySchema);
export type SearchAgentsResponse = Static<typeof SearchAgentsResponseSchema>;

export const InstallationStatusSchema = Type.Union([
  Type.Literal("enabled"),
  Type.Literal("disabled"),
  Type.Literal("uninstalled")
]);
export type InstallationStatus = Static<typeof InstallationStatusSchema>;

export const InstallationSchema = Type.Object(
  {
    installationId: InstallationIdSchema,
    workspaceId: WorkspaceIdSchema,
    agentId: AgentIdSchema,
    versionConstraint: SemverRangeSchema,
    installedVersion: SemverSchema,
    acceptedPermissions: Type.Array(PermissionIdSchema, { uniqueItems: true }),
    status: InstallationStatusSchema,
    installedAt: IsoDateTimeSchema,
    updatedAt: IsoDateTimeSchema
  },
  { additionalProperties: false }
);
export type Installation = Static<typeof InstallationSchema>;

export const InstallAgentRequestSchema = Type.Object(
  {
    agentId: AgentIdSchema,
    versionConstraint: SemverRangeSchema,
    acceptedPermissions: Type.Array(PermissionIdSchema, { uniqueItems: true })
  },
  { additionalProperties: false }
);
export type InstallAgentRequest = Static<typeof InstallAgentRequestSchema>;

export const UpdateInstallationRequestSchema = Type.Object(
  { status: Type.Union([Type.Literal("enabled"), Type.Literal("disabled")]) },
  { additionalProperties: false }
);
export type UpdateInstallationRequest = Static<typeof UpdateInstallationRequestSchema>;

export const InvokeAgentRequestSchema = Type.Object(
  { agentId: AgentIdSchema, capability: CapabilityIdSchema, input: JsonObjectSchema, stream: Type.Boolean() },
  { additionalProperties: false }
);
export type InvokeAgentRequest = Static<typeof InvokeAgentRequestSchema>;

export const InvokeAgentResponseSchema = Type.Object(
  { invocationId: InvocationIdSchema, rootTaskId: TaskIdSchema, traceId: TraceIdSchema, status: InvocationStatusSchema },
  { additionalProperties: false }
);
export type InvokeAgentResponse = Static<typeof InvokeAgentResponseSchema>;

export const InvocationDetailResponseSchema = Type.Object(
  { invocation: InvocationRecordSchema, events: Type.Array(InvocationEventSchema) },
  { additionalProperties: false }
);
export type InvocationDetailResponse = Static<typeof InvocationDetailResponseSchema>;

export const TraceResponseSchema = Type.Object(
  { traceId: TraceIdSchema, invocations: Type.Array(InvocationRecordSchema) },
  { additionalProperties: false }
);
export type TraceResponse = Static<typeof TraceResponseSchema>;
