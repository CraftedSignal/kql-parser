package kql

import (
	"testing"
)

// Real-world KQL queries from Microsoft Sentinel, Defender XDR, and community repositories
// Sources:
// - https://github.com/Bert-JanP/Hunting-Queries-Detection-Rules
// - https://github.com/Azure/Azure-Sentinel
// - https://github.com/reprise99/Sentinel-Queries

var realWorldKQLQueries = []struct {
	name  string
	query string
}{
	// Shadow Copy Deletion Detection (Ransomware)
	{
		name: "ShadowCopyDeletion",
		query: `let CommonRansomwareExecutionCommands = dynamic([@'vssadmin.exe delete shadows /all /quiet',
@'wmic.exe shadowcopy delete', @'wbadmin delete catalog -quiet',
@'Get-WmiObject Win32_Shadowcopy | ForEach-Object {$_.Delete();}',
@'del /s /f /q c:\*.VHD c:\*.bac c:\*.bak c:\*.wbcat c:\*.bkf c:\Backup*.* c:\backup*.* c:\*.set c:\*.win c:\*.dsk',
@'wbadmin delete systemstatebackup -keepVersions:0',
@'schtasks.exe /Change /TN "\Microsoft\Windows\SystemRestore\SR" /disable',
@'reg add "HKLM\SOFTWARE\Policies\Microsoft\Windows NT\SystemRestore" /v "DisableConfig" /t "REG_DWORD" /d "1" /f']);
DeviceProcessEvents
| where ProcessCommandLine has_any (CommonRansomwareExecutionCommands)
| project-reorder Timestamp, ProcessCommandLine, DeviceName, AccountName`,
	},

	// Kerberoasting Detection
	{
		name: "KerberoastingDetection",
		query: `let starttime = 1d;
let endtime = 1h;
let prev23hThreshold = 4;
let prev1hThreshold = 15;
let Kerbevent = (union isfuzzy=true
(SecurityEvent
| where TimeGenerated >= ago(starttime)
| where EventID == 4769
| parse EventData with * 'TicketEncryptionType">' TicketEncryptionType "<" *
| where TicketEncryptionType == '0x17'
| parse EventData with * 'TicketOptions">' TicketOptions "<" *
| where TicketOptions == '0x40810000'
| parse EventData with * 'Status">' Status "<" *
| where Status == '0x0'
| parse EventData with * 'ServiceName">' ServiceName "<" *
| where ServiceName !contains "$" and ServiceName !contains "krbtgt"
| parse EventData with * 'TargetUserName">' TargetUserName "<" *
| where TargetUserName !contains "$@" and TargetUserName !contains ServiceName
| parse EventData with * 'IpAddress">::ffff:' ClientIPAddress "<" *
),
(
WindowsEvent
| where TimeGenerated >= ago(starttime)
| where EventID == 4769 and EventData has '0x17' and EventData has '0x40810000' and EventData has 'krbtgt'
| extend TicketEncryptionType = tostring(EventData.TicketEncryptionType)
| where TicketEncryptionType == '0x17'
| extend TicketOptions = tostring(EventData.TicketOptions)
| where TicketOptions == '0x40810000'
| extend Status = tostring(EventData.Status)
| where Status == '0x0'
| extend ServiceName = tostring(EventData.ServiceName)
| where ServiceName !contains "$" and ServiceName !contains "krbtgt"
| extend TargetUserName = tostring(EventData.TargetUserName)
| where TargetUserName !contains "$@" and TargetUserName !contains ServiceName
| extend ClientIPAddress = tostring(EventData.IpAddress)
));
let Kerbevent23h = Kerbevent
| where TimeGenerated >= ago(starttime) and TimeGenerated < ago(endtime)
| summarize ServiceNameCountPrev23h = dcount(ServiceName), ServiceNameSet23h = makeset(ServiceName)
by Computer, TargetUserName,TargetDomainName, ClientIPAddress, TicketOptions, TicketEncryptionType, Status
| where ServiceNameCountPrev23h < prev23hThreshold;
let Kerbevent1h =
Kerbevent
| where TimeGenerated >= ago(endtime)
| summarize min(TimeGenerated), max(TimeGenerated), ServiceNameCountPrev1h = dcount(ServiceName), ServiceNameSet1h = makeset(ServiceName)
by Computer, TargetUserName, TargetDomainName, ClientIPAddress, TicketOptions, TicketEncryptionType, Status;
Kerbevent1h
| join kind=leftanti
(
Kerbevent23h
) on TargetUserName, TargetDomainName
| where ServiceNameCountPrev1h > prev1hThreshold
| project StartTime = min_TimeGenerated, EndTime = max_TimeGenerated, TargetUserName, Computer, ClientIPAddress, TicketOptions,
TicketEncryptionType, Status, ServiceNameCountPrev1h, ServiceNameSet1h, TargetDomainName
| extend HostName = tostring(split(Computer, ".")[0]), DomainIndex = toint(indexof(Computer, '.'))
| extend HostNameDomain = iff(DomainIndex != -1, substring(Computer, DomainIndex + 1), Computer)
| extend TargetAccount = strcat(TargetDomainName, "\\", TargetUserName)
| project-away DomainIndex`,
	},

	// Excessive Logon Failures
	{
		name: "ExcessiveLogonFailures",
		query: `let starttime = 8d;
let endtime = 1d;
let threshold = 0.333;
let countlimit = 50;
SecurityEvent
| where TimeGenerated >= ago(endtime)
| where EventID == 4625 and AccountType =~ "User"
| where IpAddress !in ("127.0.0.1", "::1")
| summarize StartTime = min(TimeGenerated), EndTime = max(TimeGenerated), CountToday = count() by EventID, Account, LogonTypeName, SubStatus, AccountType, Computer, WorkstationName, IpAddress, Process
| join kind=leftouter (
    SecurityEvent
    | where TimeGenerated between (ago(starttime) .. ago(endtime))
    | where EventID == 4625 and AccountType =~ "User"
    | where IpAddress !in ("127.0.0.1", "::1")
    | summarize CountPrev7day = count() by EventID, Account, LogonTypeName, SubStatus, AccountType, Computer, WorkstationName, IpAddress
) on EventID, Account, LogonTypeName, SubStatus, AccountType, Computer, WorkstationName, IpAddress
| where CountToday >= coalesce(CountPrev7day,0)*threshold and CountToday >= countlimit
| extend Reason = case(
SubStatus =~ '0xC000005E', 'There are currently no logon servers available to service the logon request.',
SubStatus =~ '0xC0000064', 'User logon with misspelled or bad user account',
SubStatus =~ '0xC000006A', 'User logon with misspelled or bad password',
SubStatus =~ '0xC000006D', 'Bad user name or password',
SubStatus =~ '0xC000006E', 'Unknown user name or bad password',
SubStatus =~ '0xC000006F', 'User logon outside authorized hours',
SubStatus =~ '0xC0000070', 'User logon from unauthorized workstation',
SubStatus =~ '0xC0000071', 'User logon with expired password',
SubStatus =~ '0xC0000072', 'User logon to account disabled by administrator',
SubStatus =~ '0xC00000DC', 'Indicates the Sam Server was in the wrong state to perform the desired operation',
SubStatus =~ '0xC0000133', 'Clocks between DC and other computer too far out of sync',
SubStatus =~ '0xC000015B', 'The user has not been granted the requested logon type at this machine',
SubStatus =~ '0xC000018C', 'The logon request failed because the trust relationship between the primary domain and the trusted domain failed',
SubStatus =~ '0xC0000192', 'An attempt was made to logon, but the Netlogon service was not started',
SubStatus =~ '0xC0000193', 'User logon with expired account',
SubStatus =~ '0xC0000224', 'User is required to change password at next logon',
SubStatus =~ '0xC0000225', 'Evidently a bug in Windows and not a risk',
SubStatus =~ '0xC0000234', 'User logon with account locked',
SubStatus =~ '0xC00002EE', 'Failure Reason: An Error occurred during Logon',
SubStatus =~ '0xC0000413', 'Logon Failure: The machine you are logging onto is protected by an authentication firewall',
strcat('Unknown reason substatus: ', SubStatus))
| extend WorkstationName = iff(WorkstationName == "-" or isempty(WorkstationName), Computer , WorkstationName)
| project StartTime, EndTime, EventID, Account, LogonTypeName, SubStatus, Reason, AccountType, Computer, WorkstationName, IpAddress, CountToday, CountPrev7day, Avg7Day = round(CountPrev7day*1.00/7,2), Process
| summarize StartTime = min(StartTime), EndTime = max(EndTime), Computer = make_set(Computer,128), IpAddressList = make_set(IpAddress,128), sum(CountToday), sum(CountPrev7day), avg(Avg7Day)
by EventID, Account, LogonTypeName, SubStatus, Reason, AccountType, WorkstationName, Process
| order by sum_CountToday desc nulls last
| extend timestamp = StartTime, NTDomain = tostring(split(Account, '\\', 0)[0]), Name = tostring(split(Account, '\\', 1)[0])`,
	},

	// Syslog Anomaly Detection
	{
		name: "SyslogAnomalyDetection",
		query: `let Computers=Syslog
| where TimeGenerated >= ago(4d)
| summarize EventCount=count() by Computer, bin(TimeGenerated, 15m)
| where EventCount >= 1000
| order by TimeGenerated
| summarize EventCount=make_list(EventCount), TimeGenerated=make_list(TimeGenerated) by Computer
| extend outliers=series_decompose_anomalies(EventCount, 2)
| mv-expand TimeGenerated, EventCount, outliers
| where outliers == 1
| distinct Computer
;
Syslog
| where TimeGenerated >= ago(4d)
| where Computer in (Computers)
| summarize EventCount=count() by Computer, bin(TimeGenerated, 15m)
| render timechart`,
	},

	// Global Admins List
	{
		name: "ListGlobalAdmins",
		query: `IdentityInfo
| where AssignedRoles contains "Global Admin"
| distinct AccountName, AccountDomain, AccountUPN, AccountSID`,
	},

	// Sentinel Anomalies
	{
		name: "SentinelAnomalies",
		query: `let TimeFrame = 7d;
Anomalies
| where TimeGenerated > ago(TimeFrame)
| extend DetailedResultsKQL = ExtendedLinks[0].DetailBladeInputs
| project-reorder TimeGenerated, Description, UserPrincipalName, RuleName, Tactics, DetailedResultsKQL, Entities`,
	},

	// Analytics Rules Efficiency
	{
		name: "AnalyticsRulesEfficiency",
		query: `let TimeRange = 30d;
SecurityIncident
| where TimeGenerated > ago(TimeRange)
| summarize arg_max(TimeGenerated, *) by IncidentNumber
| where RelatedAnalyticRuleIds != "[]"
| where isnotempty(Classification)
| summarize
     TotalIncidentsTriggered = count(),
     TotalUndetermined = countif(Classification == "Undetermined"),
     TotalBenignPositive = countif(Classification == "BenignPositive"),
     TotalTruePositive = countif(Classification == "TruePositive"),
     TotalFalsePositive = countif(Classification == "FalsePositive")
     by tostring(RelatedAnalyticRuleIds), Title
| sort by TotalFalsePositive, TotalIncidentsTriggered`,
	},

	// PowerShell Encoded Commands
	{
		name: "PowerShellEncodedCommands",
		query: `let EncodedList = dynamic(['-encodedcommand', '-enc']);
let TimeFrame = 48h;
DeviceProcessEvents
| where Timestamp > ago(TimeFrame)
| where ProcessCommandLine contains "powershell" or InitiatingProcessCommandLine contains "powershell"
| where ProcessCommandLine has_any (EncodedList) or InitiatingProcessCommandLine has_any (EncodedList)
| extend base64String = extract(@'\s+([A-Za-z0-9+/]{20}\S+$)', 1, ProcessCommandLine)
| extend DecodedCommandLine = base64_decode_tostring(base64String)
| extend DecodedCommandLineReplaceEmptyPlaces = replace_string(DecodedCommandLine, '\u0000', '')
| where isnotempty(base64String) and isnotempty(DecodedCommandLineReplaceEmptyPlaces)
| summarize UniqueExecutionsList = make_set(DecodedCommandLineReplaceEmptyPlaces) by DeviceName
| extend TotalUniqueEncodedCommandsExecuted = array_length(UniqueExecutionsList)
| project DeviceName, TotalUniqueEncodedCommandsExecuted, UniqueExecutionsList
| sort by TotalUniqueEncodedCommandsExecuted`,
	},

	// BloodHound Detection
	{
		name: "BloodHoundDetection",
		query: `let BloodhoundCommands = dynamic(['-collectionMethod', 'invoke-bloodhound' ,'get-bloodHounddata']);
DeviceProcessEvents
| where ProcessCommandLine has_any (BloodhoundCommands)
| project
     Timestamp,
     DeviceName,
     AccountName,
     AccountDomain,
     ProcessCommandLine,
     FileName,
     InitiatingProcessCommandLine,
     InitiatingProcessFileName`,
	},

	// Complex Join with Multiple Tables
	{
		name: "ComplexJoinMultipleTables",
		query: `let timeframe = 1d;
let threshold = 3;
SigninLogs
| where TimeGenerated >= ago(timeframe)
| where ResultType != "0"
| where AppDisplayName has_any ("Azure", "Office", "SharePoint", "OneDrive")
| summarize FailedAttempts = count(), Apps = make_set(AppDisplayName), Resources = make_set(ResourceDisplayName)
    by UserPrincipalName, IPAddress, Location, tostring(DeviceDetail), UserAgent
| where FailedAttempts >= threshold
| join kind=inner (
    AuditLogs
    | where TimeGenerated >= ago(timeframe)
    | where OperationName has_any ("Add member to group", "Add user", "Update user")
    | extend TargetUPN = tostring(TargetResources[0].userPrincipalName)
    | project TimeGenerated, InitiatedByUPN = tostring(InitiatedBy.user.userPrincipalName), OperationName, TargetUPN
) on $left.UserPrincipalName == $right.InitiatedByUPN
| project-reorder UserPrincipalName, FailedAttempts, Apps, IPAddress, Location`,
	},

	// Time Series Analysis
	{
		name: "TimeSeriesAnalysis",
		query: `let lookback = 14d;
let timeframe = 1h;
let scorethreshold = 5;
let PersssssistAnomalyData = materialize (
    DeviceNetworkEvents
    | where TimeGenerated >= ago(lookback)
    | where ActionType == "ConnectionSuccess"
    | summarize TotalBytesSent = sum(SentBytes) by DestinationIP = RemoteIP, DestinationPort = RemotePort, bin(TimeGenerated, timeframe)
    | summarize EventCount = make_list(TotalBytesSent), TimeList = make_list(TimeGenerated) by DestinationIP, DestinationPort
    | extend outliers = series_decompose_anomalies(EventCount, scorethreshold)
    | mv-expand TimeGenerated = TimeList, EventCount, outliers to typeof(double)
    | where outliers > 0
);
PersssssistAnomalyData
| summarize AnomalyHours = dcount(TimeGenerated), TotalBytes = sum(tolong(EventCount)) by DestinationIP, DestinationPort
| where AnomalyHours > 5 and TotalBytes > 10000000
| sort by TotalBytes desc`,
	},

	// Azure Activity Analysis
	{
		name: "AzureActivityAnalysis",
		query: `AzureActivity
| where TimeGenerated > ago(7d)
| where OperationNameValue has_any ("Microsoft.Compute/virtualMachines/write", "Microsoft.Compute/virtualMachines/delete")
| where ActivityStatusValue == "Success"
| extend ResourceInfo = parse_json(Properties)
| extend VMName = tostring(split(ResourceId, "/")[-1])
| summarize
    Operations = count(),
    FirstOperation = min(TimeGenerated),
    LastOperation = max(TimeGenerated),
    VMList = make_set(VMName)
    by CallerIpAddress, Caller, OperationNameValue
| where Operations > 5
| order by Operations desc`,
	},

	// Network Connection Anomalies
	{
		name: "NetworkConnectionAnomalies",
		query: `let lookback = 7d;
let RareConnections = DeviceNetworkEvents
| where TimeGenerated > ago(lookback)
| where ActionType == "ConnectionSuccess"
| where RemotePort in (22, 23, 3389, 5900, 5985, 5986)
| where not(RemoteIP startswith "10." or RemoteIP startswith "192.168." or RemoteIP startswith "172.16.")
| summarize ConnectionCount = count(), FirstSeen = min(TimeGenerated), LastSeen = max(TimeGenerated),
    Devices = make_set(DeviceName), LocalIPs = make_set(LocalIP)
    by RemoteIP, RemotePort
| where ConnectionCount < 5
| extend TotalDevices = array_length(Devices)
| where TotalDevices < 3;
RareConnections
| join kind=leftouter (
    ThreatIntelligenceIndicator
    | where TimeGenerated > ago(lookback)
    | where NetworkIP != ""
    | distinct NetworkIP, ThreatType, ConfidenceScore
) on $left.RemoteIP == $right.NetworkIP
| project-reorder RemoteIP, RemotePort, ConnectionCount, ThreatType, ConfidenceScore`,
	},

	// User Behavior Analysis
	{
		name: "UserBehaviorAnalysis",
		query: `let timeframe = 14d;
let baseline = 30d;
BehaviorAnalytics
| where TimeGenerated > ago(timeframe)
| where ActivityType has_any ("FailedLogOn", "LogOn", "ElevateAccess")
| summarize
    ActivityCount = count(),
    UniqueDevices = dcount(SourceDevice),
    UniqueIPs = dcount(SourceIPAddress),
    AvgRiskScore = avg(InvestigationPriority)
    by UserPrincipalName, ActivityType, bin(TimeGenerated, 1d)
| join kind=leftouter (
    BehaviorAnalytics
    | where TimeGenerated between (ago(baseline) .. ago(timeframe))
    | summarize BaselineCount = count() by UserPrincipalName, ActivityType
) on UserPrincipalName, ActivityType
| extend AnomalyScore = iff(BaselineCount > 0, (ActivityCount - BaselineCount) * 1.0 / BaselineCount, 1.0)
| where AnomalyScore > 0.5 or AvgRiskScore > 50`,
	},

	// Process Creation with Parent Analysis
	{
		name: "ProcessCreationWithParentAnalysis",
		query: `DeviceProcessEvents
| where TimeGenerated > ago(1d)
| where FileName in~ ("cmd.exe", "powershell.exe", "pwsh.exe", "wscript.exe", "cscript.exe", "mshta.exe")
| where InitiatingProcessFileName in~ ("winword.exe", "excel.exe", "powerpnt.exe", "outlook.exe", "msaccess.exe")
| extend CommandLineLength = strlen(ProcessCommandLine)
| summarize
    EventCount = count(),
    AvgCommandLineLength = avg(CommandLineLength),
    Devices = make_set(DeviceName, 10),
    SampleCommands = make_set(ProcessCommandLine, 5)
    by FileName, InitiatingProcessFileName, AccountName
| where EventCount > 2 or AvgCommandLineLength > 500
| order by EventCount desc`,
	},

	// DNS Query Analysis
	{
		name: "DNSQueryAnalysis",
		query: `let lookback = 7d;
let threshold = 100;
DnsEvents
| where TimeGenerated > ago(lookback)
| where QueryType in ("A", "AAAA", "CNAME")
| extend Domain = tostring(split(Name, ".")[-2])
| extend TLD = tostring(split(Name, ".")[-1])
| where TLD !in ("local", "internal", "corp", "lan")
| summarize
    QueryCount = count(),
    UniqueSubdomains = dcount(Name),
    FirstQuery = min(TimeGenerated),
    LastQuery = max(TimeGenerated),
    ClientIPs = make_set(ClientIP, 50)
    by Domain, TLD
| where QueryCount > threshold or UniqueSubdomains > 50
| extend EntropyScore = log(UniqueSubdomains) / log(2)
| where EntropyScore > 3
| order by QueryCount desc`,
	},

	// Registry Modification Tracking
	{
		name: "RegistryModificationTracking",
		query: `let SuspiciousKeys = dynamic([
    @"SOFTWARE\Microsoft\Windows\CurrentVersion\Run",
    @"SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnce",
    @"SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon",
    @"SYSTEM\CurrentControlSet\Services",
    @"SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\Explorer\Run"
]);
DeviceRegistryEvents
| where TimeGenerated > ago(24h)
| where ActionType in ("RegistryValueSet", "RegistryKeyCreated")
| where RegistryKey has_any (SuspiciousKeys)
| where InitiatingProcessFileName !in ("svchost.exe", "services.exe", "msiexec.exe", "TiWorker.exe")
| summarize
    ModificationCount = count(),
    Values = make_set(RegistryValueName, 10),
    Data = make_set(RegistryValueData, 10)
    by DeviceName, RegistryKey, InitiatingProcessFileName, InitiatingProcessCommandLine
| where ModificationCount > 1
| order by ModificationCount desc`,
	},

	// File Hash Hunting
	{
		name: "FileHashHunting",
		query: `let MaliciousHashes = dynamic([
    "e99a18c428cb38d5f260853678922e03",
    "098f6bcd4621d373cade4e832627b4f6",
    "5d41402abc4b2a76b9719d911017c592"
]);
DeviceFileEvents
| where TimeGenerated > ago(7d)
| where ActionType in ("FileCreated", "FileModified")
| where MD5 in (MaliciousHashes) or SHA256 in (MaliciousHashes)
| summarize
    EventCount = count(),
    FirstSeen = min(TimeGenerated),
    LastSeen = max(TimeGenerated),
    Devices = make_set(DeviceName),
    FilePaths = make_set(FolderPath)
    by FileName, MD5, SHA256
| extend DeviceCount = array_length(Devices)
| order by DeviceCount desc, EventCount desc`,
	},

	// Authentication Patterns
	{
		name: "AuthenticationPatterns",
		query: `let timeframe = 7d;
union SigninLogs, AADNonInteractiveUserSignInLogs
| where TimeGenerated > ago(timeframe)
| extend DeviceOS = tostring(DeviceDetail.operatingSystem)
| extend DeviceBrowser = tostring(DeviceDetail.browser)
| extend DeviceId = tostring(DeviceDetail.deviceId)
| summarize
    SuccessCount = countif(ResultType == "0"),
    FailureCount = countif(ResultType != "0"),
    UniqueDevices = dcount(DeviceId),
    UniqueIPs = dcount(IPAddress),
    UniqueLocations = dcount(Location),
    OSTypes = make_set(DeviceOS, 5),
    Browsers = make_set(DeviceBrowser, 5)
    by UserPrincipalName, AppDisplayName, bin(TimeGenerated, 1h)
| extend FailureRate = FailureCount * 100.0 / (SuccessCount + FailureCount)
| where FailureRate > 30 or UniqueLocations > 3`,
	},

	// Cloud App Activity
	{
		name: "CloudAppActivity",
		query: `CloudAppEvents
| where TimeGenerated > ago(24h)
| where ActionType has_any ("FileDownloaded", "FileUploaded", "FileShared", "FileDeleted")
| extend FileExtension = tostring(split(ObjectName, ".")[-1])
| extend FileSize = tolong(RawEventData.file_size)
| summarize
    TotalActions = count(),
    TotalSize = sum(FileSize),
    UniqueFiles = dcount(ObjectName),
    Actions = make_set(ActionType)
    by AccountDisplayName, Application, FileExtension, bin(TimeGenerated, 1h)
| where TotalActions > 50 or TotalSize > 100000000
| order by TotalSize desc`,
	},

	// Threat Intelligence Correlation
	{
		name: "ThreatIntelligenceCorrelation",
		query: `let TI = ThreatIntelligenceIndicator
| where TimeGenerated > ago(30d)
| where isnotempty(NetworkIP) or isnotempty(DomainName) or isnotempty(FileHashValue)
| where ConfidenceScore > 50
| extend IndicatorType = case(
    isnotempty(NetworkIP), "IP",
    isnotempty(DomainName), "Domain",
    isnotempty(FileHashValue), "FileHash",
    "Other")
| project IndicatorValue = coalesce(NetworkIP, DomainName, FileHashValue), IndicatorType, ThreatType, ConfidenceScore;
DeviceNetworkEvents
| where TimeGenerated > ago(1d)
| join kind=inner (TI | where IndicatorType == "IP") on $left.RemoteIP == $right.IndicatorValue
| summarize
    HitCount = count(),
    Devices = make_set(DeviceName, 20),
    FirstHit = min(TimeGenerated),
    LastHit = max(TimeGenerated)
    by RemoteIP, ThreatType, ConfidenceScore
| order by ConfidenceScore desc, HitCount desc`,
	},

	// Custom Function Usage
	{
		name: "CustomFunctionUsage",
		query: `let GetRiskLevel = (score: int) {
    case(
        score >= 90, "Critical",
        score >= 70, "High",
        score >= 50, "Medium",
        score >= 30, "Low",
        "Informational"
    )
};
let TimeWindow = 7d;
SecurityAlert
| where TimeGenerated > ago(TimeWindow)
| extend RiskScore = toint(ExtendedProperties.RiskScore)
| extend RiskLevel = GetRiskLevel(RiskScore)
| summarize AlertCount = count(), AvgRiskScore = avg(RiskScore) by RiskLevel, AlertName, ProductName
| where AlertCount > 5
| order by AvgRiskScore desc`,
	},

	// Data Exfiltration Detection
	{
		name: "DataExfiltrationDetection",
		query: `let lookback = 14d;
let timeframe = 1h;
let bytethreshold = 50000000;
DeviceNetworkEvents
| where TimeGenerated > ago(lookback)
| where ActionType == "ConnectionSuccess"
| where RemotePort in (80, 443, 8080, 8443)
| where not(RemoteIP startswith "10." or RemoteIP startswith "192.168." or RemoteIP startswith "172.")
| summarize TotalBytesSent = sum(SentBytes), TotalBytesReceived = sum(ReceivedBytes)
    by DeviceName, RemoteIP, RemotePort, bin(TimeGenerated, timeframe)
| extend Ratio = TotalBytesSent * 1.0 / max_of(TotalBytesReceived, 1)
| where TotalBytesSent > bytethreshold and Ratio > 10
| summarize
    HourlyExfil = make_list(TotalBytesSent),
    TimeStamps = make_list(TimeGenerated)
    by DeviceName, RemoteIP, RemotePort
| extend outliers = series_decompose_anomalies(HourlyExfil, 3)
| mv-expand TimeStamps, HourlyExfil, outliers
| where outliers > 0`,
	},

	// Multi-Table Complex Query
	{
		name: "MultiTableComplexQuery",
		query: `let timeframe = 7d;
let SuspiciousUsers = SigninLogs
| where TimeGenerated > ago(timeframe)
| where ResultType != "0"
| summarize FailedCount = count() by UserPrincipalName
| where FailedCount > 10
| project UserPrincipalName;
let UserAlerts = SecurityAlert
| where TimeGenerated > ago(timeframe)
| mv-expand todynamic(Entities)
| extend EntityType = tostring(Entities.Type)
| where EntityType == "account"
| extend AlertUPN = tostring(Entities.Name)
| summarize AlertCount = count(), AlertTypes = make_set(AlertName) by AlertUPN;
SuspiciousUsers
| join kind=leftouter (UserAlerts) on $left.UserPrincipalName == $right.AlertUPN
| join kind=leftouter (
    AuditLogs
    | where TimeGenerated > ago(timeframe)
    | where OperationName has "password"
    | extend TargetUPN = tostring(TargetResources[0].userPrincipalName)
    | summarize PasswordOps = count() by TargetUPN
) on $left.UserPrincipalName == $right.TargetUPN
| project UserPrincipalName, AlertCount = coalesce(AlertCount, 0), PasswordOps = coalesce(PasswordOps, 0), AlertTypes`,
	},

	// Complex Parsing
	{
		name: "ComplexParsing",
		query: `Syslog
| where TimeGenerated > ago(1d)
| where Facility == "authpriv" and SeverityLevel in ("err", "warning", "crit")
| parse SyslogMessage with * "user=" User:string " " *
| parse SyslogMessage with * "src=" SourceIP:string " " *
| parse SyslogMessage with * "port=" Port:int " " *
| parse SyslogMessage with * "proto=" Protocol:string *
| where isnotempty(User)
| summarize
    EventCount = count(),
    UniqueSourceIPs = dcount(SourceIP),
    UniquePorts = dcount(Port),
    SourceIPList = make_set(SourceIP, 20),
    PortList = make_set(Port, 20)
    by Computer, User, ProcessName
| where EventCount > 100 or UniqueSourceIPs > 5`,
	},

	// Render Visualization
	{
		name: "RenderVisualization",
		query: `SecurityEvent
| where TimeGenerated > ago(7d)
| where EventID in (4624, 4625, 4648, 4672)
| summarize EventCount = count() by EventID, bin(TimeGenerated, 1h)
| render timechart with (title="Authentication Events Over Time", xtitle="Time", ytitle="Event Count")`,
	},

	// Make-series for Forecasting
	{
		name: "MakeSeriesForecasting",
		query: `let starttime = 14d;
let endtime = 0d;
let interval = 1h;
SecurityEvent
| where TimeGenerated between (ago(starttime) .. ago(endtime))
| where EventID == 4625
| make-series FailedLogons = count() default = 0 on TimeGenerated from ago(starttime) to ago(endtime) step interval by Computer
| extend (anomalies, score, baseline) = series_decompose_anomalies(FailedLogons, 1.5, -1, 'linefit')
| mv-expand TimeGenerated, FailedLogons, anomalies, score, baseline
| where anomalies == 1
| project TimeGenerated, Computer, FailedLogons, score, baseline`,
	},
}

