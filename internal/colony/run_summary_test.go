package colony

import "testing"

func TestValidateRunSummaryPolicy(t *testing.T) {
	tests := []struct {
		name    string
		policy  RunSummaryPolicy
		wantErr bool
	}{
		{name: "empty", policy: "", wantErr: false},
		{name: "auto", policy: RunSummaryAuto, wantErr: false},
		{name: "required", policy: RunSummaryRequired, wantErr: false},
		{name: "disabled", policy: RunSummaryDisabled, wantErr: false},
		{name: "invalid", policy: "maybe", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bee := Bee{Role: "builder", RunSummary: tt.policy}
			err := bee.ValidateRunSummaryPolicy()
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolvedRunSummaryPolicyDefaultsAuto(t *testing.T) {
	bee := Bee{Role: "builder"}
	if bee.ResolvedRunSummaryPolicy() != RunSummaryAuto {
		t.Fatalf("policy = %q", bee.ResolvedRunSummaryPolicy())
	}
}
