package colony_test

import (
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
)

func TestLoadBeeCompletionContractIntentOverride(t *testing.T) {
	root := t.TempDir()
	writeBeeYAML(t, root, "archivist", `role: archivist
adapter: cursor
prompt_template: archivist.md
default_intent: task
intents:
  - task
  - skill
completion_contract:
  required:
    - type: INSIGHT
      kind_one_of:
        - context.note
  intents:
    skill:
      required:
        - type: INSIGHT
          kind_one_of:
            - skill.proposed
`)

	bee, _, err := colony.LoadBee(root, "archivist")
	if err != nil {
		t.Fatal(err)
	}
	def := bee.CompletionContract.ForIntent("task")
	if len(def.Required) != 1 || def.Required[0].KindOneOf[0] != "context.note" {
		t.Fatalf("task contract = %+v, want the default note contract", def)
	}
	skill := bee.CompletionContract.ForIntent("skill")
	if len(skill.Required) != 1 || skill.Required[0].KindOneOf[0] != "skill.proposed" {
		t.Fatalf("skill contract = %+v, want the proposal override", skill)
	}
}

func TestLoadBeeCompletionContractWithoutOverrides(t *testing.T) {
	root := t.TempDir()
	writeBeeYAML(t, root, "guard", `role: guard
adapter: cursor
prompt_template: guard.md
completion_contract:
  required:
    - type: VERIFICATION
      kind_one_of:
        - verification.success
        - verification.failed
`)

	bee, _, err := colony.LoadBee(root, "guard")
	if err != nil {
		t.Fatal(err)
	}
	if len(bee.CompletionContract.Intents) != 0 {
		t.Fatalf("intents = %+v, want none", bee.CompletionContract.Intents)
	}
	if got := bee.CompletionContract.ForIntent("anything"); len(got.Required) != 1 {
		t.Fatalf("default contract = %+v, want the declared rule", got)
	}
}

func TestLoadBeeRejectsInvalidCompletionContractIntent(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "empty intent key",
			body: `completion_contract:
  intents:
    " ":
      required:
        - type: INSIGHT
          kind_one_of: [context.note]
`,
			want: "intent key is required",
		},
		{
			name: "override without rules",
			body: `completion_contract:
  intents:
    skill: {}
`,
			want: "at least one rule is required",
		},
		{
			name: "invalid override rule",
			body: `completion_contract:
  intents:
    skill:
      required:
        - type: BOGUS
          kind_one_of: [skill.proposed]
`,
			want: "invalid type",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeBeeYAML(t, root, "archivist", "role: archivist\nadapter: cursor\nprompt_template: archivist.md\n"+tc.body)
			_, _, err := colony.LoadBee(root, "archivist")
			if err == nil {
				t.Fatal("expected load error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
