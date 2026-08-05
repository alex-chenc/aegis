# Aegis native agent Hook bridge

The released `aegis-codex-hook` binary is a backward-compatible name for the
shared native Hook bridge. Provision the product-specific adapter with:

```sh
# Claude Code
/opt/aegis-agent/aegis-codex-hook provision --agent-type claude-code

# OpenClaw (installs the typed-hook plugin and adds its load path)
/opt/aegis-agent/aegis-codex-hook provision --agent-type openclaw

# Hermes shell hooks
/opt/aegis-agent/aegis-codex-hook provision --agent-type hermes

# Zcode (Claude-compatible command hooks; Stop is intentionally not session end)
/opt/aegis-agent/aegis-codex-hook provision --agent-type zcode
```

The Agent uses the matching command to remove only Aegis-managed entries when
an integration is switched off:

```sh
/opt/aegis-agent/aegis-codex-hook remove --agent-type claude-code
```

User-owned hooks remain in place. The source manifest entry for the disabled
integration is removed at the same time.

All four adapters write signed events to the Agent Guard Unix socket. The
helper accepts extra official fields without logging them. A tool event carries
the product tool name/input/result; its actual command PID still comes from the
Agent eBPF correlation path.
