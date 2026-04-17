# 检测场景和规则引擎设计 - V5.0

**版本**: 5.0
**状态**: 设计中
**日期**: 2026-03-19

---

## 1. ATT&CK战术覆盖

### 1.1 14个战术完整覆盖

| 战术 | MITRE ID | 检测场景 | 检测方式 | 阻断动作 |
|:---|:---|:---|:---|:---|
| **侦察** | TA0043 | 端口扫描、服务探测、信息收集 | 网络监控、进程监控 | 杀进程 |
| **资源开发** | TA0042 | 恶意软件开发、工具获取 | 文件监控、网络监控 | 杀进程 |
| **初始访问** | TA0001 | 暴力破解、钓鱼、漏洞利用 | 登录日志、网络流量 | 杀进程 |
| **执行** | TA0002 | 反弹shell、命令注入、脚本执行 | 命令行监控、进程监控 | 杀进程 |
| **持久化** | TA0003 | 计划任务、启动项、服务 | crontab监控、文件监控 | 杀进程 |
| **提权** | TA0004 | SUID/SGID、内核漏洞、sudo滥用 | 权限监控、进程监控 | 杀进程 |
| **防御规避** | TA0005 | 日志清除、进程隐藏、文件删除 | 日志监控、文件监控 | 杀进程 |
| **凭据访问** | TA0006 | 凭据转储、密码破解 | 文件访问监控、进程监控 | 杀进程 |
| **发现** | TA0007 | 网络扫描、系统信息收集 | 网络监控、进程监控 | 杀进程 |
| **横向移动** | TA0008 | SSH爆破、远程执行 | 网络监控、登录日志 | 杀进程 |
| **收集** | TA0009 | 文件收集、屏幕截图 | 文件访问监控 | 杀进程 |
| **命令控制** | TA0011 | 反弹shell、隧道、DNS隧道 | 网络监控、进程监控 | 杀进程 |
| **数据渗出** | TA0010 | 数据上传、DNS渗出 | 网络监控、文件监控 | 杀进程 |
| **影响** | TA0040 | 数据加密、数据删除、服务停止 | 文件监控、进程监控 | 杀进程 |

---

## 2. 核心检测场景

### 2.1 执行战术 (TA0002)

#### T1059.004 - Unix Shell

**检测场景**: 反弹shell、命令执行

**检测规则**:
```yaml
rule: reverse_shell_detection
mitre_id: T1059.004
severity: critical
detection:
  - pattern: "/bin/(ba)?sh.*-i"
  - pattern: "nc.*-e.*(/bin/)?(ba)?sh"
  - pattern: "python.*socket.*connect"
  - pattern: "perl.*socket.*connect"
  - pattern: "ruby.*socket.*connect"
  - pattern: "php.*fsockopen"
  - pattern: "mkfifo.*nc"
  - pattern: "socat.*exec"
block_action: kill_process
```

**事件采集**:
```bash
# auditd规则
-a always,exit -F arch=b64 -S execve -k process_exec
```

#### T1059.001 - PowerShell

**检测场景**: PowerShell恶意命令执行

**检测规则**:
```yaml
rule: powershell_detection
mitre_id: T1059.001
severity: high
detection:
  - pattern: "powershell.*-enc"
  - pattern: "powershell.*-e"
  - pattern: "powershell.*bypass"
  - pattern: "powershell.*hidden"
block_action: kill_process
```

#### T1059.003 - Windows Command Shell

**检测场景**: Windows命令执行

**检测规则**:
```yaml
rule: cmd_detection
mitre_id: T1059.003
severity: high
detection:
  - pattern: "cmd.*/c"
  - pattern: "cmd.*/k"
  - pattern: "cmd.*echo"
block_action: kill_process
```

---

### 2.2 持久化战术 (TA0003)

#### T1053.003 - Cron

**检测场景**: cron计划任务持久化

**检测规则**:
```yaml
rule: cron_persistence
mitre_id: T1053.003
severity: high
detection:
  - pattern: "crontab.*-e"
  - pattern: "crontab.*-l"
  - pattern: "/etc/cron"
  - pattern: "/var/spool/cron"
block_action: kill_process
```

**事件采集**:
```bash
# auditd规则
-w /etc/crontab -p wa -k cron_modification
-w /etc/cron.d/ -p wa -k cron_modification
-w /var/spool/cron/ -p wa -k cron_modification
```

#### T1543.002 - Systemd Service

**检测场景**: systemd服务持久化

