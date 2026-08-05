import { spawn } from "node:child_process";
import { definePluginEntry } from "openclaw/plugin-sdk/plugin-entry";

const helper = process.env.AEGIS_AGENT_HOOK_BINARY || "/opt/aegis-agent/aegis-codex-hook";
const socket = process.env.AEGIS_AGENT_GUARD_SOCKET || "/run/aegis-agent/agent-guard-hook.sock";
const privateKey = process.env.AEGIS_AGENT_HOOK_PRIVATE_KEY || "/etc/aegis-agent/codex-hook.key";
const stateDir = process.env.AEGIS_AGENT_HOOK_STATE_DIR || `/tmp/aegis-openclaw-hook-${process.getuid?.() ?? 0}`;
const sourceId = process.env.AEGIS_AGENT_GUARD_SOURCE_ID || "openclaw-plugin-v1";

const first = (...values) => values.find((value) => value !== undefined && value !== null && String(value).trim() !== "");
function emit(hookEventName, event = {}, ctx = {}, response, error) {
  const sessionId = first(ctx.sessionId, ctx.sessionKey, event.sessionId, event.sessionKey);
  if (!sessionId) return;
  const payload = {
    agent_type: "openclaw", hook_event_name: hookEventName, session_id: String(sessionId), pid: process.pid,
    tool_name: first(event.toolName, event.tool_name), tool_call_id: first(event.toolCallId, event.tool_call_id, event.runId),
    turn_id: first(ctx.runId, event.runId, ctx.turnId, event.turnId), tool_input: event.params ?? event.toolInput,
    tool_response: response, error: error ?? event.error,
  };
  const child = spawn(helper, ["--agent-type", "openclaw", "--source-id", sourceId, "--socket", socket, "--private-key", privateKey, "--state-dir", stateDir], {
    stdio: ["pipe", "ignore", "ignore"], windowsHide: true,
  });
  child.on("error", () => {});
  child.stdin.end(JSON.stringify(payload));
}

export default definePluginEntry({
  id: "aegis-agent-guard", name: "Aegis Agent Guard", description: "Aegis trusted OpenClaw lifecycle and tool telemetry adapter.",
  register(api) {
    api.on("session_start", (event, ctx) => emit("SessionStart", event, ctx));
    api.on("before_tool_call", (event, ctx) => emit("PreToolUse", event, ctx));
    api.on("after_tool_call", (event, ctx) => emit("PostToolUse", event, ctx, event.result, event.error));
    api.on("session_end", (event, ctx) => emit("SessionEnd", event, ctx));
  },
});