// Additional real-world queries from Bert-JanP repository
var additionalRealWorldQueries = []struct {
	name  string
	query string
}{
	// Local Account Created Detection
	{
		name: "LocalAccountCreated",
		query: `let Servers = DeviceInfo
| where DeviceType == 'Server'
| summarize make_set(DeviceId);
let WorkStations = DeviceInfo
| where DeviceType == 'Workstation'
| summarize make_set(DeviceId);
DeviceEvents
| where ActionType == 'UserAccountCreated'
| extend DeviceNameWithoutDomain = extract(@'(.*?)\.', 1, DeviceName)
| where AccountDomain =~ DeviceNameWithoutDomain
| extend DeviceType = iff(DeviceId in (WorkStations), 'WorkStation', iff(DeviceId in (Servers), 'Server', 'Other'))
| project Timestamp, DeviceName, DeviceType, ActionType, AccountDomain, AccountName, AccountSid`,
	},

	// PSExec Executions
	{
		name: "PSExecExecutions",
		query: `DeviceProcessEvents
| where ProcessCommandLine contains "psexec.exe"
| extend RemoteDevice = extract(@'\\\\(.*)c:', 1, ProcessCommandLine)
| summarize TotalRemoteDevices = dcount(RemoteDevice), RemoteDeviceList = make_set(RemoteDevice), ExecutedCommands = make_set(ProcessCommandLine) by DeviceName
| sort by TotalRemoteDevices`,
	},

	// Mshta Executions
	{
		name: "MshtaExecutions",
		query: `let SuspiciousChildProcesses = dynamic(['cmd.exe', 'powershell.exe', 'bash.exe', 'csscript.exe', 'mshta.exe', 'msiexec.exe', 'rundll32.exe']);
DeviceProcessEvents
| where InitiatingProcessFileName =~ 'mshta.exe' or ProcessVersionInfoOriginalFileName =~ 'mshta.exe'
| project-reorder Timestamp, DeviceName, ProcessCommandLine, InitiatingProcessCommandLine, AccountUpn, ProcessVersionInfoOriginalFileName`,
	},

	// Security Log Cleared
	{
		name: "SecurityLogCleared",
		query: `DeviceEvents
| where ActionType == 'SecurityLogCleared'
| project Timestamp, DeviceName, ActionType`,
	},

	// Remote SMB Connection
	{
		name: "RemoteSMBConnection",
		query: `DeviceNetworkEvents
| where RemoteIPType == "Public"
| where RemotePort == 445
| where ActionType == "ConnectionSuccess"
| project-reorder Timestamp, DeviceName, RemoteIP`,
	},

	// Cleartext Password in CommandLine
	{
		name: "CleartextPasswordInCommandLine",
		query: `DeviceProcessEvents
| where ProcessCommandLine has_all ("-password", "*")
| extend UserName = tostring(extract(@'user(?:name)?[=\s](\w+)', 1, ProcessCommandLine))
| summarize TotalExecutions = count(), UniqueCommands = dcount(ProcessCommandLine), CommandLines = make_set(ProcessCommandLine, 1000), UniqueUsers = dcount(UserName), UserNames = make_set(UserName) by DeviceName
| sort by UniqueUsers, UniqueCommands, TotalExecutions`,
	},

	// Scheduled Task Hide Detection
	{
		name: "ScheduledTaskHide",
		query: `SecurityEvent
| where EventID == 4657
| extend EventData = parse_xml(EventData).EventData.Data
| mv-expand bagexpansion=array EventData
| evaluate bag_unpack(EventData)
| extend Key = tostring(column_ifexists('@Name', "")), Value = column_ifexists('#text', "")
| evaluate pivot(Key, any(Value), TimeGenerated, TargetAccount, Computer, EventSourceName, Channel, Task, Level, EventID, Activity, TargetLogonId, SourceComputerId, EventOriginId, Type, _ResourceId, TenantId, SourceSystem, ManagementGroupName, IpAddress, Account)
| extend ObjectName = column_ifexists('ObjectName', ""), OperationType = column_ifexists('OperationType', ""), ObjectValueName = column_ifexists('ObjectValueName', "")
| where ObjectName has 'Schedule\\TaskCache\\Tree' and ObjectValueName == "SD" and OperationType == "%%1906"
| extend HostName = tostring(split(Computer, ".")[0]), DomainIndex = toint(indexof(Computer, '.'))
| extend HostNameDomain = iff(DomainIndex != -1, substring(Computer, DomainIndex + 1), Computer)
| extend AccountName = tostring(split(TargetAccount, @'\')[1]), AccountNTDomain = tostring(split(TargetAccount, @'\')[0])
| extend timestamp = TimeGenerated`,
	},

	// Azure AD Sign-in Risk
	{
		name: "AzureADSignInRisk",
		query: `SigninLogs
| where RiskLevelDuringSignIn != "none"
| where ResultType == "0"
| extend DeviceOS = tostring(DeviceDetail.operatingSystem)
| extend DeviceTrust = tostring(DeviceDetail.trustType)
| summarize RiskySignins = count(), UniqueApps = dcount(AppDisplayName), Locations = make_set(Location)
by UserPrincipalName, RiskLevelDuringSignIn, DeviceOS, DeviceTrust
| order by RiskySignins desc`,
	},

	// MFA Fatigue Attack Detection
	{
		name: "MFAFatigueAttack",
		query: `AADNonInteractiveUserSignInLogs
| where TimeGenerated > ago(1h)
| where ResultType == "50074"
| summarize MFARequests = count(), UniqueIPs = dcount(IPAddress), IPList = make_set(IPAddress)
by UserPrincipalName, AppDisplayName, bin(TimeGenerated, 5m)
| where MFARequests > 5
| order by MFARequests desc`,
	},

	// Malware in Recycle Bin
	{
		name: "MalwareInRecycleBin",
		query: `DeviceFileEvents
| where FolderPath has @'\$Recycle.Bin\'
| where FileName endswith ".exe" or FileName endswith ".dll" or FileName endswith ".ps1" or FileName endswith ".bat"
| summarize FileCount = count(), Files = make_set(FileName)
by DeviceName, FolderPath
| where FileCount > 3`,
	},

	// LSASS Memory Dump Detection
	{
		name: "LSASSMemoryDump",
		query: `DeviceProcessEvents
| where FileName in~ ("procdump.exe", "procdump64.exe", "sqldumper.exe", "rundll32.exe", "comsvcs.dll")
| where ProcessCommandLine has_any ("lsass", "Local Security Authority")
| project-reorder Timestamp, DeviceName, FileName, ProcessCommandLine, InitiatingProcessFileName`,
	},

	// Suspicious Scheduled Task Creation
	{
		name: "SuspiciousScheduledTask",
		query: `DeviceProcessEvents
| where FileName =~ "schtasks.exe"
| where ProcessCommandLine has "/create"
| where ProcessCommandLine has_any ("powershell", "cmd.exe", "wscript", "cscript", "mshta", "regsvr32")
| project-reorder Timestamp, DeviceName, ProcessCommandLine, AccountName`,
	},

	// Registry Run Key Persistence
	{
		name: "RegistryRunKeyPersistence",
		query: `DeviceRegistryEvents
| where RegistryKey has_any (@"SOFTWARE\Microsoft\Windows\CurrentVersion\Run", @"SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnce")
| where ActionType == "RegistryValueSet"
| where RegistryValueData has_any (".exe", ".dll", ".bat", ".ps1", ".vbs", "powershell", "cmd")
| project-reorder Timestamp, DeviceName, RegistryKey, RegistryValueName, RegistryValueData`,
	},

	// WMI Persistence Detection
	{
		name: "WMIPersistence",
		query: `DeviceEvents
| where ActionType == "WmiBindEventFilterToConsumer"
| project-reorder Timestamp, DeviceName, AdditionalFields
| extend Consumer = tostring(parse_json(AdditionalFields).Consumer)
| extend Filter = tostring(parse_json(AdditionalFields).Filter)
| where isnotempty(Consumer)`,
	},

	// Service Creation with Suspicious Path
	{
		name: "SuspiciousServiceCreation",
		query: `DeviceEvents
| where ActionType == "ServiceInstalled"
| extend ServiceName = tostring(parse_json(AdditionalFields).ServiceName)
| extend ServicePath = tostring(parse_json(AdditionalFields).ServiceStartType)
| where ServicePath has_any (@"\Temp\", @"\AppData\", @"\ProgramData\", @"\Users\Public\")
| project-reorder Timestamp, DeviceName, ServiceName, ServicePath`,
	},

	// DNS Tunneling Detection
	{
		name: "DNSTunnelingDetection",
		query: `DnsEvents
| where TimeGenerated > ago(1d)
| extend SubdomainLength = strlen(tostring(split(Name, ".")[0]))
| where SubdomainLength > 30
| summarize QueryCount = count(), UniqueSubdomains = dcount(Name), SampleQueries = make_set(Name, 10)
by ClientIP, QueryType
| where UniqueSubdomains > 100 or QueryCount > 500`,
	},

	// Brute Force RDP Detection
	{
		name: "BruteForceRDP",
		query: `SecurityEvent
| where EventID == 4625
| where LogonType == 10
| summarize FailedAttempts = count(), UniqueAccounts = dcount(TargetAccount), Accounts = make_set(TargetAccount)
by IpAddress, Computer, bin(TimeGenerated, 1h)
| where FailedAttempts > 20
| order by FailedAttempts desc`,
	},

	// Privilege Escalation via Token Manipulation
	{
		name: "TokenManipulation",
		query: `SecurityEvent
| where EventID == 4624
| where LogonType == 9
| where LogonProcessName == "seclogo"
| project-reorder TimeGenerated, Computer, TargetUserName, TargetDomainName, IpAddress, ProcessName`,
	},

	// Cobalt Strike Beacon Detection
	{
		name: "CobaltStrikeBeacon",
		query: `DeviceNetworkEvents
| where RemotePort in (80, 443, 8080, 8443)
| where ActionType == "ConnectionSuccess"
| summarize ConnectionCount = count(), BytesSent = sum(SentBytes), BytesReceived = sum(ReceivedBytes)
by DeviceName, RemoteIP, RemotePort, bin(Timestamp, 1m)
| where ConnectionCount > 50 and BytesSent < 1000 and BytesReceived < 5000`,
	},

	// Office Macro Execution
	{
		name: "OfficeMacroExecution",
		query: `DeviceProcessEvents
| where InitiatingProcessFileName in~ ("winword.exe", "excel.exe", "powerpnt.exe", "outlook.exe")
| where FileName in~ ("cmd.exe", "powershell.exe", "wscript.exe", "cscript.exe", "mshta.exe", "regsvr32.exe")
| project-reorder Timestamp, DeviceName, InitiatingProcessFileName, FileName, ProcessCommandLine`,
	},

	// Suspicious PowerShell Download Cradle
	{
		name: "PowerShellDownloadCradle",
		query: `DeviceProcessEvents
| where FileName =~ "powershell.exe" or FileName =~ "pwsh.exe"
| where ProcessCommandLine has_any ("WebClient", "DownloadString", "DownloadFile", "Invoke-WebRequest", "iwr", "curl", "wget", "Net.WebClient", "Start-BitsTransfer")
| project-reorder Timestamp, DeviceName, ProcessCommandLine, InitiatingProcessFileName`,
	},

	// Pass-the-Hash Detection
	{
		name: "PassTheHashDetection",
		query: `SecurityEvent
| where EventID == 4624
| where LogonType == 9
| where AuthenticationPackageName == "Negotiate"
| where LogonProcessName == "seclogo"
| project-reorder TimeGenerated, Computer, TargetUserName, TargetDomainName, WorkstationName, IpAddress`,
	},

	// Golden Ticket Detection
	{
		name: "GoldenTicketDetection",
		query: `SecurityEvent
| where EventID == 4769
| extend TicketOptions = extract(@'TicketOptions">(.*?)<', 1, EventData)
| extend TicketEncryptionType = extract(@'TicketEncryptionType">(.*?)<', 1, EventData)
| where TicketEncryptionType == "0x17"
| where TicketOptions == "0x40810000"
| project-reorder TimeGenerated, Computer, TargetUserName, ServiceName, IpAddress`,
	},

	// DCSync Attack Detection
	{
		name: "DCSyncAttack",
		query: `SecurityEvent
| where EventID == 4662
| where ObjectType contains "domainDNS"
| where Properties has_any ("Replicating Directory Changes", "1131f6ad-9c07-11d1-f79f-00c04fc2dcd2", "1131f6aa-9c07-11d1-f79f-00c04fc2dcd2")
| project-reorder TimeGenerated, Computer, SubjectUserName, SubjectDomainName, ObjectName`,
	},
}

