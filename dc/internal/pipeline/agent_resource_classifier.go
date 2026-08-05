package pipeline

import (
	"encoding/json"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"

	"dc/internal/model"
)

type RuleEvaluationOptions struct {
	TrustedCIDRs             []string
	TrustedDomains           []string
	TrustedPorts             []int
	TrustedProcessDigests    []string
	ExcludedRuleKeys         []string
	ExcludedResourceGroups   []string
	ExcludedOperations       []string
	PolicyExceptionEventIDs  []string
	ApprovedChangeEventIDs   []string
	ExpectedIdentityEventIDs []string
}

func ClassifyAgentBehavior(event *model.AgentBehaviorEvent, options RuleEvaluationOptions) *model.AgentBehaviorEvent {
	if event == nil {
		return nil
	}
	classified := *event
	resource := decodeJSONObject(event.Resource)
	attributes := objectField(resource, "attributes")
	if resource == nil {
		resource = map[string]any{}
	}
	if attributes == nil {
		attributes = map[string]any{}
	}
	resource["attributes"] = attributes

	switch event.Category {
	case "file":
		path := firstString(
			stringValueAny(attributes["resolved_path"]),
			stringValueAny(attributes["host_path"]),
			event.ResourceIdentity,
			stringValueAny(resource["identity"]),
			stringValueAny(attributes["raw_path"]),
		)
		classification := classifyFilePath(path, event.ResourceClassification)
		classified.ResourceClassification = classification
		resource["classification"] = classification
		attributes["path_risk"] = classifyFileRisk(path, classification, attributes)
	case "network":
		externality := classifyNetworkDestination(attributes, options)
		classified.ResourceClassification = externality
		resource["classification"] = externality
		attributes["externality"] = externality
	case "process":
		category := classifyCommand(event)
		if category != "" {
			classified.ResourceClassification = category
			resource["classification"] = category
			attributes["command_category"] = category
		}
	case "tool":
		category := classifyToolCommand(attributes, event.ResourceIdentity)
		if category != "" {
			classified.ResourceClassification = category
			resource["classification"] = category
			attributes["command_category"] = category
		}
	case "identity":
		classification := classifyIdentityTransition(attributes, event.Outcome)
		classified.ResourceClassification = classification
		resource["classification"] = classification
		attributes["privilege_transition"] = classification
	}
	classified.Resource = mustJSON(resource, map[string]any{})
	return &classified
}

func classifyFilePath(path, existing string) string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if existing == "security_control" {
		return existing
	}
	switch {
	case clean == "/etc/shadow", clean == "/etc/gshadow",
		pathUnder(clean, "/root/.ssh"), pathMatchesHomeDir(clean, ".ssh"):
		return "credential"
	case clean == "/etc/sudoers", pathUnder(clean, "/etc/sudoers.d"), pathUnder(clean, "/etc/pam.d"):
		return "privilege_policy"
	case pathUnder(clean, "/root/.aws"), pathMatchesHomeDir(clean, ".aws"),
		pathUnder(clean, "/root/.kube"), pathMatchesHomeDir(clean, ".kube"),
		pathUnder(clean, "/etc/kubernetes/pki"):
		return "cloud_or_cluster_credential"
	case pathUnder(clean, "/etc/systemd/system"), pathUnder(clean, "/etc/cron.d"),
		pathUnder(clean, "/var/spool/cron"), isShellProfile(clean):
		return "persistence"
	case clean == "/var/run/docker.sock", clean == "/run/docker.sock",
		clean == "/run/containerd/containerd.sock", clean == "/run/podman/podman.sock",
		pathUnder(clean, "/etc/docker"), pathUnder(clean, "/etc/containerd"):
		return "container_control"
	case existing != "":
		return existing
	default:
		return "ordinary_file"
	}
}

func classifyFileRisk(path, classification string, attributes map[string]any) string {
	if containsStringValue([]string{
		"credential", "privilege_policy", "cloud_or_cluster_credential",
		"persistence", "security_control", "container_control",
	}, classification) {
		return "sensitive"
	}
	if boolValue(attributes["hidden"]) || boolValue(attributes["executable"]) {
		return "elevated"
	}
	clean := filepath.Clean(path)
	if pathUnder(clean, "/tmp") || pathUnder(clean, "/var/tmp") {
		return "temporary"
	}
	if strings.Contains(clean, "/workspace/") || strings.HasSuffix(clean, "/workspace") {
		return "workspace"
	}
	return "ordinary"
}

