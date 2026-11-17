package kql

import (
	"context"
	"strings"
	"testing"
)

// These exact finite controls caught ordinary extraction and semantic support
// lost when additive provenance spent the preexisting output allowance.
const boundedMetadataID44 = "Event\n| where (((TerminalSessionId contains \"4625\" or TerminalSessionId contains \"22\") or TerminalSessionId matches regex \"(?i)diskshadow.*\\\\\\\\taskhost\\\\.exe\") and EventID in~ (\"C:\\\\Users\\\\Default\\\\\", \"4697\", @'\\wmic.exe') and (SubStatus in~ (\"Net.WebClient\", @'\\powershell.exe') or SubStatus matches regex \"(?i)\\\\\\\\whoami\\\\.exe.*\\\\\\\\wmic\\\\.exe\") and not(((ProcessName endswith \"-enc\" or ProcessName endswith \"-ExecutionPolicy Bypass\" or ProcessName endswith \"4688\") and (ProviderName matches regex \"(?i)\\\\\\\\mshta\\\\.exe.*net group\" or ProviderName in~ (\"C:\\\\Program Files\\\\\", \"4657\")) and IPAddress in~ (@'\\explorer.exe', \"4724\", \"C:\\\\Windows\\\\WinSxS\\\\\"))) and not((DestinationIP contains \"4444\" or DestinationIP contains \"FromBase64String\" or DestinationIP contains \"4688\")) and not((Direction matches regex \"(?i)-ExecutionPolicy Bypass.*5985|4688.*4657\" or Direction =~ \"4732\")) and not((TargetDomainName in~ (\"iwr \", \"22\", \"4728\") and RelativeTargetName in~ (\"Invoke-Expression\", \"ntdsutil\", @'\\regsvr32.exe') and ((cmd_line endswith \"4720\" or cmd_line endswith \"sekurlsa\") or cmd_line matches regex \"(?i)nltest /dclist.*admin\") and TerminalSessionId in~ (\"0x40000000\", \"net user\", \"High\"))))"
const boundedMetadataSingleSource593 = "Syslog\n| where ((AccountName contains \"whoami\" and AccountName contains \"4663\" and AccountName contains \"\\\\mshta.exe\" and AccountName contains \"wbadmin\") or LogonType in~ (\"DownloadString\", \"11\") or InitiatingProcessParentCommandLine in~ (\"4657\", \"IEX\", \"0x40000000\", @'\\net.exe', \"1102\", \"C:\\\\Users\\\\Public\\\\\", \"\\\\AppData\\\\Local\\\\Temp\\\\\") or InitiatingProcessVersionInfoProductName in~ (\"shadow\", @'\\certutil.exe') or (CallerProcessName in~ (\"3\", \"4616\") and not(TaskValue in~ (\"C:\\\\Windows\\\\WinSxS\\\\\", \"wbadmin\", @'\\mshta.exe', \"4648\", \"sekurlsa\", \"4104\", @'\\bitsadmin.exe', \"C:\\\\Temp\\\\\"))))"

func TestBoundedMetadataPreservesDefaultSupport(t *testing.T) {
	t.Run("exact ID44 extraction", func(t *testing.T) {
		got := ExtractConditions(boundedMetadataID44)
		if len(got.Errors) != 0 || len(got.Conditions) != 22 || got.Expression == nil || got.Dataflow == nil || !got.Dataflow.Complete {
			t.Fatalf("ordinary extraction lost support: %+v", got)
		}
		if len(got.Dataflow.Stages) != 1 || got.Dataflow.Stages[0].Source != strings.SplitN(boundedMetadataID44, "| ", 2)[1] {
			t.Fatalf("authored stage source changed: %+v", got.Dataflow)
		}
	})
	t.Run("exact 593 byte semantic control", func(t *testing.T) {
		got := AnalyzeQuery(boundedMetadataSingleSource593)
		if len(got.Errors) != 0 || len(got.Conditions) != 9 || got.Expression == nil || got.Dataflow == nil || !got.Dataflow.Complete {
			t.Fatalf("ordinary analysis lost support: %+v", got)
		}
		if got.Expression.Operator != BooleanOr || len(got.Expression.Children) != 5 || got.Expression.Children[0].Operator != BooleanAnd || len(got.Expression.Children[0].Children) != 4 || got.Expression.Children[4].Operator != BooleanAnd || len(got.Expression.Children[4].Children) != 2 {
			t.Fatalf("boolean grouping changed: %+v", got.Expression)
		}
		leaf := got.Expression.Children[0].Children[0].Condition
		if leaf == nil || leaf.FieldBinding == nil || got.Conditions[0].FieldBinding == nil || leaf.FieldBinding == got.Conditions[0].FieldBinding {
			t.Fatalf("semantic field references do not own detached copies: %+v", got)
		}
		leaf.FieldBinding.ScopeID = "changed"
		if got.Conditions[0].FieldBinding.ScopeID != "s0" {
			t.Fatal("leaf mutation reached top-level condition")
		}
	})
}