// Additional edge case queries to test parser robustness
var edgeCaseKQLQueries = []struct {
	name  string
	query string
}{
	{
		name:  "EmptyQuery",
		query: "",
	},
	{
		name:  "JustTableName",
		query: "SecurityEvent",
	},
	{
		name:  "SimpleProject",
		query: "SecurityEvent | project Computer, EventID, TimeGenerated",
	},
	{
		name:  "NestedParentheses",
		query: "SecurityEvent | where ((EventID == 4624) and ((Status == \"Success\") or (Status == \"0\")))",
	},
	{
		name:  "VerbatimStrings",
		query: `SecurityEvent | where FilePath == @"C:\Windows\System32\cmd.exe"`,
	},
	{
		name:  "UnicodeInQuery",
		query: `SecurityEvent | where Message contains "utilisateur" or Message contains "Benutzer"`,
	},
	{
		name:  "LongIdentifier",
		query: "DeviceProcessEvents | where InitiatingProcessParentCreationTimeFromProcessCreationTime > 1000",
	},
	{
		name:  "MultipleLetStatements",
		query: `let a = 1; let b = 2; let c = a + b; SecurityEvent | where EventID == c`,
	},
	{
		name:  "ComplexDynamic",
		query: `let items = dynamic(["a", "b", "c"]); SecurityEvent | where Account has_any (items)`,
	},
	{
		name:  "DateTimeLiterals",
		query: `SecurityEvent | where TimeGenerated > datetime(2024-01-01) and TimeGenerated < datetime(2024-12-31T23:59:59Z)`,
	},
	{
		name:  "TimespanLiterals",
		query: `SecurityEvent | where TimeGenerated > ago(1d) | where TimeGenerated < ago(1h)`,
	},
	{
		name:  "NullHandling",
		query: `SecurityEvent | where isnotnull(IpAddress) and isnotempty(Account) | where isnull(TargetAccount)`,
	},
	{
		name:  "ArrayOperations",
		query: `SecurityEvent | extend arr = pack_array(Account, Computer, IpAddress) | where array_length(arr) > 0`,
	},
	{
		name:  "StringFunctions",
		query: `SecurityEvent | extend lower_account = tolower(Account) | extend upper_computer = toupper(Computer) | where strlen(Account) > 10`,
	},
	{
		name:  "MathFunctions",
		query: `SecurityEvent | summarize cnt = count() | extend log_cnt = log(cnt), sqrt_cnt = sqrt(cnt), abs_val = abs(cnt - 100)`,
	},
	{
		name:  "ConditionalCase",
		query: `SecurityEvent | extend Severity = case(EventID == 4625, "High", EventID == 4624, "Low", "Medium")`,
	},
	{
		name:  "Iff",
		query: `SecurityEvent | extend IsAdmin = iff(Account contains "admin", true, false)`,
	},
	{
		name:  "ExtractRegex",
		query: `SecurityEvent | extend Domain = extract(@"@(.+)$", 1, Account)`,
	},
	{
		name:  "ParseJson",
		query: `AuditLogs | extend Props = parse_json(AdditionalDetails) | extend Key = tostring(Props[0].key)`,
	},
	{
		name:  "SplitAndIndex",
		query: `SecurityEvent | extend Parts = split(Account, "\\") | extend Domain = Parts[0], User = Parts[1]`,
	},
	{
		name:  "BagUnpack",
		query: `SecurityEvent | extend Details = pack("Event", EventID, "Time", TimeGenerated) | evaluate bag_unpack(Details)`,
	},
	{
		name:  "ExternalData",
		query: `externaldata(ip:string, country:string) [@"https://example.com/data.csv"] with (format="csv")`,
	},
	{
		name:  "Materialize",
		query: `let data = materialize(SecurityEvent | where TimeGenerated > ago(1d)); data | summarize count() by EventID`,
	},
	{
		name:  "TopNested",
		query: `SecurityEvent | top-nested 5 of Computer by count(), top-nested 3 of EventID by count()`,
	},
	{
		name:  "Facet",
		query: `SecurityEvent | facet by Computer, EventID`,
	},
	{
		name:  "Consume",
		query: `SecurityEvent | consume`,
	},
	{
		name:  "GetSchema",
		query: `SecurityEvent | getschema`,
	},
	{
		name:  "SampleDistinct",
		query: `SecurityEvent | sample-distinct 10 of Computer`,
	},
	{
		name:  "MvExpand",
		query: `SecurityEvent | mv-expand kind=array Entities | extend EntityType = tostring(Entities.Type)`,
	},
	{
		name:  "Serialize",
		query: `SecurityEvent | serialize rn = row_number() | where rn <= 100`,
	},
	{
		name:  "Partition",
		query: `SecurityEvent | partition by Computer (summarize count())`,
	},
	{
		name:  "Fork",
		query: `SecurityEvent | fork (where EventID == 4624) (where EventID == 4625)`,
	},
	{
		name:  "Union",
		query: `union SecurityEvent, WindowsEvent | where TimeGenerated > ago(1d)`,
	},
	{
		name:  "UnionWithWildcard",
		query: `union Security* | summarize count() by Type`,
	},
	{
		name:  "JoinKinds",
		query: `SecurityEvent | join kind=inner (AuditLogs) on $left.Account == $right.Identity`,
	},
	{
		name:  "Lookup",
		query: `SecurityEvent | lookup kind=leftouter (datatable(Code:int, Desc:string)[4624, "Success", 4625, "Failure"]) on $left.EventID == $right.Code`,
	},
	{
		name:  "RangeOperator",
		query: `range x from 1 to 10 step 1 | extend squared = x * x`,
	},
	{
		name:  "Print",
		query: `print x = 1, y = 2, z = "test"`,
	},
	{
		name:  "Datatable",
		query: `datatable(Name:string, Value:int)["A", 1, "B", 2, "C", 3] | summarize sum(Value)`,
	},
}

