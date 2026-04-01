package model

import "strings"

var mitreChineseMapping = map[string]struct {
	Name        string
	Description string
}{
	// T1003 - OS Credential Dumping
	"T1003.001": {
		Name:        "LSASS内存转储",
		Description: "攻击者从LSASS进程内存中提取凭据，可能使用procdump、mimikatz等工具",
	},
	"T1003.008": {
		Name:        "/etc/passwd和/etc/shadow",
		Description: "攻击者读取/etc/passwd和/etc/shadow文件获取用户凭据",
	},

	// T1005 - Data from Local System
	"T1005": {
		Name:        "本地系统数据收集",
		Description: "攻击者从本地系统收集敏感数据",
	},

	// T1021 - Remote Services
	"T1021.002": {
		Name:        "SMB/Windows Admin Shares",
		Description: "攻击者使用SMB协议进行横向移动",
	},
	"T1021.004": {
		Name:        "SSH远程执行",
		Description: "攻击者使用SSH进行远程命令执行和横向移动",
	},

	// T1041 - Exfiltration Over C2 Channel
	"T1041": {
		Name:        "通过C2通道数据渗出",
		Description: "攻击者通过命令与控制通道渗出数据",
	},

	// T1046 - Network Service Scanning
	"T1046": {
		Name:        "网络服务扫描",
		Description: "攻击者扫描网络上的开放端口和服务",
	},

	// T1048 - Exfiltration Over Alternative Protocol
	"T1048": {
		Name:        "通过替代协议数据渗出",
		Description: "攻击者使用非标准协议渗出数据",
	},

	// T1053 - Scheduled Task/Job
	"T1053.003": {
		Name:        "Cron任务持久化",
		Description: "攻击者通过cron计划任务建立持久化",
	},

	// T1059 - Command and Scripting Interpreter
	"T1059.001": {
		Name:        "PowerShell命令执行",
		Description: "攻击者使用PowerShell执行恶意命令，常见于编码命令、绕过策略等",
	},
	"T1059.003": {
		Name:        "Windows命令行执行",
		Description: "攻击者使用cmd.exe执行恶意命令",
	},
	"T1059.004": {
		Name:        "Unix Shell反向Shell",
		Description: "攻击者使用bash/sh建立反向Shell连接，实现远程控制",
	},

	// T1068 - Exploitation for Privilege Escalation
	"T1068": {
		Name:        "权限提升漏洞利用",
		Description: "攻击者利用漏洞提升系统权限",
	},

	// T1070 - Indicator Removal
	"T1070.002": {
		Name:        "清除Linux日志",
		Description: "攻击者删除或清除Linux系统日志以掩盖踪迹",
	},
	"T1070.004": {
		Name:        "文件删除",
		Description: "攻击者删除恶意文件以隐藏攻击痕迹",
	},

	// T1082 - System Information Discovery
	"T1082": {
		Name:        "系统信息收集",
		Description: "攻击者收集操作系统、版本、配置等系统信息",
	},

	// T1110 - Brute Force
	"T1110": {
		Name:        "暴力破解",
		Description: "攻击者通过暴力破解获取账户访问权限",
	},

	// T1113 - Screen Capture
	"T1113": {
		Name:        "屏幕截图",
		Description: "攻击者截取屏幕内容以窃取敏感信息",
	},

	// T1190 - Exploit Public-Facing Application
	"T1190": {
		Name:        "公开应用漏洞利用",
		Description: "攻击者利用面向公众的应用程序漏洞",
	},

	// T1222 - File and Directory Permissions Modification
	"T1222.002": {
		Name:        "Linux文件权限修改",
		Description: "攻击者修改文件或目录权限以获取访问权限",
	},

	// T1486 - Data Destruction
	"T1486": {
		Name:        "数据破坏",
		Description: "攻击者破坏或加密数据（如勒索软件）",
	},

	// T1489 - Service Stop
	"T1489": {
		Name:        "停止服务",
		Description: "攻击者停止系统服务以规避检测或造成破坏",
	},

	// T1490 - Inhibit System Recovery
	"T1490": {
		Name:        "阻止系统恢复",
		Description: "攻击者删除备份、快照等以阻止系统恢复",
	},

	// T1543 - Create or Modify System Process
	"T1543.002": {
		Name:        "Systemd服务持久化",
		Description: "攻击者创建或修改systemd服务以建立持久化",
	},

	// T1547 - Boot or Logon Autostart Execution
	"T1547.001": {
		Name:        "注册表启动项",
		Description: "攻击者通过注册表启动项建立持久化",
	},

	// T1548 - Abuse Elevation Control Mechanism
	"T1548.001": {
		Name:        "Setuid/Setgid滥用",
		Description: "攻击者利用setuid/setgid权限提升特权",
	},
	"T1548.003": {
		Name:        "Sudo滥用",
		Description: "攻击者滥用sudo权限执行特权操作",
	},

	// T1572 - Protocol Tunneling
	"T1572": {
		Name:        "协议隧道",
		Description: "攻击者使用隧道协议隐藏通信流量",
	},

	// T1573 - Encrypted Channel
	"T1573": {
		Name:        "加密通道",
		Description: "攻击者使用加密通道进行通信以规避检测",
	},

	// T1587 - Develop Capabilities
	"T1587": {
		Name:        "能力开发",
		Description: "攻击者开发恶意软件或漏洞利用工具",
	},

	// T1588 - Obtain Capabilities
	"T1588": {
		Name:        "获取能力",
		Description: "攻击者获取恶意软件、漏洞利用工具等攻击能力",
	},

	// T1592 - Gather Victim Host Information
	"T1592": {
		Name:        "收集目标主机信息",
		Description: "攻击者收集目标主机的配置、安全软件等信息",
	},

	// T1595 - Active Scanning
	"T1595": {
		Name:        "主动扫描",
		Description: "攻击者主动扫描目标系统以发现漏洞",
	},
}

// GetMITREChineseDescription returns Chinese name and description for a MITRE ID
func GetMITREChineseDescription(mitreID string) (name string, description string) {
	upperID := strings.ToUpper(mitreID)
	if mapping, ok := mitreChineseMapping[upperID]; ok {
		return mapping.Name, mapping.Description
	}
	return "", ""
}