func metadataBudgetFailure(t *testing.T, want string, run func()) {
	t.Helper()
	defer func() {
		value := recover()
		abort, ok := value.(budgetAbort)
		if !ok || abort.err == nil || !strings.Contains(abort.err.Error(), want) {
			t.Fatalf("want %s budget failure, got %v", want, value)
		}
	}()
	run()
}

func TestMetadataReservationBoundariesAreAtomic(t *testing.T) {
	limits := defaultParseLimits()
	limits.maxMetadataItems, limits.maxMetadataBytes = 2, 3
	b, cancel := newParseBudget(context.Background(), limits)
	defer cancel()
	b.reserveMetadata(1, 2)
	b.reserveMetadata(1, 1)
	metadataBudgetFailure(t, "metadata items", func() { b.reserveMetadata(1, 0) })
	metadataBudgetFailure(t, "metadata bytes", func() { b.reserveMetadata(0, 1) })
	if b.metadataItems != 2 || b.metadataBytes != 3 || b.output != 0 {
		t.Fatalf("failed reservations changed counters: %+v", b)
	}
	// An overflowing addition and invalid negative counts must not wrap around
	// the subtraction checks or replenish either allowance.
	metadataBudgetFailure(t, "metadata items", func() { b.reserveMetadata(int(^uint(0)>>1), 0) })
	metadataBudgetFailure(t, "metadata bytes", func() { b.reserveMetadata(0, int(^uint(0)>>1)) })
	metadataBudgetFailure(t, "metadata items", func() { b.reserveMetadata(-1, 0) })
	metadataBudgetFailure(t, "metadata bytes", func() { b.reserveMetadata(0, -1) })
	if b.metadataItems != 2 || b.metadataBytes != 3 {
		t.Fatalf("overflow changed counters: %+v", b)
	}
}

func TestMetadataReferenceClonesChargeEachOwner(t *testing.T) {
	for _, tc := range []struct {
		name, want   string
		items, bytes int
	}{
		{"items", "metadata items", 1, 18},
		{"bytes", "metadata bytes", 2, 17},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := defaultParseLimits()
			limits.maxMetadataItems, limits.maxMetadataBytes = tc.items, tc.bytes
			b, cancel := newParseBudget(context.Background(), limits)
			defer cancel()
			ref := &FieldBindingRef{ScopeID: "s0", Kind: "input", BindingID: "b0"}
			copy := cloneFieldBindingRef(ref, b)
			if copy == ref || *copy != *ref {
				t.Fatalf("reference copy is not independent: %+v", copy)
			}
			metadataBudgetFailure(t, tc.want, func() { cloneFieldBindingRef(ref, b) })
			if b.metadataItems != 1 || b.metadataBytes != 9 || b.output != 0 {
				t.Fatalf("reference bytes or failed reservation charged incorrectly: %+v", b)
			}
		})
	}
}