func TestFuzzKQLParser(t *testing.T) {
	for _, tt := range realWorldKQLQueries {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractConditions(tt.query)

			// The parser should not panic and should return a result
			if result == nil {
				t.Errorf("ExtractConditions returned nil for query: %s", tt.name)
				return
			}

			// Log results for analysis
			t.Logf("Query: %s", tt.name)
			t.Logf("  Conditions: %d", len(result.Conditions))
			t.Logf("  Errors: %d", len(result.Errors))

			if len(result.Errors) > 0 {
				t.Logf("  Error details: %v", result.Errors)
			}

			for i, cond := range result.Conditions {
				if i < 5 { // Only log first 5 conditions
					t.Logf("  Condition %d: %s %s %q (stage %d)", i, cond.Field, cond.Operator, cond.Value, cond.PipeStage)
				}
			}
		})
	}
}

func TestFuzzKQLParserEdgeCases(t *testing.T) {
	for _, tt := range edgeCaseKQLQueries {
		t.Run(tt.name, func(t *testing.T) {
			// Recover from any panics
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Parser panicked on query %s: %v", tt.name, r)
				}
			}()

			result := ExtractConditions(tt.query)

			if result == nil {
				t.Errorf("ExtractConditions returned nil for query: %s", tt.name)
				return
			}

			t.Logf("Query: %s - Conditions: %d, Errors: %d", tt.name, len(result.Conditions), len(result.Errors))
		})
	}
}

