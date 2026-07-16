package colony_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
)

func writeBeeYAML(t *testing.T, root, role, content string) {
	t.Helper()
	dir := filepath.Join(root, ".paseka", "bees")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, role+".yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadBeeSubscribesPublishes(t *testing.T) {
	root := t.TempDir()
	writeBeeYAML(t, root, "builder", `role: builder
adapter: cursor
prompt_template: builder.md
subscribes:
  - type: SIGNAL
    kind: task.ready
    dispatch: task
  - type: MUTATION
    kind: code.proposal
    dispatch: direct
publishes:
  - type: MUTATION
    kind: code.proposal
`)

	bee, _, err := colony.LoadBee(root, "builder")
	if err != nil {
		t.Fatal(err)
	}
	if len(bee.Subscribes) != 2 {
		t.Fatalf("subscribes = %d, want 2", len(bee.Subscribes))
	}
	if bee.Subscribes[0].Dispatch != colony.DispatchTask {
		t.Fatalf("dispatch = %q", bee.Subscribes[0].Dispatch)
	}
	if len(bee.Publishes) != 1 {
		t.Fatalf("publishes = %d, want 1", len(bee.Publishes))
	}
}

func TestLoadBeeBackwardCompatibleWithoutRules(t *testing.T) {
	root := t.TempDir()
	writeBeeYAML(t, root, "builder", `role: builder
adapter: cursor
prompt_template: builder.md
`)

	bee, _, err := colony.LoadBee(root, "builder")
	if err != nil {
		t.Fatal(err)
	}
	if len(bee.Subscribes) != 0 || len(bee.Publishes) != 0 {
		t.Fatalf("expected empty rules, got subscribes=%d publishes=%d", len(bee.Subscribes), len(bee.Publishes))
	}
	if !bee.CanHandleTaskReady() {
		t.Fatal("empty subscribes should allow task.ready dispatch")
	}
}

func TestLoadBeeRejectsInvalidEventType(t *testing.T) {
	root := t.TempDir()
	writeBeeYAML(t, root, "builder", `role: builder
adapter: cursor
subscribes:
  - type: LOG
    kind: noise
`)

	_, _, err := colony.LoadBee(root, "builder")
	if err == nil {
		t.Fatal("expected error for non-domain event type")
	}
}

func TestCanHandleTaskReadyWithRules(t *testing.T) {
	bee := colony.Bee{
		Role: "guard",
		Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}, Dispatch: colony.DispatchDirect},
		},
	}
	if bee.CanHandleTaskReady() {
		t.Fatal("guard without task.ready subscribe should not handle task.ready")
	}

	builder := colony.Bee{
		Role: "builder",
		Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: string(protocol.TaskEventReady)}, Dispatch: colony.DispatchTask},
		},
	}
	if !builder.CanHandleTaskReady() {
		t.Fatal("builder with task.ready subscribe should handle task.ready")
	}
}

func TestDirectSubscribers(t *testing.T) {
	bees := map[string]colony.Bee{
		"guard": {
			Role: "guard",
			Subscribes: []colony.SubscriptionRule{
				{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}, Dispatch: colony.DispatchDirect},
			},
		},
		"builder": {
			Role: "builder",
			Subscribes: []colony.SubscriptionRule{
				{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
			},
		},
	}
	roles := colony.DirectSubscribers(bees, protocol.EventMutation, "code.proposal")
	if len(roles) != 1 || roles[0] != "guard" {
		t.Fatalf("direct subscribers = %v, want [guard]", roles)
	}
}

func TestDeclaresPublishAdvisory(t *testing.T) {
	bee := colony.Bee{
		Role: "builder",
		Publishes: []colony.PublicationRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}},
		},
	}
	if !bee.DeclaresPublish(protocol.EventMutation, "code.proposal") {
		t.Fatal("expected declared publish")
	}
	if bee.DeclaresPublish(protocol.EventInsight, "task.plan") {
		t.Fatal("unexpected declared publish")
	}
	if len(bee.Publishes) > 0 && bee.DeclaresPublish(protocol.EventInsight, "task.plan") {
		t.Fatal("undeclared publish should be false when publishes list is non-empty")
	}
}

func TestAnyBeeDeclaresPublishExplicitOnly(t *testing.T) {
	bees := map[string]colony.Bee{
		"scout": {Role: "scout"},
		"receiver": {
			Role: "receiver",
			Publishes: []colony.PublicationRule{
				{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: string(protocol.TaskEventCompleted)}},
			},
		},
	}
	if !colony.AnyBeeDeclaresPublish(bees, protocol.EventVerification, string(protocol.TaskEventCompleted)) {
		t.Fatal("expected receiver to declare task.completed")
	}
	if colony.AnyBeeDeclaresPublish(bees, protocol.EventMutation, string(protocol.MutationCodeProposal)) {
		t.Fatal("empty publishes must not count as explicit declaration")
	}
	if colony.AnyBeeDeclaresPublish(map[string]colony.Bee{"scout": {Role: "scout"}}, protocol.EventVerification, string(protocol.TaskEventCompleted)) {
		t.Fatal("expected false when no bee declares publish")
	}
}

func TestExplicitlyDeclaresPublish(t *testing.T) {
	bee := colony.Bee{
		Role: "builder",
		Publishes: []colony.PublicationRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}},
		},
	}
	if !bee.ExplicitlyDeclaresPublish(protocol.EventMutation, "code.proposal") {
		t.Fatal("expected explicit declaration")
	}
	if bee.ExplicitlyDeclaresPublish(protocol.EventVerification, string(protocol.TaskEventCompleted)) {
		t.Fatal("unexpected declaration")
	}
	if (colony.Bee{Role: "scout"}).ExplicitlyDeclaresPublish(protocol.EventMutation, "code.proposal") {
		t.Fatal("empty publishes must be false for explicit check")
	}
}

func TestLoadAllBees(t *testing.T) {
	root := t.TempDir()
	writeBeeYAML(t, root, "builder", `role: builder
adapter: cursor
`)
	writeBeeYAML(t, root, "guard", `role: guard
adapter: cursor
`)

	bees, err := colony.LoadAllBees(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(bees) != 2 {
		t.Fatalf("bees = %d, want 2", len(bees))
	}
}
