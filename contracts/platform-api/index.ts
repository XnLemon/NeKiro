import { Type, type Static } from "@sinclair/typebox";

import { AgentCardSchema, AgentCatalogEntrySchema } from "../agent-card/index.js";
import {
  CursorPageSchema,
  IdentifierSchema,
  InstallationStatusSchema,
  InvocationStatusSchema,
  IsoDateTimeSchema,
  JsonObjectSchema,
  SemverSchema
} from "../common/index.js";
import { InvocationEventSchema, InvocationRecordSchema } from "../events/index.js";

export const RegisterAgentRequestSchema = Type.Object(
  { card: AgentCardSchema },
  { additionalProperties: false }
);
export type RegisterAgentRequest = Static<typeof RegisterAgentRequestSchema>;

export const RegisterAgentResponseSchema = AgentCatalogEntrySchema;
export type RegisterAgentResponse = Static<typeof RegisterAgentResponseSchema>;

export const PublishAgentRequestSchema = Type.Object(
  { agentId: IdentifierSchema, version: SemverSchema },
  { additionalProperties: false }
);
export type PublishAgentRequest = Static<typeof PublishAgentRequestSchema>;

export const SearchAgentsQuerySchema = Type.Object(
  {
    query: Type.Optional(Type.String({ minLength: 1, maxLength: 256 })),
    capability: Type.Optional(IdentifierSchema),
    ownerId: Type.Optional(IdentifierSchema),
    limit: Type.Optional(Type.Integer({ minimum: 1, maximum: 100 })),
    cursor: Type.Optional(Type.String({ minLength: 1 }))
  },
  { additionalProperties: false }
);
export type SearchAgentsQuery = Static<typeof SearchAgentsQuerySchema>;

export const SearchAgentsResponseSchema = CursorPageSchema(AgentCatalogEntrySchema);
export type SearchAgentsResponse = Static<typeof SearchAgentsResponseSchema>;

export const InstallationSchema = Type.Object(
  {
    installationId: IdentifierSchema,
    workspaceId: IdentifierSchema,
    agentId: IdentifierSchema,
    versionConstraint: Type.String({ minLength: 1 }),
    installedVersion: SemverSchema,
    acceptedPermissions: Type.Array(IdentifierSchema, { uniqueItems: true }),
    status: InstallationStatusSchema,
    installedAt: IsoDateTimeSchema,
    updatedAt: IsoDateTimeSchema
  },
  { additionalProperties: false }
);
export type Installation = Static<typeof InstallationSchema>;

export const InstallAgentRequestSchema = Type.Object(
  {
    agentId: IdentifierSchema,
    versionConstraint: Type.String({ minLength: 1 }),
    acceptedPermissions: Type.Array(IdentifierSchema, { uniqueItems: true })
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
  {
    agentId: IdentifierSchema,
    capability: IdentifierSchema,
    input: JsonObjectSchema,
    stream: Type.Boolean()
  },
  { additionalProperties: false }
);
export type InvokeAgentRequest = Static<typeof InvokeAgentRequestSchema>;

export const InvokeAgentResponseSchema = Type.Object(
  {
    invocationId: IdentifierSchema,
    rootTaskId: IdentifierSchema,
    traceId: IdentifierSchema,
    status: InvocationStatusSchema
  },
  { additionalProperties: false }
);
export type InvokeAgentResponse = Static<typeof InvokeAgentResponseSchema>;

export const InvocationDetailResponseSchema = Type.Object(
  {
    invocation: InvocationRecordSchema,
    events: Type.Array(InvocationEventSchema)
  },
  { additionalProperties: false }
);
export type InvocationDetailResponse = Static<typeof InvocationDetailResponseSchema>;

export const TraceResponseSchema = Type.Object(
  {
    traceId: IdentifierSchema,
    invocations: Type.Array(InvocationRecordSchema)
  },
  { additionalProperties: false }
);
export type TraceResponse = Static<typeof TraceResponseSchema>;
