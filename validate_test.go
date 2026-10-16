package kql

import (
	"strings"
	"testing"
)

func TestValidate_AcceptsQueries(t *testing.T) {
	for name, query := range map[string]string{
		"filter":                         `SecurityEvent | where EventID == 4625`,
		"lets":                           "let threshold = 5;\nlet allowed = dynamic([\"a\", \"b\"]);\nSigninLogs\n| where AppDisplayName !in (allowed)\n| summarize count() by UserPrincipalName\n| where count_ > threshold",
		"leading union":                  `union SecurityEvent, WindowsEvent | where EventID == 4625`,
		"multi-line subquery":            "union\n(\n\n    DeviceProcessEvents\n    | where FileName =~ \"cmd.exe\"\n),\n(\n\n    SecurityEvent\n    | where EventID == 4688\n)",
		"comments":                       "// header\nSecurityEvent // inline\n| where EventID == 4625",
		"url after a quote in a comment": "SecurityEvent\n| extend Note = 1 // a \"quoted note\n| extend References = \"https://example.com/a\"",
		"union in brackets":              "(\n    union\n\n    (DeviceFileEvents\n    | where Timestamp > ago(1d))\n)",
		"datetime with a space":          `SecurityEvent | where TimeGenerated between (datetime(2026-02-04 00:00:00) .. datetime(2026-02-04 23:59:59.123))`,
		"make-graph":                     "ExposureGraphEdges\n| make-graph SourceNodeId --> TargetNodeId with ExposureGraphNodes on NodeId",
		"notcontains":                    `DeviceFileEvents | where FolderPath notcontains @"C:\Windows" and FileName notcontainscs "x"`,
		"adjacent strings":               `DeviceProcessEvents | where ProcessCommandLine contains "cmd /c start " " /min" or FileName in ("a.exe" "b.exe", 'c' 'd')`,
		"union in a string":              `T | where Authorization has_any ("' OR ", " UNION ", " SLEEP(")`,
		"join conditions across lines":   "SigninLogs\n| join kind=leftouter IpHistory\non $left.Actor == $right.AccountUpn\nand $left.IPAddress == $right.IPAddress\n| extend Seen = coalesce(DaysSeen, 0)",
		"graph-match":                    "ExposureGraphEdges\n| make-graph SourceNodeId --> TargetNodeId with ExposureGraphNodes on NodeId\n| graph-match (SourceNode)-[anyEdge*1..3]->(TargetNode)\n    project IncomingNodeName = SourceNode.NodeName,\n    Edges = anyEdge.EdgeLabel\n| where isnotempty(IncomingNodeName)",
		"nested union":                   "union (union (A | where x == 1), (B | where y == 1)), (C | where z == 1)",
		"reserved words in brackets":     `T | where ['limit'] == 100 and host['scan'].vuln == 1`,
		"with_source":                    "union with_source=TableName SecurityEvent, WindowsEvent | where EventID == 4625",
	} {
		t.Run(name, func(t *testing.T) {
			if errs := Validate(query); len(errs) > 0 {
				t.Fatalf("Validate() = %q, want no errors", errs)
			}
		})
	}
}

func TestValidate_RejectsNonQueries(t *testing.T) {
	for name, query := range map[string]string{
		"markdown write-up":    "# Golden Ticket Detection\n## Description\nDetects forged tickets.\n## Query\nSecurityEvent | where EventID == 4769",
		"documentation text":   "Description: Detects forged tickets.\nQuery: SecurityEvent | where EventID == 4769",
		"two queries":          "SecurityEvent\n| where EventID == 4625\n\n\nSigninLogs\n| where ResultType != 0",
		"leading pipe":         "| where EventID == 4625",
		"logscale":             "#event_simpleName=ProcessRollup2 ParentBaseFileName=/notepad.exe/i",
		"suricata":             `alert dns $HOME_NET any -> any any (msg:"ET MALWARE"; dns.query; content:".me"; sid:1;)`,
		"yara":                 "rule Example {\n  strings:\n    $a = \"evil\"\n  condition:\n    $a\n}",
		"curly quotes":         `DeviceProcessEvents | where FileName in~ (“brave.exe”, “opera.exe”)`,
		"workbook parameter":   `SecurityEvent | where TimeGenerated {TimeRange}`,
		"escaped newline text": `SecurityEvent\r\n| where EventID == 4625`,
		"unknown operator":     `DeviceFileEvents | where FileName has_suffix ".exe"`,
	} {
		t.Run(name, func(t *testing.T) {
			if errs := Validate(query); len(errs) == 0 {
				t.Fatal("Validate() reported no errors")
			}
		})
	}
}

func TestValidate_ShortensExpectedTokenLists(t *testing.T) {
	errs := Validate("SecurityEvent | where")
	if len(errs) == 0 {
		t.Fatal("Validate() reported no errors")
	}
	for _, msg := range errs {
		if strings.Count(msg, ",") > 10 {
			t.Fatalf("error lists every expected token: %q", msg)
		}
	}
}