// Benchmark parsing performance
func BenchmarkKQLParser(b *testing.B) {
	// Use a complex real-world query for benchmarking
	query := realWorldKQLQueries[1].query // Kerberoasting detection

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ExtractConditions(query)
	}
}

func BenchmarkKQLParserSimple(b *testing.B) {
	query := `SecurityEvent | where EventID == 4624 and Status == "Success"`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ExtractConditions(query)
	}
}

// Test all additional real-world queries from security detection repositories
func TestFuzzKQLParserAdditionalQueries(t *testing.T) {
	for _, tt := range additionalRealWorldQueries {
		t.Run(tt.name, func(t *testing.T) {
			// Recover from any panics
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Parser panicked on query %s: %v", tt.name, r)
				}
			}()

			result := ExtractConditions(tt.query)

			if result == nil {
				t.Errorf("ExtractConditions returned nil for query: %s", tt.name)
				return
			}

			t.Logf("Query: %s", tt.name)
			t.Logf("  Conditions: %d", len(result.Conditions))
			t.Logf("  Errors: %d", len(result.Errors))

			if len(result.Errors) > 0 {
				t.Logf("  Error details: %v", result.Errors)
			}

			for i, cond := range result.Conditions {
				if i < 5 { // Only log first 5 conditions
					t.Logf("  Condition %d: %s %s %q (stage %d)", i, cond.Field, cond.Operator, cond.Value, cond.PipeStage)
				}
			}
		})
	}
}

