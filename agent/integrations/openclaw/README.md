# Aegis OpenClaw adapter

This plugin is the OpenClaw-native producer for Agent Guard. It subscribes to
`session_start`, `before_tool_call`, `after_tool_call`, and `session_end` typed
hooks, then forwards only the signed helper input to the local Aegis Unix
socket. It does not hold the Ed25519 private key and never writes events to the
database.

The plugin is observation-only. If the helper or socket is unavailable, the
OpenClaw tool call continues normally. The helper uses the OpenClaw gateway PID
as the session root; the actual command PID is resolved from Aegis eBPF events.

Set `AEGIS_AGENT_HOOK_BINARY`, `AEGIS_AGENT_GUARD_SOCKET`,
`AEGIS_AGENT_HOOK_PRIVATE_KEY`, `AEGIS_AGENT_HOOK_STATE_DIR`, and
`AEGIS_AGENT_GUARD_SOURCE_ID` when OpenClaw is not using the default Aegis
installation paths.
