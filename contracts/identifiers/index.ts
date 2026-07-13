import { Type, type Static } from "@sinclair/typebox";

const safeIdentifierOptions = {
  minLength: 1,
  maxLength: 128,
  pattern: "^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,127})$"
} as const;

export const AgentIdSchema = Type.String({ ...safeIdentifierOptions, title: "AgentId" });
export type AgentId = Static<typeof AgentIdSchema>;

export const OwnerIdSchema = Type.String({ ...safeIdentifierOptions, title: "OwnerId" });
export type OwnerId = Static<typeof OwnerIdSchema>;

export const CapabilityIdSchema = Type.String({ ...safeIdentifierOptions, title: "CapabilityId" });
export type CapabilityId = Static<typeof CapabilityIdSchema>;

export const PermissionIdSchema = Type.String({ ...safeIdentifierOptions, title: "PermissionId" });
export type PermissionId = Static<typeof PermissionIdSchema>;

export const WorkspaceIdSchema = Type.String({ ...safeIdentifierOptions, title: "WorkspaceId" });
export type WorkspaceId = Static<typeof WorkspaceIdSchema>;

export const InstallationIdSchema = Type.String({ ...safeIdentifierOptions, title: "InstallationId" });
export type InstallationId = Static<typeof InstallationIdSchema>;

export const InvocationIdSchema = Type.String({ ...safeIdentifierOptions, title: "InvocationId" });
export type InvocationId = Static<typeof InvocationIdSchema>;

export const TaskIdSchema = Type.String({ ...safeIdentifierOptions, title: "TaskId" });
export type TaskId = Static<typeof TaskIdSchema>;

export const TraceIdSchema = Type.String({ ...safeIdentifierOptions, title: "TraceId" });
export type TraceId = Static<typeof TraceIdSchema>;

export const EventIdSchema = Type.String({ ...safeIdentifierOptions, title: "EventId" });
export type EventId = Static<typeof EventIdSchema>;

export const CallerIdSchema = Type.String({ ...safeIdentifierOptions, title: "CallerId" });
export type CallerId = Static<typeof CallerIdSchema>;
