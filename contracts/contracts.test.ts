import { Value } from "@sinclair/typebox/value";
import { describe, expect, it } from "vitest";

import { AgentCardSchema } from "./agent-card/index.js";
import { PhaseOneA2AProfile } from "./a2a-profile/index.js";
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
  });

  it("rejects secrets and runtime health fields", () => {
    expect(Value.Check(AgentCardSchema, { ...card, apiKey: "secret" })).toBe(false);
    expect(Value.Check(AgentCardSchema, { ...card, status: "healthy" })).toBe(false);
  });

  it("requires at least one skill", () => {
    expect(Value.Check(AgentCardSchema, { ...card, skills: [] })).toBe(false);
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

  it("pins the phase-one A2A profile", () => {
    expect(PhaseOneA2AProfile.protocolVersion).toBe("0.3.0");
    expect(PhaseOneA2AProfile.requiredMethods).toContain("message/stream");
  });
});
