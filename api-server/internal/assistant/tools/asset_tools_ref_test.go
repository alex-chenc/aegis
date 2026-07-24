package tools

import (
	"strings"
	"testing"

	"api-server/internal/assistant"
)

// TestAssetCollectionTriggerDeclaresOperationRefFields verifies that
// Asset.Collection.Trigger declares task_id as an OperationRefField so the
// runtime can deterministically bind it to a downstream Asset.Collection.Get
// step as a previous_step argument.
func TestAssetCollectionTriggerDeclaresOperationRefFields(t *testing.T) {
	registry := assistant.NewToolRegistry()
	if err := RegisterAssetTools(registry, AssetToolDeps{}); err != nil {
		t.Fatal(err)
	}
	spec, ok := registry.Get("Asset.Collection.Trigger")
	if !ok {
		t.Fatal("Asset.Collection.Trigger not registered")
	}
	refs := spec.ResultContract.OperationRefFields
	var found bool
	for _, ref := range refs {
		if strings.EqualFold(ref, "task_id") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected OperationRefFields to include task_id, got %v", refs)
	}
}
