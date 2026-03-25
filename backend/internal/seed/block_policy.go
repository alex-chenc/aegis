package seed

import (
	"aegis-system/internal/model"
	"aegis-system/internal/repository"
	"log"

	"gorm.io/gorm"
)

var DefaultBlockPolicies = []model.BlockPolicy{
	{MitreID: "t1003", MitreName: "OS Credential Dumping", Enabled: true, AutoBlock: false, AutoDispose: true, Action: "quarantine_file"},
	{MitreID: "t1003.001", MitreName: "LSASS Memory Dump", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1003.008", MitreName: "/etc/passwd and /etc/shadow Access", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1005", MitreName: "Local Data Collection", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1021.002", MitreName: "SMB Lateral Movement", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1021.004", MitreName: "SSH Lateral Movement", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1041", MitreName: "C2 Exfiltration", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "block_connection"},
	{MitreID: "t1046", MitreName: "Network Service Discovery", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1048", MitreName: "Alternative Protocol Exfiltration", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "block_connection"},
	{MitreID: "t1053.003", MitreName: "Cron Job Persistence", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1059.001", MitreName: "PowerShell Suspicious Commands", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1059.003", MitreName: "Windows Command Shell", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1059.004", MitreName: "Unix Shell", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1068", MitreName: "Exploitation for Privilege Escalation", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1070.002", MitreName: "Log Clearing", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1082", MitreName: "System Information Discovery", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1110", MitreName: "Brute Force", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1113", MitreName: "Screen Capture", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1190", MitreName: "Exploit Public-Facing Application", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1222.002", MitreName: "File Permission Modification", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1486", MitreName: "Data Encrypted for Impact", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1489", MitreName: "Service Stop", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1490", MitreName: "Inhibit System Recovery", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1547.001", MitreName: "Registry Run Keys Persistence", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1548", MitreName: "Abuse Elevation Control Mechanism", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1548.001", MitreName: "Setuid and Setgid Abuse", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1548.003", MitreName: "Sudo Abuse", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1572", MitreName: "Protocol Tunneling", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "block_connection"},
	{MitreID: "t1573", MitreName: "Encrypted Channel", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "block_connection"},
	{MitreID: "t1587", MitreName: "Capability Development", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1588", MitreName: "Capability Obtainment", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1592", MitreName: "Host Information Gathering", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
	{MitreID: "t1595", MitreName: "Active Scanning", Enabled: true, AutoBlock: false, AutoDispose: false, Action: "kill_process"},
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