func classifyNetworkDestination(attributes map[string]any, options RuleEvaluationOptions) string {
	domain := strings.TrimSuffix(strings.ToLower(stringValueAny(attributes["observed_domain"])), ".")
	if domain != "" && stringValueAny(attributes["dns_evidence_source"]) != "" &&
		matchesTrustedDomain(domain, options.TrustedDomains) {
		return "trusted"
	}
	port := int(numberValueAny(attributes["destination_port"]))
	for _, trustedPort := range options.TrustedPorts {
		if port == trustedPort {
			return "trusted"
		}
	}
	address, err := netip.ParseAddr(stringValueAny(attributes["destination_ip"]))
	if err != nil {
		return "special_or_unknown"
	}
	for _, rawPrefix := range options.TrustedCIDRs {
		prefix, parseErr := netip.ParsePrefix(rawPrefix)
		if parseErr == nil && prefix.Contains(address) {
			return "trusted"
		}
	}
	switch {
	case address.IsLoopback():
		return "loopback"
	case address.IsLinkLocalUnicast():
		return "link_local"
	case address.IsPrivate():
		return "private"
	case !address.IsGlobalUnicast() || isSpecialAddress(address):
		return "special_or_unknown"
	default:
		return "external"
	}
}

func isSpecialAddress(address netip.Addr) bool {
	for _, value := range []string{
		"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32",
	} {
		prefix := netip.MustParsePrefix(value)
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func classifyCommand(event *model.AgentBehaviorEvent) string {
	executable := filepath.Base(strings.TrimSpace(event.ProcessExe))
	if executable == "." || executable == "" {
		var argv []string
		if json.Unmarshal(event.CommandArgv, &argv) == nil && len(argv) > 0 {
			executable = filepath.Base(argv[0])
		}
	}
	return classifyExecutable(executable)
}

func classifyToolCommand(attributes map[string]any, toolName string) string {
	command := stringValueAny(attributes["command"])
	if command == "" {
		if input, ok := attributes["tool_input"].(map[string]any); ok {
			for _, key := range []string{"command", "cmdline", "command_line", "script"} {
				if value := stringValueAny(input[key]); value != "" {
					command = value
					break
				}
			}
		}
	}
	if command == "" {
		command = toolName
	}
	// A shell tool may wrap the real command in `bash -lc`; scan all tokens
	// and return the first sensitive executable instead of classifying only the
	// wrapper.
	for _, token := range strings.Fields(command) {
		if category := classifyExecutable(filepath.Base(strings.Trim(token, "\"'`;,()[]{}"))); category != "" {
			return category
		}
	}
	return ""
}

func classifyExecutable(executable string) string {
	switch executable {
	case "curl", "wget", "nc", "ncat", "socat", "ssh", "scp":
		return "network_transfer"
	case "sudo", "su", "pkexec":
		return "privilege"
	case "chmod", "chown", "setfacl", "setcap":
		return "permission_change"
	case "nsenter", "unshare", "mount", "umount", "chroot":
		return "namespace_mount"
	case "useradd", "usermod", "crontab", "systemctl":
		return "account_persistence"
	case "rm", "dd", "shred":
		return "destructive"
	case "auditctl", "iptables", "nft", "ufw":
		return "security_control"
	default:
		if strings.HasPrefix(executable, "mkfs") {
			return "destructive"
		}
		return ""
	}
}

func classifyIdentityTransition(attributes map[string]any, outcome string) string {
	if outcome != "success" {
		return "attempted"
	}
	before, beforeOK := int64Value(attributes["euid_before"])
	after, afterOK := int64Value(attributes["euid_after"])
	if beforeOK && afterOK && before != 0 && after == 0 {
		if stringValueAny(attributes["user_namespace"]) == "host" {
			return "host_root_gain"
		}
		return "container_root_transition"
	}
	if boolValue(attributes["capability_gained"]) {
		return "capability_gain"
	}
	return "inconclusive"
}

func decodeJSONObject(value json.RawMessage) map[string]any {
	var result map[string]any
	if json.Unmarshal(value, &result) != nil {
		return map[string]any{}
	}
	return result
}

func objectField(value map[string]any, key string) map[string]any {
	child, _ := value[key].(map[string]any)
	return child
}

func firstString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func pathUnder(path, parent string) bool {
	return path == parent || strings.HasPrefix(path, strings.TrimSuffix(parent, "/")+"/")
}

func pathMatchesHomeDir(path, child string) bool {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	return len(parts) >= 3 && parts[0] == "home" && parts[1] != "" && parts[2] == child
}

func isShellProfile(path string) bool {
	base := filepath.Base(path)
	return (strings.HasPrefix(path, "/home/") || strings.HasPrefix(path, "/root/")) &&
		containsStringValue([]string{".profile", ".bashrc", ".bash_profile", ".zshrc"}, base)
}

func matchesTrustedDomain(domain string, trusted []string) bool {
	for _, raw := range trusted {
		suffix := strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), "."), ".")
		if suffix != "" && (domain == suffix || strings.HasSuffix(domain, "."+suffix)) {
			return true
		}
	}
	return false
}

func containsStringValue(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func stringValueAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func numberValueAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		result, _ := typed.Float64()
		return result
	case string:
		result, _ := strconv.ParseFloat(typed, 64)
		return result
	default:
		return 0
	}
}

func int64Value(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case json.Number:
		result, err := typed.Int64()
		return result, err == nil
	case string:
		result, err := strconv.ParseInt(typed, 10, 64)
		return result, err == nil
	default:
		return 0, false
	}
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}