**检测规则**:
```yaml
rule: systemd_persistence
mitre_id: T1543.002
severity: high
detection:
  - pattern: "systemctl.*enable"
  - pattern: "systemctl.*start"
  - pattern: "/etc/systemd/system"
  - pattern: "/lib/systemd/system"
block_action: kill_process
```

**事件采集**:
```bash
# auditd规则
-w /etc/systemd/system/ -p wa -k systemd_modification
-w /lib/systemd/system/ -p wa -k systemd_modification
```

#### T1547.001 - Registry Run Keys

**检测场景**: 注册表启动项持久化

**检测规则**:
```yaml
rule: registry_persistence
mitre_id: T1547.001
severity: high
detection:
  - pattern: "reg.*add"
  - pattern: "reg.*query"
  - pattern: "HKCU.*Run"
  - pattern: "HKLM.*Run"
block_action: kill_process
```

---

### 2.3 提权战术 (TA0004)

#### T1068 - Exploitation for Privilege Escalation

**检测场景**: 提权漏洞利用

**检测规则**:
```yaml
rule: privilege_escalation_exploit
mitre_id: T1068
severity: critical
detection:
  - pattern: "chmod.*u\\+s"
  - pattern: "chmod.*4755"
  - pattern: "chown.*root"
  - pattern: "setuid"
  - pattern: "setgid"
  - pattern: "sudo.*-i"
  - pattern: "sudo.*su"
block_action: kill_process
```

**事件采集**:
```bash
# auditd规则
-a always,exit -F arch=b64 -S chmod -S chown -k privilege_change
-w /usr/bin/sudo -p x -k sudo_usage
-w /usr/bin/su -p x -k su_usage
```

#### T1548.001 - Setuid and Setgid

**检测场景**: SUID/SGID滥用

**检测规则**:
```yaml
rule: setuid_detection
mitre_id: T1548.001
severity: high
detection:
  - pattern: "chmod.*u\\+s"
  - pattern: "chmod.*g\\+s"
  - pattern: "chmod.*4755"
  - pattern: "chmod.*2755"
block_action: kill_process
```

#### T1548.003 - Sudo and Sudo Caching

**检测场景**: sudo滥用

**检测规则**:
```yaml
rule: sudo_abuse
mitre_id: T1548.003
severity: high
detection:
  - pattern: "sudo.*-i"
  - pattern: "sudo.*su"
  - pattern: "sudo.*bash"
  - pattern: "sudo.*sh"
block_action: kill_process
```

---

### 2.4 防御规避战术 (TA0005)

#### T1070.002 - Clear Linux or Mac System Logs

**检测场景**: 系统日志清除

**检测规则**:
```yaml
rule: log_clearing
mitre_id: T1070.002
severity: critical
detection:
  - pattern: "rm.*/var/log"
  - pattern: "shred.*/var/log"
  - pattern: "echo.*>/var/log"
  - pattern: "truncate.*/var/log"
  - pattern: "journalctl.*--vacuum"
block_action: kill_process
```

**事件采集**:
```bash
# auditd规则
-w /var/log/ -p wa -k log_modification
```

#### T1070.004 - File Deletion

**检测场景**: 文件删除

**检测规则**:
```yaml
rule: file_deletion
mitre_id: T1070.004
severity: high
detection:
  - pattern: "rm.*-rf"
  - pattern: "rm.*-f"
  - pattern: "shred"
  - pattern: "srm"
block_action: kill_process
```

#### T1222.002 - Linux and Mac File and Directory Permissions Modification

**检测场景**: 文件权限修改

**检测规则**:
```yaml
rule: file_permission_modification
mitre_id: T1222.002
severity: high
detection:
  - pattern: "chmod.*777"
  - pattern: "chmod.*755.*(/etc|/usr|/bin)"
  - pattern: "chown.*root.*(/etc|/usr|/bin)"
block_action: kill_process
```

---

### 2.5 凭据访问战术 (TA0006)

#### T1003.008 - /etc/passwd and /etc/shadow

**检测场景**: 密码文件访问

**检测规则**:
```yaml
rule: password_file_access
mitre_id: T1003.008
severity: critical
detection:
  - pattern: "cat.*/etc/passwd"
  - pattern: "cat.*/etc/shadow"
  - pattern: "cat.*/etc/gshadow"
  - pattern: "getent.*shadow"
block_action: kill_process
```

**事件采集**:
```bash
# auditd规则
-w /etc/passwd -p r -k password_file_access
-w /etc/shadow -p r -k password_file_access
-w /etc/gshadow -p r -k password_file_access
```

