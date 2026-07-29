// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package k8s

import (
	"regexp"
	"strings"
)

// podSliceRE matches the systemd cgroup driver's kubepods slice naming:
// kubepods-<qos>-pod<uid>.slice. <qos> (burstable/besteffort) is absent
// for the Guaranteed QoS class, and <uid> uses "_" in place of "-".
var podSliceRE = regexp.MustCompile(`^kubepods-(?:[a-z]+-)?pod([0-9a-f_]+)\.slice$`)

// containerScopeRE matches the three container-runtime scope naming
// schemes memaudit supports.
var containerScopeRE = regexp.MustCompile(`^(?:cri-containerd|crio|docker)-([0-9a-f]+)\.scope$`)

// PodUID extracts a pod UID from a cgroup path by scanning its segments
// for a kubepods slice, e.g.
// "kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod1234abcd_5678_90ef_ab12_cdef34567890.slice"
// yields "1234abcd-5678-90ef-ab12-cdef34567890". Returns ("", false) if
// no segment matches.
func PodUID(cgroupPath string) (string, bool) {
	for seg := range strings.SplitSeq(cgroupPath, "/") {
		m := podSliceRE.FindStringSubmatch(seg)
		if m == nil {
			continue
		}
		return strings.ReplaceAll(m[1], "_", "-"), true
	}
	return "", false
}

// ContainerID extracts a container ID from a cgroup path's
// cri-containerd/crio/docker scope segment. Returns ("", false) if no
// segment matches.
func ContainerID(cgroupPath string) (string, bool) {
	for seg := range strings.SplitSeq(cgroupPath, "/") {
		m := containerScopeRE.FindStringSubmatch(seg)
		if m == nil {
			continue
		}
		return m[1], true
	}
	return "", false
}
