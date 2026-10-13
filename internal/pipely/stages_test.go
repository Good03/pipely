package pipely

import (
	"reflect"
	"testing"
)

func TestStageDependencies(t *testing.T) {
	stages, e := ParseStages(`stages:
- stage: Build
- stage: Test
- stage: Dev
- stage: Nonprod
- stage: Prod
- stage: Independent
  dependsOn: []
- stage: Audit
  dependsOn: Build
  isSkippable: false
`)
	if e != nil {
		t.Fatal(e)
	}
	included, skipped, e := ResolveStages(stages, []string{"Prod"})
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(included, []string{"Build", "Test", "Dev", "Nonprod", "Prod", "Audit"}) {
		t.Fatalf("included %v", included)
	}
	if !reflect.DeepEqual(skipped, []string{"Independent"}) {
		t.Fatalf("skipped %v", skipped)
	}
	inc, _, e := ResolveStages(stages, []string{"Independent"})
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(inc, []string{"Build", "Independent", "Audit"}) {
		t.Fatalf("independent selected: %v", inc)
	}
}
func TestInvalidExpandedGraphs(t *testing.T) {
	for _, text := range []string{
		"stages: [{stage: A, dependsOn: B}, {stage: B, dependsOn: A}]",
		"stages: [{stage: A, dependsOn: Missing}]",
		"stages: [{stage: A}, {stage: A}]",
		"stages: [{template: stages.yaml}]",
		"stages: [{stage: A, dependsOn: '${{ parameters.dep }}'}]",
		"stages: [{stage: A, isSkippable: '${{ parameters.skip }}'}]",
		"stages: unexpected",
	} {
		if _, e := ParseStages(text); e == nil {
			t.Errorf("accepted invalid graph %s", text)
		}
	}
}
func TestBranchingDependencies(t *testing.T) {
	stages, e := ParseStages(`stages:
- stage: Build
- stage: Unit
  dependsOn: Build
- stage: Integration
  dependsOn: Build
- stage: Prod
  dependsOn: [Unit, Integration]
- stage: Docs
  dependsOn: []
`)
	if e != nil {
		t.Fatal(e)
	}
	inc, _, e := ResolveStages(stages, []string{"Prod"})
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(inc, []string{"Build", "Unit", "Integration", "Prod"}) {
		t.Fatal(inc)
	}
	if _, _, e = ResolveStages(stages, []string{"prod"}); e == nil {
		t.Fatal("unknown stage accepted")
	}
}
func TestImplicitStage(t *testing.T) {
	s, e := ParseStages("steps:\n- script: echo hello\n")
	if e != nil || len(s) != 0 {
		t.Fatalf("%v %v", s, e)
	}
}

func TestParameterObjects(t *testing.T) {
	for _, text := range []string{"null", "[]", "true", "broken"} {
		if _, e := ParseParameters(text); e == nil {
			t.Errorf("accepted %s", text)
		}
	}
	if p, e := ParseParameters(`{"deploy":false,"count":2,"services":["orders"]}`); e != nil || p["deploy"] != false {
		t.Fatalf("%v %v", p, e)
	}
}
