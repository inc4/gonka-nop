package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/inc4/gonka-nop/internal/status"
)

const testNodeID = "node1"

func makeExistingNode() status.AdminNodesNodeInfo {
	return status.AdminNodesNodeInfo{
		ID:            testNodeID,
		Host:          "10.0.0.5",
		InferencePort: 5050,
		PoCPort:       8080,
		MaxConcurrent: 64,
		Models: map[string]status.AdminModelConfig{
			"Qwen/QwQ-32B": {
				Args: []string{
					"--gpu-memory-utilization", "0.90",
					"--tensor-parallel-size", "2",
					"--max-model-len", "24576",
				},
			},
		},
		Hardware: []status.AdminHardware{{Type: "NVIDIA GeForce RTX 4090", Count: 2}},
	}
}

func TestMergeNodeUpdate_NoFlags_IsIdentity(t *testing.T) {
	in := makeExistingNode()
	out, warns, err := mergeNodeUpdate(in, updateFlags{})
	if err != nil {
		t.Fatalf("mergeNodeUpdate: %v", err)
	}
	if len(warns) > 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("not identity:\nin:  %+v\nout: %+v", in, out)
	}
}

func TestMergeNodeUpdate_MaxConcurrentOverride(t *testing.T) {
	in := makeExistingNode()
	out, _, err := mergeNodeUpdate(in, updateFlags{MaxConcurrent: 128})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if out.MaxConcurrent != 128 {
		t.Errorf("MaxConcurrent = %d, want 128", out.MaxConcurrent)
	}
	// Other fields preserved.
	if out.Host != in.Host || out.InferencePort != in.InferencePort {
		t.Errorf("unrelated fields mutated: %+v", out)
	}
}

func TestMergeNodeUpdate_TPOverride_PatchesExistingModelArgs(t *testing.T) {
	in := makeExistingNode()
	out, _, err := mergeNodeUpdate(in, updateFlags{TP: 4})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	args := out.Models["Qwen/QwQ-32B"].Args
	// Find --tensor-parallel-size and check its value.
	for i, a := range args {
		if a == "--tensor-parallel-size" && i+1 < len(args) && args[i+1] != "4" {
			t.Errorf("TP = %s, want 4 (args: %v)", args[i+1], args)
		}
	}
}

func TestMergeNodeUpdate_ModelSwitchToKimi_RecipeArgs(t *testing.T) {
	in := makeExistingNode()
	out, warns, err := mergeNodeUpdate(in, updateFlags{Model: "moonshotai/Kimi-K2.6"})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if len(warns) > 0 {
		t.Errorf("unexpected warnings on recipe swap: %v", warns)
	}
	if _, ok := out.Models["moonshotai/Kimi-K2.6"]; !ok {
		t.Fatal("merged record does not contain Kimi model")
	}
	if _, ok := out.Models["Qwen/QwQ-32B"]; ok {
		t.Error("old model should have been replaced")
	}
	args := out.Models["moonshotai/Kimi-K2.6"].Args
	if len(args) != 20 {
		t.Errorf("Kimi args length = %d, want 20 (12 flag groups)", len(args))
	}
	if !contains(args, "--attention-backend") {
		t.Errorf("Kimi args missing --attention-backend: %v", args)
	}
}

func TestMergeNodeUpdate_VLLMArgsOverridesRecipe_WithWarning(t *testing.T) {
	in := makeExistingNode()
	out, warns, err := mergeNodeUpdate(in, updateFlags{
		Model:    "moonshotai/Kimi-K2.6",
		VLLMArgs: "--custom-arg foo",
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "--vllm-args overrides recipe") {
		t.Errorf("expected exactly 1 vllm-args-override warning, got: %v", warns)
	}
	args := out.Models["moonshotai/Kimi-K2.6"].Args
	if !reflect.DeepEqual(args, []string{"--custom-arg", "foo"}) {
		t.Errorf("expected only --custom-arg foo, got %v", args)
	}
}

func TestMergeNodeUpdate_UnknownModel(t *testing.T) {
	in := makeExistingNode()
	_, _, err := mergeNodeUpdate(in, updateFlags{Model: "not/a/model"})
	if err == nil {
		t.Fatal("expected error for unknown model")
	}
	if !strings.Contains(err.Error(), "not a governance-approved recipe") {
		t.Errorf("error should cite recipe registry: %v", err)
	}
}

func TestOverrideTP_ReplacesExisting(t *testing.T) {
	in := []string{"--gpu-memory-utilization", "0.90", "--tensor-parallel-size", "2", "--max-model-len", "8192"}
	got := overrideTP(in, 4)
	want := []string{"--gpu-memory-utilization", "0.90", "--tensor-parallel-size", "4", "--max-model-len", "8192"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestOverrideTP_InsertsWhenMissing(t *testing.T) {
	got := overrideTP([]string{"--enable-expert-parallel"}, 4)
	want := []string{"--enable-expert-parallel", "--tensor-parallel-size", "4"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestPutAdminNode_HappyPath(t *testing.T) {
	var receivedBody []byte
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		receivedPath = r.URL.Path
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	node := makeExistingNode()
	if err := putAdminNode(context.Background(), srv.URL, testNodeID, node); err != nil {
		t.Fatalf("putAdminNode: %v", err)
	}
	if receivedPath != "/admin/v1/nodes/"+testNodeID {
		t.Errorf("path = %q, want /admin/v1/nodes/%s", receivedPath, testNodeID)
	}
	var parsed status.AdminNodesNodeInfo
	if err := json.Unmarshal(receivedBody, &parsed); err != nil {
		t.Fatalf("body parse: %v", err)
	}
	if parsed.ID != testNodeID {
		t.Errorf("body.ID = %q, want %q", parsed.ID, testNodeID)
	}
}

func TestPutAdminNode_5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := putAdminNode(context.Background(), srv.URL, testNodeID, makeExistingNode())
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status code: %v", err)
	}
}

func TestListKnownNodeIDs(t *testing.T) {
	entries := []status.AdminNodesEntry{
		{Node: status.AdminNodesNodeInfo{ID: "node1"}},
		{Node: status.AdminNodesNodeInfo{ID: "node2"}},
	}
	got := listKnownNodeIDs(entries)
	if got != "node1, node2" {
		t.Errorf("got %q, want %q", got, "node1, node2")
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