#### T1003.001 - LSASS Memory

**检测场景**: LSASS内存转储

**检测规则**:
```yaml
rule: lsass_dump
mitre_id: T1003.001
severity: critical
detection:
  - pattern: "procdump.*lsass"
  - pattern: "mimikatz"
  - pattern: "sekurlsa"
block_action: kill_process
```

---

### 2.6 发现战术 (TA0007)

#### T1046 - Network Service Discovery

**检测场景**: 网络服务发现

**检测规则**:
```yaml
rule: network_discovery
mitre_id: T1046
severity: medium
detection:
  - pattern: "nmap"
  - pattern: "masscan"
  - pattern: "netcat.*-z"
  - pattern: "nc.*-z"
block_action: kill_process
```

#### T1082 - System Information Discovery

**检测场景**: 系统信息收集

**检测规则**:
```yaml
rule: system_info_discovery
mitre_id: T1082
severity: medium
detection:
  - pattern: "uname.*-a"
  - pattern: "cat.*/etc/os-release"
  - pattern: "hostname"
  - pattern: "ifconfig"
  - pattern: "ip.*addr"
block_action: kill_process
```

---

### 2.7 横向移动战术 (TA0008)

#### T1021.004 - SSH

**检测场景**: SSH横向移动

**检测规则**:
```yaml
rule: ssh_lateral_movement
mitre_id: T1021.004
severity: high
detection:
  - pattern: "ssh.*-i"
  - pattern: "ssh.*-o.*StrictHostKeyChecking=no"
  - pattern: "scp.*-i"
block_action: kill_process
```

#### T1021.002 - SMB/Windows Admin Shares

**检测场景**: SMB横向移动

**检测规则**:
```yaml
rule: smb_lateral_movement
mitre_id: T1021.002
severity: high
detection:
  - pattern: "smbclient"
  - pattern: "net.*use"
  - pattern: "psexec"
block_action: kill_process
```

---

### 2.8 收集战术 (TA0009)

#### T1005 - Data from Local System

**检测场景**: 本地数据收集

**检测规则**:
```yaml
rule: local_data_collection
mitre_id: T1005
severity: high
detection:
  - pattern: "find.*/home.*-name.*\\.pdf"
  - pattern: "find.*/home.*-name.*\\.doc"
  - pattern: "find.*/home.*-name.*\\.xls"
  - pattern: "tar.*-czf"
block_action: kill_process
```

#### T1113 - Screen Capture

**检测场景**: 屏幕截图

**检测规则**:
```yaml
rule: screen_capture
mitre_id: T1113
severity: medium
detection:
  - pattern: "scrot"
  - pattern: "import"
  - pattern: "xwd"
  - pattern: "xclip.*-selection.*clipboard"
block_action: kill_process
```

---

### 2.9 命令控制战术 (TA0011)

#### T1573 - Encrypted Channel

**检测场景**: 加密隧道

**检测规则**:
```yaml
rule: encrypted_channel
mitre_id: T1573
severity: critical
detection:
  - pattern: "ssh.*-R"
  - pattern: "ssh.*-L"
  - pattern: "ssh.*-D"
  - pattern: "chisel"
  - pattern: "frp"
block_action: kill_process
```

#### T1572 - Protocol Tunneling

**检测场景**: 协议隧道

**检测规则**:
```yaml
rule: protocol_tunneling
mitre_id: T1572
severity: critical
detection:
  - pattern: "iodine"
  - pattern: "dns2tcp"
  - pattern: "dnscat"
block_action: kill_process
```

---

### 2.10 数据渗出战术 (TA0010)

#### T1041 - Exfiltration Over C2 Channel

**检测场景**: 通过C2通道渗出

**检测规则**:
```yaml
rule: c2_exfiltration
mitre_id: T1041
severity: critical
detection:
  - pattern: "curl.*-X.*POST"
  - pattern: "wget.*--post"
  - pattern: "curl.*-d"
block_action: kill_process
```

#### T1048 - Exfiltration Over Alternative Protocol

**检测场景**: 通过替代协议渗出

**检测规则**:
```yaml
rule: alternative_protocol_exfiltration
mitre_id: T1048
severity: critical
detection:
  - pattern: "scp"
  - pattern: "rsync"
  - pattern: "ftp"
block_action: kill_process
```

---

### 2.11 影响战术 (TA0040)

#### T1486 - Data Encrypted for Impact