// Test all queries combined to verify total coverage
func TestFuzzKQLParserTotalCoverage(t *testing.T) {
	totalQueries := len(realWorldKQLQueries) + len(additionalRealWorldQueries) + len(edgeCaseKQLQueries)
	t.Logf("Total queries in fuzz corpus: %d", totalQueries)
	t.Logf("  - Real-world queries: %d", len(realWorldKQLQueries))
	t.Logf("  - Additional real-world queries: %d", len(additionalRealWorldQueries))
	t.Logf("  - Edge case queries: %d", len(edgeCaseKQLQueries))

	successCount := 0
	partialSuccessCount := 0
	errorCount := 0

	allQueries := make([]struct {
		name  string
		query string
	}, 0, totalQueries)

	for _, q := range realWorldKQLQueries {
		allQueries = append(allQueries, q)
	}
	for _, q := range additionalRealWorldQueries {
		allQueries = append(allQueries, q)
	}
	for _, q := range edgeCaseKQLQueries {
		allQueries = append(allQueries, q)
	}

	for _, tt := range allQueries {
		result := ExtractConditions(tt.query)
		if result == nil {
			errorCount++
			continue
		}

		if len(result.Errors) == 0 {
			successCount++
		} else if len(result.Conditions) > 0 {
			partialSuccessCount++
		} else {
			errorCount++
		}
	}

	t.Logf("Results:")
	t.Logf("  - Full success (no errors): %d (%.1f%%)", successCount, float64(successCount)*100/float64(totalQueries))
	t.Logf("  - Partial success (conditions extracted with errors): %d (%.1f%%)", partialSuccessCount, float64(partialSuccessCount)*100/float64(totalQueries))
	t.Logf("  - Failed (no conditions): %d (%.1f%%)", errorCount, float64(errorCount)*100/float64(totalQueries))
}
