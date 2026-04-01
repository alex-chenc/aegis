package seed

import (
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/pkg/logger"

	"go.uber.org/zap"
)

// DefaultBlockPolicies contains the 36 default MITRE ATT&CK block policies
var DefaultBlockPolicies = []model.BlockPolicy{
	// TA0001 - Initial Access
	{MitreID: "T1190", MitreName: "利用面向公网的应用", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1110", MitreName: "暴力破解", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},

	// TA0002 - Execution
	{MitreID: "T1059.004", MitreName: "Unix Shell", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1059.001", MitreName: "PowerShell", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1059.003", MitreName: "Windows Command Shell", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1053.003", MitreName: "Cron", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},

	// TA0003 - Persistence
	{MitreID: "T1543.002", MitreName: "Systemd Service", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1547.001", MitreName: "Registry Run Keys", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1547.006", MitreName: "Kernel Modules", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},

	// TA0004 - Privilege Escalation
	{MitreID: "T1068", MitreName: "漏洞提权", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1548.001", MitreName: "Setuid和Setgid", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1548.003", MitreName: "Sudo和Sudo缓存", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1068.001", MitreName: "Linux特权提升", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},

	// TA0005 - Defense Evasion
	{MitreID: "T1070.002", MitreName: "清除日志", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1070.004", MitreName: "文件删除", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1222.002", MitreName: "文件权限修改", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1034", MitreName: "路径穿越", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1556.002", MitreName: "密码过滤器", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},

	// TA0006 - Credential Access
	{MitreID: "T1003.008", MitreName: "/etc/passwd和/etc/shadow", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1003.001", MitreName: "LSASS内存", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1555.003", MitreName: "Unix密码缓存", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},

	// TA0007 - Discovery
	{MitreID: "T1046", MitreName: "网络服务发现", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1082", MitreName: "系统信息发现", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1018", MitreName: "远程系统发现", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1057", MitreName: "进程发现", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},

	// TA0008 - Lateral Movement
	{MitreID: "T1021.004", MitreName: "SSH", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1021.002", MitreName: "SMB", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1570", MitreName: "横向文件传输", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},

	// TA0009 - Collection
	{MitreID: "T1005", MitreName: "本地系统数据", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1113", MitreName: "屏幕截图", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1056.001", MitreName: "键盘记录", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},

	// TA0011 - Command and Control
	{MitreID: "T1573", MitreName: "加密通道", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1572", MitreName: "协议隧道", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1095", MitreName: "C2通道", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},

	// TA0010 - Exfiltration
	{MitreID: "T1041", MitreName: "C2通道渗出", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1048", MitreName: "替代协议渗出", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},

	// TA0040 - Impact
	{MitreID: "T1486", MitreName: "数据加密", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1490", MitreName: "抑制系统恢复", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1489", MitreName: "服务停止", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
}

// SeedBlockPolicies seeds the default block policies if they don't exist
func SeedBlockPolicies(repo *repository.BlockPolicyRepository) error {
	created := 0
	for _, policy := range DefaultBlockPolicies {
		existing, err := repo.FindByMitreID(policy.MitreID)
		if err != nil {
			logger.Warn("failed to check existing policy", zap.String("mitre_id", policy.MitreID), zap.Error(err))
			continue
		}
		if existing != nil {
			logger.Debug("policy already exists, skipping", zap.String("mitre_id", policy.MitreID))
			continue
		}

		if err := repo.Create(&policy); err != nil {
			logger.Warn("failed to create policy", zap.String("mitre_id", policy.MitreID), zap.Error(err))
			continue
		}
		created++
	}

	logger.Info("block policies seeded", zap.Int("created", created), zap.Int("total", len(DefaultBlockPolicies)))
	return nil
}