**检测场景**: 数据加密勒索

**检测规则**:
```yaml
rule: ransomware_detection
mitre_id: T1486
severity: critical
detection:
  - pattern: "encrypt"
  - pattern: "ransom"
  - pattern: "\\$.*bitcoin"
  - pattern: "\\.locked$"
block_action: kill_process
```

#### T1490 - Inhibit System Recovery

**检测场景**: 禁用系统恢复

**检测规则**:
```yaml
rule: inhibit_recovery
mitre_id: T1490
severity: critical
detection:
  - pattern: "vssadmin.*delete"
  - pattern: "wmic.*shadowcopy"
  - pattern: "bcdedit.*recoveryenabled.*no"
block_action: kill_process
```

#### T1489 - Service Stop

**检测场景**: 服务停止

**检测规则**:
```yaml
rule: service_stop
mitre_id: T1489
severity: high
detection:
  - pattern: "systemctl.*stop"
  - pattern: "service.*stop"
  - pattern: "kill.*-9"
block_action: kill_process
```

---

### 2.12 侦察战术 (TA0043)

#### T1595 - Active Scanning

**检测场景**: 主动扫描

**检测规则**:
```yaml
rule: active_scanning
mitre_id: T1595
severity: medium
detection:
  - pattern: "nmap"
  - pattern: "masscan"
  - pattern: "zmap"
  - pattern: "netcat.*-z"
block_action: kill_process
```

#### T1592 - Gather Victim Host Information

**检测场景**: 收集受害主机信息

**检测规则**:
```yaml
rule: host_info_gathering
mitre_id: T1592
severity: medium
detection:
  - pattern: "uname.*-a"
  - pattern: "cat.*/proc/version"
  - pattern: "cat.*/etc/issue"
block_action: kill_process
```

---

### 2.13 资源开发战术 (TA0042)

#### T1587 - Develop Capabilities

**检测场景**: 开发恶意能力

**检测规则**:
```yaml
rule: capability_development
mitre_id: T1587
severity: high
detection:
  - pattern: "gcc.*-o"
  - pattern: "g\\+\\+.*-o"
  - pattern: "make"
  - pattern: "python.*-m.*py_compile"
block_action: kill_process
```

#### T1588 - Obtain Capabilities

**检测场景**: 获取恶意能力

**检测规则**:
```yaml
rule: capability_obtainment
mitre_id: T1588
severity: high
detection:
  - pattern: "wget.*\\.sh"
  - pattern: "curl.*\\.sh"
  - pattern: "wget.*\\.py"
  - pattern: "curl.*\\.py"
block_action: kill_process
```

---

### 2.14 初始访问战术 (TA0001)

#### T1190 - Exploit Public-Facing Application

**检测场景**: 利用面向公网的应用

**检测规则**:
```yaml
rule: public_app_exploit
mitre_id: T1190
severity: critical
detection:
  # Log4Shell
  - pattern: "\\$\\{jndi:ldap://"
  # Spring4Shell
  - pattern: "class\\.module\\.classLoader"
  # Shellshock
  - pattern: "\\(\\)\\s*\\{.*;.*\\}"
  # Dirty COW
  - process: "dirtycow"
block_action: kill_process
```

#### T1110 - Brute Force

**检测场景**: 暴力破解

**检测规则**:
```yaml
rule: brute_force
mitre_id: T1110
severity: high
detection:
  - pattern: "hydra"
  - pattern: "medusa"
  - pattern: "ncrack"
  - pattern: "patator"
block_action: kill_process
```

---

## 3. 规则引擎架构

### 3.1 规则引擎组件

```
┌─────────────────────────────────────────────────────────────┐
│                    规则引擎架构                             │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. 规则加载器 (Rule Loader)                                │
│     • 从Backend接收Sigma规则                                │
│     • 规则热更新（增量下发）                                │
│                                                             │
│  2. 规则解析器 (Rule Parser)                                │
│     • Sigma YAML解析                                        │
│     • Detection语法解析（selection/condition）              │
│                                                             │
│  3. 规则匹配器 (Rule Matcher)                               │
│     • 按logsource分类索引                                   │
│     • 字段匹配（contains/startswith/endswith）              │
│     • 条件组合（and/or/not）                                │
│                                                             │
│  4. 规则管理器 (Rule Manager)                               │
│     • 全量/增量规则同步                                     │
│     • 规则版本管理                                          │
│     • LLM规则生成                                           │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 规则文件结构

```
/rules（本地缓存，由Backend下发）
|-- /sigma
|   |-- process_creation
|   |   |-- reverse_shell_t1059_004.yml
|   |   |-- privilege_escalation_t1068.yml
|   |-- file_access
|   |   |-- password_file_t1003.yml
|   |   |-- log_clearing_t1070.yml
|   |-- network_connection
|   |   |-- suspicious_connection_t1021.yml
|   |   |-- encrypted_channel_t1573.yml
|-- /llm_generated
|   |-- rule_auto_001.yml
|   |-- rule_auto_002.yml
```

---

## 4. LLM规则生成

### 4.1 规则生成流程

```
事件发生
    ↓
