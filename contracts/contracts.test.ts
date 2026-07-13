import { Value } from "@sinclair/typebox/value";
import { describe, expect, it } from "vitest";

import type { Message, MessageSendParams } from "@a2a-js/sdk";

import {
  A2A_CORE_METHODS,
  A2ATaskStateSchema,
  CancelTaskRequestSchema,
  GetTaskRequestSchema,
  PhaseOneA2AProfile,
  SendMessageRequestSchema,
  SendStreamingMessageRequestSchema,
  SendStreamingMessageResponseSchema
} from "./a2a-profile/index.js";
import { AgentCardSchema, validateAgentCard } from "./agent-card/index.js";
import { PlatformErrorSchema } from "./errors/index.js";
import { InvocationEventSchema } from "./events/index.js";
import { InstallAgentRequestSchema } from "./platform-api/index.js";

const card = {
  schemaVersion: "0.1",
  agentId: "contract-review",
  name: "Contract Review Agent",
  description: "Reviews contracts against a declared checklist.",
  owner: { id: "nene7ko", displayName: "Nene7ko" },
  version: "1.0.0",
  protocol: { type: "a2a", version: "0.3.0", transport: "JSONRPC", endpoint: "http://contract-agent:4101" },
  skills: [
    {
      id: "contract.review",
      name: "Review contract",
      description: "Reviews a contract.",
      inputSchema: { type: "object" },
      outputSchema: { type: "object" },
      requiredPermissions: ["document.read"]
    }
  ],
  authentication: { type: "none" },
  permissions: [{ id: "document.read", description: "Read the supplied document." }],
  limits: { timeoutMs: 30_000, maxInputBytes: 1_000_000, maxOutputBytes: 1_000_000, streaming: true }
};

describe("Agent Card contract", () => {
  it("accepts a complete versioned card", () => {
    expect(Value.Check(AgentCardSchema, card)).toBe(true);
    expect(validateAgentCard(card)).toEqual({ valid: true, card });
  });

  it("rejects secrets and runtime health fields", () => {
    expect(Value.Check(AgentCardSchema, { ...card, apiKey: "secret" })).toBe(false);
    expect(Value.Check(AgentCardSchema, { ...card, status: "healthy" })).toBe(false);
  });

  it("requires at least one skill", () => {
    expect(Value.Check(AgentCardSchema, { ...card, skills: [] })).toBe(false);
  });

  it("rejects duplicate semantic ids and undeclared permissions", () => {
    const duplicateSkill = { ...card.skills[0], description: "Different description" };
    const duplicateResult = validateAgentCard({ ...card, skills: [...card.skills, duplicateSkill] });
    expect(duplicateResult.valid).toBe(false);

    const undeclaredResult = validateAgentCard({
      ...card,
      skills: [{ ...card.skills[0], requiredPermissions: ["database.write"] }]
    });
    expect(undeclaredResult.valid).toBe(false);
  });

  it("rejects invalid semantic versions", () => {
    expect(Value.Check(AgentCardSchema, { ...card, version: "1.0.0-01" })).toBe(false);
  });
});