func TestMetadataNestedEmptyInputsConsumeSlots(t *testing.T) {
	limits := defaultParseLimits()
	limits.maxMetadataItems, limits.maxMetadataBytes = 4, 0
	b, cancel := newParseBudget(context.Background(), limits)
	defer cancel()
	flow := &QueryDataflow{Stages: []DataflowStage{{Inputs: []ScopeReference{{}, {}}}}}
	got := cloneQueryDataflow(flow, b)
	if got == flow || len(got.Stages[0].Inputs) != 2 || b.metadataItems != 4 {
		t.Fatalf("nested empty records escaped structural accounting: %+v", b)
	}
	limits.maxMetadataItems = 3
	b2, cancel2 := newParseBudget(context.Background(), limits)
	defer cancel2()
	metadataBudgetFailure(t, "metadata items", func() { cloneQueryDataflow(flow, b2) })
}

func TestMetadataPublicExhaustionReturnsOnlyErrors(t *testing.T) {
	for _, tc := range []struct {
		name, want   string
		items, bytes int
	}{
		{"items", "metadata items", 1, 1 << 20},
		{"bytes", "metadata bytes", 4096, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := defaultParseLimits()
			limits.maxMetadataItems, limits.maxMetadataBytes = tc.items, tc.bytes
			parsed := extractConditionsContext(context.Background(), "T | where x == 1", limits)
			if len(parsed.Errors) != 1 || !strings.Contains(parsed.Errors[0], tc.want) || parsed.Dataflow != nil || parsed.Expression != nil || len(parsed.Conditions) != 0 {
				t.Fatalf("partial extraction: %+v", parsed)
			}
			semantic := analyzeQueryContext(context.Background(), "T | where x == 1", limits)
			if len(semantic.Errors) != 1 || !strings.Contains(semantic.Errors[0], tc.want) || semantic.Dataflow != nil || semantic.Expression != nil || len(semantic.Conditions) != 0 {
				t.Fatalf("partial semantic model: %+v", semantic)
			}
		})
	}
}

func TestMetadataAccountingRetainsLegacyOperandLimit(t *testing.T) {
	query := `T | where x == "` + strings.Repeat("a", 1500) + `"`
	got := ExtractConditions(query)
	if len(got.Errors) != 1 || !strings.Contains(got.Errors[0], "output items") || len(got.Conditions) != 0 || got.Dataflow != nil {
		t.Fatalf("legacy operand-heavy admission relaxed: %+v", got)
	}
}

func TestMetadataReservationChecksCancellation(t *testing.T) {
	ctx, cancelContext := context.WithCancel(context.Background())
	b, cancel := newParseBudget(ctx, defaultParseLimits())
	defer cancel()
	cancelContext()
	metadataBudgetFailure(t, "context canceled", func() { b.reserveMetadata(1, 1) })
	if b.metadataItems != 0 || b.metadataBytes != 0 {
		t.Fatal("canceled reservation grew metadata")
	}
}

func TestMetadataIdentityKeepsLegacyKeyBytes(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		value                      *Operand
		legacyBytes, metadataBytes int
	}{
		{"literal nil tag", &Operand{Kind: OperandString, Source: `"a"`, Value: "a"}, 23, 1},
		{"field reference", &Operand{Kind: OperandField, Source: "y", Value: "y", FieldBinding: &FieldBindingRef{ScopeID: "s0", Kind: "input"}}, 20, 14},
		{"nested tags", &Operand{Kind: OperandList, Items: []Operand{{Kind: OperandString, Source: `"a"`, Value: "a"}}}, 40, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key, added := operandIdentityParts(tc.value, nil)
			if added != tc.metadataBytes || len(key)-added != tc.legacyBytes {
				t.Fatalf("identity moved legacy bytes: key=%q legacy=%d metadata=%d", key, len(key)-added, added)
			}
		})
	}
	// The independent pre-wave2 diagnostic measured this exact extraction at
	// 2677 legacy units. Reference tags must not silently raise or lower it.
	b, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	got := extractConditionsInternal(boundedMetadataID44, b)
	if len(got.Errors) != 0 || b.output != 2677 {
		t.Fatalf("legacy key/operand accounting shifted: output=%d errors=%v", b.output, got.Errors)
	}
}