LLM分析
    ↓
识别新模式
    ↓
生成规则草案
    ↓
自动测试规则
    ↓
测试通过 → 部署规则
    ↓
测试失败 → 丢弃规则
```

### 4.2 LLM规则生成提示

```
分析以下安全事件，识别攻击模式并生成Sigma规则：

事件信息：
- 时间: {{timestamp}}
- 主机: {{hostname}}
- 进程: {{process_name}}
- 命令行: {{command_line}}
- 用户: {{username}}
- 文件: {{file_path}}
- 网络: {{remote_addr}}
- MITRE ID: {{mitre_id}}

请生成Sigma规则（YAML格式），要求：
1. 规则ID以mitre_id命名
2. 包含title, id, status, description, logsource, detection, level, tags
3. detection部分使用selection和condition
4. 使用适当的字段修饰符（contains/startswith/endswith）
5. 规则应尽可能精确，减少误报

返回格式：
title: {{description}}
id: {{mitre_id}}_auto_generated
status: experimental
description: {{description}}
logsource:
    category: process_creation
    product: linux
detection:
    selection:
        CommandLine|contains:
            - '{{pattern1}}'
            - '{{pattern2}}'
    condition: selection
level: {{severity}}
tags:
    - attack.{{mitre_id}}
```

### 4.3 规则测试

```go
// 规则测试
func testRule(rule *SigmaRule, historicalEvents []*RuntimeEvent) (bool, error) {
    // 1. 解析Sigma规则
    parsed, err := sigma.ParseRule([]byte(rule.Content))
    if err != nil {
        return false, err
    }
    
    // 2. 编译规则匹配器
    matcher := sigma.CompileMatcher(parsed)
    
    // 3. 用历史事件测试
    truePositives := 0
    falsePositives := 0
    
    for _, event := range historicalEvents {
        if matcher.Match(event.ToMap()) {
            if event.IsMalicious {
                truePositives++
            } else {
                falsePositives++
            }
        }
    }
    
    // 4. 计算准确率
    precision := float64(truePositives) / float64(truePositives+falsePositives)
    
    // 5. 判断是否通过测试
    return precision > 0.8, nil
}
```

---

## 5. 规则热更新

### 5.1 规则更新流程

```
Backend推送新规则
    ↓
Agent接收规则
    ↓
验证规则格式
    ↓
编译规则
    ↓
测试规则
    ↓
替换旧规则
    ↓
记录更新日志
```

### 5.2 规则版本管理

```go
// 规则版本
type RuleVersion struct {
    Version     string
    RuleID      string
    Content     string
    GeneratedBy string  // "manual" | "llm"
    TestedAt    time.Time
    DeployedAt  time.Time
    IsActive    bool
}

// 规则管理器
type RuleManager struct {
    versions map[string]*RuleVersion
    active   map[string]*SigmaRule
    index    *RuleIndex  // 按logsource分类索引
}

func (m *RuleManager) DeployRule(ruleID string, content string, generatedBy string) error {
    // 1. 验证规则
    if err := m.validateRule(content); err != nil {
        return err
    }
    
    // 2. 编译规则
    parsed, err := sigma.ParseRule([]byte(content))
    if err != nil {
        return err
    }
    
    // 3. 测试规则
    passed, err := m.testRule(parsed)
    if err != nil {
        return err
    }
    
    if !passed {
        return fmt.Errorf("rule test failed")
    }
    
    // 4. 部署规则
    version := &RuleVersion{
        Version:     generateVersion(),
        RuleID:      ruleID,
        Content:     content,
        GeneratedBy: generatedBy,
        TestedAt:    time.Now(),
        DeployedAt:  time.Now(),
        IsActive:    true,
    }
    
    m.versions[ruleID] = version
    m.active[ruleID] = parsed
    
    return nil
}
```

---

**文档结束**
