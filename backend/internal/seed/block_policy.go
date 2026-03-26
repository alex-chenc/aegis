package seed

import (
	"aegis-system/internal/model"
	"aegis-system/internal/repository"
	"log"

	"gorm.io/gorm"
)

var DefaultBlockPolicies = []model.BlockPolicy{
	{MitreID: "T1003", MitreName: "OS Credential Dumping", Enabled: true, AutoBlock: false, AutoDispose: true, Action: "quarantine_file"},
	{MitreID: "T1003.001", MitreName: "LSASS Memory Dump", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1003.008", MitreName: "/etc/passwd and /etc/shadow Access", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1005", MitreName: "Local Data Collection", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1021.002", MitreName: "SMB Lateral Movement", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1021.004", MitreName: "SSH Lateral Movement", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1041", MitreName: "C2 Exfiltration", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "block_connection"},
	{MitreID: "T1046", MitreName: "Network Service Discovery", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1048", MitreName: "Alternative Protocol Exfiltration", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "block_connection"},
	{MitreID: "T1053.003", MitreName: "Cron Job Persistence", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1059.001", MitreName: "PowerShell Suspicious Commands", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1059.003", MitreName: "Windows Command Shell", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1059.004", MitreName: "Unix Shell", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1068", MitreName: "Exploitation for Privilege Escalation", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1070.002", MitreName: "Log Clearing", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1082", MitreName: "System Information Discovery", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1110", MitreName: "Brute Force", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1113", MitreName: "Screen Capture", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1190", MitreName: "Exploit Public-Facing Application", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1222.002", MitreName: "File Permission Modification", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1486", MitreName: "Data Encrypted for Impact", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1489", MitreName: "Service Stop", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1490", MitreName: "Inhibit System Recovery", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1547.001", MitreName: "Registry Run Keys Persistence", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1548", MitreName: "Abuse Elevation Control Mechanism", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1548.001", MitreName: "Setuid and Setgid Abuse", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1548.003", MitreName: "Sudo Abuse", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1572", MitreName: "Protocol Tunneling", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "block_connection"},
	{MitreID: "T1573", MitreName: "Encrypted Channel", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "block_connection"},
	{MitreID: "T1587", MitreName: "Capability Development", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1588", MitreName: "Capability Obtainment", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1592", MitreName: "Host Information Gathering", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "T1595", MitreName: "Active Scanning", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
}

func SeedBlockPolicies(db *gorm.DB) {
	repo := repository.NewBlockPolicyRepository(db)

	for _, policy := range DefaultBlockPolicies {
		var existing model.BlockPolicy
		err := db.Where("mitre_id = ?", policy.MitreID).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			if err := repo.Create(&policy); err != nil {
				log.Printf("Failed to seed block policy %s: %v", policy.MitreID, err)
			} else {
				log.Printf("Seeded block policy: %s", policy.MitreID)
			}
		}
	}
}
