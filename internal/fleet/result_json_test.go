package fleet

import (
	"encoding/json"
	"testing"
)

func TestDeployResultMarshalJSONIncludesCompileStrictFields(t *testing.T) {
	raw, err := json.Marshal(DeployResult{
		Repo:                   "x/y",
		CompileStrictApplied:   true,
		CompileStrictEffective: true,
		CompileStrictSource:    CompileStrictSourceAutoPublic,
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v; raw=%s", err, raw)
	}
	assertCompileStrictJSONFields(t, got, true, true, CompileStrictSourceAutoPublic)
}

func TestDeployResultMarshalJSONIncludesCompileStrictZeroFields(t *testing.T) {
	raw, err := json.Marshal(DeployResult{Repo: "x/y"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v; raw=%s", err, raw)
	}
	assertCompileStrictJSONFields(t, got, false, false, "")
}

func TestSyncResultMarshalJSONIncludesNestedDeployCompileStrictFields(t *testing.T) {
	raw, err := json.Marshal(SyncResult{
		Repo: "x/y",
		Deploy: &DeployResult{
			Repo:                   "x/y",
			CompileStrictApplied:   true,
			CompileStrictEffective: true,
			CompileStrictSource:    CompileStrictSourceAutoPublic,
		},
		DeployPreflight: &DeployResult{
			Repo:                   "x/y",
			CompileStrictEffective: true,
			CompileStrictSource:    CompileStrictSourceAutoFallback,
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v; raw=%s", err, raw)
	}
	deploy, ok := got["deploy"].(map[string]any)
	if !ok {
		t.Fatalf("deploy = %T; want object in %v", got["deploy"], got)
	}
	assertCompileStrictJSONFields(t, deploy, true, true, CompileStrictSourceAutoPublic)
	preflight, ok := got["deploy_preflight"].(map[string]any)
	if !ok {
		t.Fatalf("deploy_preflight = %T; want object in %v", got["deploy_preflight"], got)
	}
	assertCompileStrictJSONFields(t, preflight, false, true, CompileStrictSourceAutoFallback)
}

func TestUpgradeResultMarshalJSONIncludesCompileStrictFields(t *testing.T) {
	raw, err := json.Marshal(UpgradeResult{
		Repo:                   "x/y",
		CompileStrictApplied:   true,
		CompileStrictEffective: true,
		CompileStrictSource:    CompileStrictSourceExplicit,
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v; raw=%s", err, raw)
	}
	assertCompileStrictJSONFields(t, got, true, true, CompileStrictSourceExplicit)
}

func assertCompileStrictJSONFields(
	t *testing.T, got map[string]any, wantApplied, wantEffective bool, wantSource string,
) {
	t.Helper()
	applied, ok := got["compile_strict_applied"].(bool)
	if !ok {
		t.Fatalf("compile_strict_applied missing or non-bool in %v", got)
	}
	if applied != wantApplied {
		t.Errorf("compile_strict_applied = %v; want %v", applied, wantApplied)
	}
	effective, ok := got["compile_strict_effective"].(bool)
	if !ok {
		t.Fatalf("compile_strict_effective missing or non-bool in %v", got)
	}
	if effective != wantEffective {
		t.Errorf("compile_strict_effective = %v; want %v", effective, wantEffective)
	}
	source, ok := got["compile_strict_source"].(string)
	if !ok {
		t.Fatalf("compile_strict_source missing or non-string in %v", got)
	}
	if source != wantSource {
		t.Errorf("compile_strict_source = %q; want %q", source, wantSource)
	}
}
