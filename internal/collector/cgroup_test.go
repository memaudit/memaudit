// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"strings"
	"testing"

	"github.com/memaudit/memaudit/internal/k8s"
)

var defaultCgroupGlobs = []string{"system.slice/*.service", "kubepods.slice/**"}

func TestCgroupCollectGolden(t *testing.T) {
	got, err := NewCgroup("../../testdata/cgroup-v2-k8s/sys", defaultCgroupGlobs, 500, nil).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	assertGoldenJSON(t, "../../testdata/cgroup-v2-k8s/expected/cgroup_mem.json", got)

	for _, rec := range got {
		if rec.K8sPod != "" || rec.K8sNamespace != "" || rec.K8sContainerID != "" || rec.K8sLabels != nil {
			t.Errorf("cgroup %q: got k8s enrichment with a nil client, want none: %+v", rec.Cgroup, rec)
		}
	}
}

// TestCgroupCollectPrunesUnmatchedSubtrees proves user.slice (and
// everything under it) is never selected — it doesn't match either
// default glob at any prefix, so the walker shouldn't even descend into
// it. The fixture's user.slice/user-1000.slice/session-1.scope carries a
// memory.current file specifically so a pruning regression would show
// up here as an unexpected extra record.
func TestCgroupCollectPrunesUnmatchedSubtrees(t *testing.T) {
	got, err := NewCgroup("../../testdata/cgroup-v2-k8s/sys", defaultCgroupGlobs, 500, nil).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	for _, rec := range got {
		if rec.Cgroup == "user.slice" || strings.HasPrefix(rec.Cgroup, "user.slice/") {
			t.Fatalf("got a record under user.slice, want it pruned entirely: %q", rec.Cgroup)
		}
	}
}

func TestCgroupCollectTruncatesAtMax(t *testing.T) {
	got, err := NewCgroup("../../testdata/cgroup-v2-k8s/sys", defaultCgroupGlobs, 2, nil).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want exactly 2 (max)", len(got))
	}
}

func TestCgroupCollectV1HostIsNilNotError(t *testing.T) {
	got, err := NewCgroup("../../testdata/edge-cases/cgroup-v1-host/sys", defaultCgroupGlobs, 500, nil).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}

func TestCgroupCollectNoCgroupfsIsNilNotError(t *testing.T) {
	got, err := NewCgroup("../../testdata/edge-cases/vmstat-old-kernel/sys", defaultCgroupGlobs, 500, nil).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}

// fakeK8sClient is a minimal k8s.Client test double: no HTTP, no
// caching, just a fixed uid -> PodMeta map.
type fakeK8sClient struct {
	pods map[string]k8s.PodMeta
}

func (f *fakeK8sClient) LookupPod(_ context.Context, uid string) (k8s.PodMeta, bool, error) {
	meta, ok := f.pods[uid]
	return meta, ok, nil
}

// TestCgroupCollectK8sEnrichment proves enrichment only attaches to
// cgroups whose path resolves to a pod UID (and container ID, one level
// deeper), and leaves everything else — including the kubepods.slice and
// kubepods-burstable.slice aggregates, which carry no pod UID segment —
// untouched even with a client configured.
func TestCgroupCollectK8sEnrichment(t *testing.T) {
	const uid = "1234abcd-5678-90ef-ab12-cdef34567890"
	const containerID = "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	client := &fakeK8sClient{pods: map[string]k8s.PodMeta{
		uid: {Namespace: "default", Name: "web-0", Labels: map[string]string{"app": "web"}},
	}}

	got, err := NewCgroup("../../testdata/cgroup-v2-k8s/sys", defaultCgroupGlobs, 500, client).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	for _, rec := range got {
		switch rec.Cgroup {
		case "kubepods.slice", "kubepods.slice/kubepods-burstable.slice":
			if rec.K8sPod != "" || rec.K8sNamespace != "" || rec.K8sContainerID != "" {
				t.Errorf("cgroup %q: got enrichment %+v, want none (no pod UID in path)", rec.Cgroup, rec)
			}
		case "kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod1234abcd_5678_90ef_ab12_cdef34567890.slice":
			if rec.K8sNamespace != "default" || rec.K8sPod != "web-0" {
				t.Errorf("pod slice: got namespace=%q pod=%q, want default/web-0", rec.K8sNamespace, rec.K8sPod)
			}
			if rec.K8sContainerID != "" {
				t.Errorf("pod slice: got container ID %q, want none (no scope segment at this level)", rec.K8sContainerID)
			}
			if rec.K8sLabels["app"] != "web" {
				t.Errorf("pod slice: got labels %v, want app=web", rec.K8sLabels)
			}
		case "kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod1234abcd_5678_90ef_ab12_cdef34567890.slice/cri-containerd-" + containerID + ".scope":
			if rec.K8sNamespace != "default" || rec.K8sPod != "web-0" {
				t.Errorf("scope: got namespace=%q pod=%q, want default/web-0", rec.K8sNamespace, rec.K8sPod)
			}
			if rec.K8sContainerID != containerID {
				t.Errorf("scope: got container ID %q, want %q", rec.K8sContainerID, containerID)
			}
		}
	}
}
