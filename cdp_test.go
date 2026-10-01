package browse

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/runtime"
	"github.com/go-json-experiment/json/jsontext"
)

// TestDecodeBoolObjectGuardsNilAndDecodes covers the issue #140 nil-object
// crash: CallFunctionOn can return a nil RemoteObject without an error, and the
// old callBoolOnNode dereferenced res.Value directly — a nil panic that, via the
// MCP panic hole, would take down the process. decodeBoolObject must fail closed
// on nil while still decoding real boolean results. Reachable without Chrome.
func TestDecodeBoolObjectGuardsNilAndDecodes(t *testing.T) {
	if _, err := decodeBoolObject(nil); err == nil {
		t.Fatal("nil result object must fail closed, not panic")
	}
	if v, err := decodeBoolObject(&runtime.RemoteObject{Value: jsontext.Value(`true`)}); err != nil || !v {
		t.Fatalf("true decode = %v, %v", v, err)
	}
	if v, err := decodeBoolObject(&runtime.RemoteObject{Value: jsontext.Value(`false`)}); err != nil || v {
		t.Fatalf("false decode = %v, %v", v, err)
	}
	if _, err := decodeBoolObject(&runtime.RemoteObject{Value: jsontext.Value(`"nope"`)}); err == nil {
		t.Fatal("non-boolean value must return a decode error")
	}
	if _, err := decodeBoolObject(&runtime.RemoteObject{}); err == nil {
		t.Fatal("empty value must return a decode error, not a false positive")
	}
}

func TestRequestContextCancelsWithParent(t *testing.T) {
	d := &CDPDriver{ctx: context.Background()}
	parent, cancelParent := context.WithCancel(context.Background())
	runCtx, cancelRun := d.requestContext(parent)
	defer cancelRun()
	cancelParent()
	select {
	case <-runCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("request-scoped CDP context did not cancel with parent")
	}
}

// TestDedupAXNodesDropsRepeatedIDs covers Chrome 154 reporting a pseudo-element
// InlineTextBox twice under one NodeID: the compressor rejects duplicate ids and
// the whole page snapshot failed closed. Order and first copies must survive.
func TestDedupAXNodesDropsRepeatedIDs(t *testing.T) {
	nodes := []*accessibility.Node{
		{NodeID: "1"},
		{NodeID: "-1000000003"},
		nil,
		{NodeID: "-1000000003"},
		{NodeID: "2"},
	}
	got := dedupAXNodes(nodes)
	want := []accessibility.NodeID{"1", "-1000000003", "2"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, n := range got {
		if n.NodeID != want[i] {
			t.Fatalf("node %d = %q, want %q", i, n.NodeID, want[i])
		}
	}
}