describe("platform contracts", () => {
  it("requires explicit accepted permissions during installation", () => {
    expect(
      Value.Check(InstallAgentRequestSchema, {
        agentId: "contract-review",
        versionConstraint: "^1.0.0",
        acceptedPermissions: ["document.read"]
      })
    ).toBe(true);
    expect(Value.Check(InstallAgentRequestSchema, { agentId: "contract-review", versionConstraint: "^1.0.0" })).toBe(
      false
    );
  });

  it("keeps parent invocation identity on nested events", () => {
    expect(
      Value.Check(InvocationEventSchema, {
        eventId: "evt-2",
        schemaVersion: "0.1",
        sequence: 1,
        occurredAt: "2026-07-13T00:00:00.000Z",
        type: "started",
        status: "running",
        invocationId: "inv-child",
        rootTaskId: "task-root",
        parentInvocationId: "inv-parent",
        traceId: "trace-1",
        caller: { type: "agent", id: "contract-review" },
        workspaceId: "workspace-1",
        targetAgentId: "report-agent",
        agentCardVersion: "1.0.0",
        capability: "report.generate"
      })
    ).toBe(true);
  });

  it("rejects inconsistent event type/status pairs and unversioned events", () => {
    const base = {
      schemaVersion: "0.1",
      eventId: "evt-3",
      sequence: 2,
      occurredAt: "2026-07-13T00:00:01.000Z",
      invocationId: "inv-child",
      rootTaskId: "task-root",
      traceId: "trace-1",
      caller: { type: "agent", id: "contract-review" },
      workspaceId: "workspace-1",
      targetAgentId: "report-agent",
      agentCardVersion: "1.0.0",
      capability: "report.generate"
    };

    expect(Value.Check(InvocationEventSchema, { ...base, type: "succeeded", status: "failed", latencyMs: 1 })).toBe(
      false
    );
    expect(Value.Check(InvocationEventSchema, { ...base, type: "failed", status: "failed", latencyMs: 1 })).toBe(false);
    const { schemaVersion: _schemaVersion, ...unversioned } = base;
    expect(Value.Check(InvocationEventSchema, { ...unversioned, type: "started", status: "running" })).toBe(false);
  });

  it("rejects arbitrary secret-bearing error details", () => {
    expect(Value.Check(PlatformErrorSchema, { code: "INTERNAL_ERROR", message: "failed", apiKey: "secret" })).toBe(
      false
    );
  });

  it("pins the phase-one A2A profile", () => {
    expect(PhaseOneA2AProfile.protocolVersion).toBe("0.3.0");
    expect(PhaseOneA2AProfile.requiredMethods).toContain("message/stream");
  });
});

describe("official A2A v0.3 profile", () => {
  const message = {
    kind: "message",
    messageId: "message-1",
    role: "user",
    parts: [{ kind: "text", text: "Review this contract." }]
  } as const satisfies Message;
  const params = { message } satisfies MessageSendParams;

  it("validates official SDK message payloads for unary and streaming calls", () => {
    expect(A2A_CORE_METHODS).toEqual(["message/send", "message/stream", "tasks/get", "tasks/cancel"]);
    expect(
      Value.Check(SendMessageRequestSchema, { jsonrpc: "2.0", id: "request-1", method: "message/send", params })
    ).toBe(true);
    expect(
      Value.Check(SendStreamingMessageRequestSchema, {
        jsonrpc: "2.0",
        id: 2,
        method: "message/stream",
        params
      })
    ).toBe(true);
  });

  it("validates task lookup, cancellation, task states and stream updates", () => {
    expect(
      Value.Check(GetTaskRequestSchema, {
        jsonrpc: "2.0",
        id: "request-2",
        method: "tasks/get",
        params: { id: "task-1", historyLength: 10 }
      })
    ).toBe(true);
    expect(
      Value.Check(CancelTaskRequestSchema, {
        jsonrpc: "2.0",
        id: "request-3",
        method: "tasks/cancel",
        params: { id: "task-1" }
      })
    ).toBe(true);
    expect(Value.Check(A2ATaskStateSchema, "auth-required")).toBe(true);
    expect(
      Value.Check(SendStreamingMessageResponseSchema, {
        jsonrpc: "2.0",
        id: "request-4",
        result: {
          kind: "status-update",
          taskId: "task-1",
          contextId: "context-1",
          status: { state: "working", timestamp: "2026-07-13T00:00:00Z" },
          final: false
        }
      })
    ).toBe(true);
  });

  it("rejects credentials and methods outside the profile", () => {
    expect(
      Value.Check(SendMessageRequestSchema, {
        jsonrpc: "2.0",
        id: "request-5",
        method: "message/send",
        params: { message, token: "secret" }
      })
    ).toBe(false);
    expect(
      Value.Check(SendMessageRequestSchema, {
        jsonrpc: "2.0",
        id: "request-6",
        method: "tasks/resubscribe",
        params
      })
    ).toBe(false);
  });
});
