import { Type, type Static } from "@sinclair/typebox";
import { Value } from "@sinclair/typebox/value";

import { IsoDateTimeSchema, JsonObjectSchema, SemverSchema } from "../common/index.js";
import { AgentIdSchema, CapabilityIdSchema, OwnerIdSchema, PermissionIdSchema } from "../identifiers/index.js";

export const AgentCardSchemaVersion = "0.1" as const;

export const AgentSkillSchema = Type.Object(
  {
    id: CapabilityIdSchema,
    name: Type.String({ minLength: 1, maxLength: 120 }),
    description: Type.String({ minLength: 1, maxLength: 2_000 }),
    inputSchema: JsonObjectSchema,
    outputSchema: JsonObjectSchema,
    requiredPermissions: Type.Array(PermissionIdSchema, { uniqueItems: true })
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
    agentId: AgentIdSchema,
    name: Type.String({ minLength: 1, maxLength: 120 }),
    description: Type.String({ minLength: 1, maxLength: 4_000 }),
    owner: Type.Object(
      {
        id: OwnerIdSchema,
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
          id: PermissionIdSchema,
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

export type AgentCardValidationIssue = {
  code: "SCHEMA" | "DUPLICATE_SKILL_ID" | "DUPLICATE_PERMISSION_ID" | "UNDECLARED_PERMISSION";
  path: string;
  message: string;
};

export type AgentCardValidationResult =
  | { valid: true; card: AgentCard }
  | { valid: false; issues: AgentCardValidationIssue[] };

function duplicateValues(values: readonly string[]): Set<string> {
  const seen = new Set<string>();
  const duplicates = new Set<string>();

  for (const value of values) {
    if (seen.has(value)) {
      duplicates.add(value);
    }
    seen.add(value);
  }

  return duplicates;
}

export function validateAgentCard(value: unknown): AgentCardValidationResult {
  if (!Value.Check(AgentCardSchema, value)) {
    return {
      valid: false,
      issues: [...Value.Errors(AgentCardSchema, value)].map((error) => ({
        code: "SCHEMA" as const,
        path: error.path,
        message: error.message
      }))
    };
  }

  const card = value as AgentCard;
  const issues: AgentCardValidationIssue[] = [];
  const duplicateSkillIds = duplicateValues(card.skills.map((skill) => skill.id));
  const duplicatePermissionIds = duplicateValues(card.permissions.map((permission) => permission.id));
  const declaredPermissions = new Set(card.permissions.map((permission) => permission.id));

  for (const skillId of duplicateSkillIds) {
    issues.push({ code: "DUPLICATE_SKILL_ID", path: "/skills", message: `Duplicate skill id: ${skillId}` });
  }

  for (const permissionId of duplicatePermissionIds) {
    issues.push({
      code: "DUPLICATE_PERMISSION_ID",
      path: "/permissions",
      message: `Duplicate permission id: ${permissionId}`
    });
  }

  for (const [skillIndex, skill] of card.skills.entries()) {
    for (const permissionId of skill.requiredPermissions) {
      if (!declaredPermissions.has(permissionId)) {
        issues.push({
          code: "UNDECLARED_PERMISSION",
          path: `/skills/${skillIndex}/requiredPermissions`,
          message: `Undeclared permission: ${permissionId}`
        });
      }
    }
  }

  return issues.length === 0 ? { valid: true, card } : { valid: false, issues };
}