func TestMetadataCloneIDsRemainBounded(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(*parseBudget)
	}{
		{"condition", func(b *parseBudget) { cloneSemanticCondition(Condition{StageID: "t0", InputScopeID: "s0"}, b) }},
		{"scalar", func(b *parseBudget) { cloneScalarPredicate(&ScalarPredicate{StageID: "t0", InputScopeID: "s0"}, b) }},
		{"search", func(b *parseBudget) { cloneSearchPredicate(&SearchPredicate{StageID: "t0", InputScopeID: "s0"}, b) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := defaultParseLimits()
			limits.maxMetadataBytes = 3
			b, cancel := newParseBudget(context.Background(), limits)
			defer cancel()
			metadataBudgetFailure(t, "metadata bytes", func() { tc.run(b) })
			if b.metadataBytes != 0 {
				t.Fatal("failed ID copy partially charged bytes")
			}
		})
	}
}

func TestBoundedMetadataRetainsLegacyDefaultBoundaries(t *testing.T) {
	b, cancel := newParseBudget(context.Background(), defaultParseLimits())
	defer cancel()
	b.input(strings.Repeat("x", 128<<10))
	metadataBudgetFailure(t, "input bytes", func() { b.input(strings.Repeat("x", (128<<10)+1)) })
	b.reserveOutput(4096)
	metadataBudgetFailure(t, "output items", func() { b.reserveOutput(1) })
	b.reserveMetadata(4096, 1<<20)
	metadataBudgetFailure(t, "metadata items", func() { b.reserveMetadata(1, 0) })
	metadataBudgetFailure(t, "metadata bytes", func() { b.reserveMetadata(0, 1) })
	for range 1000000 {
		b.step()
	}
	metadataBudgetFailure(t, "work budget", func() { b.step() })

	for _, tc := range []struct{ name, query, want string }{
		{"nesting", "T | where " + strings.Repeat("(", 17) + "x==1" + strings.Repeat(")", 17), "nesting depth"},
		{"tokens", strings.Repeat("x ", 4097), "token count"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed := ExtractConditions(tc.query)
			if len(parsed.Errors) != 1 || !strings.Contains(parsed.Errors[0], tc.want) || parsed.Dataflow != nil || len(parsed.Conditions) != 0 {
				t.Fatalf("legacy boundary relaxed: %+v", parsed)
			}
		})
	}
}

func TestMetadataItemsAreSharedWithConversion(t *testing.T) {
	limits := defaultParseLimits()
	limits.maxMetadataItems = 12
	query := "T | where x == 1"
	parsed := extractConditionsContext(context.Background(), query, limits)
	if len(parsed.Errors) != 0 || len(parsed.Conditions) != 1 || parsed.Dataflow == nil {
		t.Fatalf("extraction must fit fixed item allowance: %+v", parsed)
	}
	fresh, cancel := newParseBudget(context.Background(), limits)
	defer cancel()
	if got := semanticFromExtraction(parsed, fresh); got == nil || len(got.Errors) != 0 || len(got.Conditions) != 1 || got.Dataflow == nil {
		t.Fatalf("fresh conversion must fit: %+v", got)
	}
	got := analyzeQueryContext(context.Background(), query, limits)
	if len(got.Errors) != 1 || !strings.Contains(got.Errors[0], "metadata items") || got.Expression != nil || got.Dataflow != nil || len(got.Conditions) != 0 {
		t.Fatalf("shared conversion reset metadata allowance: %+v", got)
	}
}

func TestRecursiveExtractionSharesMetadataItems(t *testing.T) {
	limits := defaultParseLimits()
	limits.maxMetadataItems = 12
	if child := extractConditionsContext(context.Background(), "U | where x == 1", limits); len(child.Errors) != 0 || child.Dataflow == nil || len(child.Conditions) != 1 {
		t.Fatalf("child alone must fit fixed allowance: %+v", child)
	}
	got := extractConditionsContext(context.Background(), "T | join (U | where x == 1) on k", limits)
	if len(got.Errors) != 1 || !strings.Contains(got.Errors[0], "metadata items") || got.Dataflow != nil || got.Expression != nil || len(got.Conditions) != 0 || len(got.Joins) != 0 {
		t.Fatalf("recursive metadata reset or partial child escaped: %+v", got)
	}
}
