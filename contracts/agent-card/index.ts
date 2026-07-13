import { Type, type Static } from "@sinclair/typebox";

import { IdentifierSchema, IsoDateTimeSchema, JsonObjectSchema, SemverSchema } from "../common/index.js";

export const AgentCardSchemaVersion = "0.1" as const;

export const AgentSkillSchema = Type.Object(
  {
    id: IdentifierSchema,
    name: Type.String({ minLength: 1, maxLength: 120 }),
    description: Type.String({ minLength: 1, maxLength: 2_000 }),
    inputSchema: JsonObjectSchema,
    outputSchema: JsonObjectSchema,
    requiredPermissions: Type.Array(IdentifierSchema, { uniqueItems: true })
  },
  { additionalProperties: false }
);
export type AgentSkill = Static<typeof AgentSkillSchema>;

export const AgentAuthenticationSchema = Type.Object(
  {
    type: Type.Union([
      Type.Literal("none"),
      Type.Literal("api_key"),
      Type.Literal("http_bearer"),
      Type.Literal("oauth2_client_credentials"),
      Type.Literal("mutual_tls")
    ])
  },
  { additionalProperties: false }
);
export type AgentAuthentication = Static<typeof AgentAuthenticationSchema>;

export const AgentLimitsSchema = Type.Object(
  {
    timeoutMs: Type.Integer({ minimum: 1, maximum: 600_000 }),
    maxInputBytes: Type.Integer({ minimum: 1 }),
    maxOutputBytes: Type.Integer({ minimum: 1 }),
    streaming: Type.Boolean()
  },
  { additionalProperties: false }
);
export type AgentLimits = Static<typeof AgentLimitsSchema>;

export const AgentCardSchema = Type.Object(
  {
    schemaVersion: Type.Literal(AgentCardSchemaVersion),
    agentId: IdentifierSchema,
    name: Type.String({ minLength: 1, maxLength: 120 }),
    description: Type.String({ minLength: 1, maxLength: 4_000 }),
    owner: Type.Object(
      {
        id: IdentifierSchema,
        displayName: Type.String({ minLength: 1, maxLength: 120 })
      },
      { additionalProperties: false }
    ),
    version: SemverSchema,
    protocol: Type.Object(
      {
        type: Type.Literal("a2a"),
        version: Type.Literal("0.3.0"),
        transport: Type.Literal("JSONRPC"),
        endpoint: Type.String({ minLength: 1, maxLength: 2_048, pattern: "^https?://[^\\s]+$" })
      },
      { additionalProperties: false }
    ),
    skills: Type.Array(AgentSkillSchema, { minItems: 1, uniqueItems: true }),
    authentication: AgentAuthenticationSchema,
    permissions: Type.Array(
      Type.Object(
        {
          id: IdentifierSchema,
          description: Type.String({ minLength: 1, maxLength: 1_000 })
        },
        { additionalProperties: false }
      ),
      { uniqueItems: true }
    ),
    limits: AgentLimitsSchema
  },
  { additionalProperties: false }
);
export type AgentCard = Static<typeof AgentCardSchema>;

export const AgentPublicationStatusSchema = Type.Union([
  Type.Literal("draft"),
  Type.Literal("published"),
  Type.Literal("disabled")
]);
export type AgentPublicationStatus = Static<typeof AgentPublicationStatusSchema>;

export const AgentCatalogEntrySchema = Type.Object(
  {
    card: AgentCardSchema,
    publicationStatus: AgentPublicationStatusSchema,
    registeredAt: IsoDateTimeSchema,
    publishedAt: Type.Optional(IsoDateTimeSchema)
  },
  { additionalProperties: false }
);
export type AgentCatalogEntry = Static<typeof AgentCatalogEntrySchema>;
