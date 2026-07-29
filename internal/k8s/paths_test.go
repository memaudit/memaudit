// SPDX-FileCopyrightText: 2026 the memaudit authors
// SPDX-License-Identifier: Apache-2.0

package k8s

import "testing"

func TestPodUID(t *testing.T) {
	cases := []struct {
		name       string
		cgroupPath string
		wantUID    string
		wantOK     bool
	}{
		{
			name:       "burstable QoS",
			cgroupPath: "kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod1234abcd_5678_90ef_ab12_cdef34567890.slice/cri-containerd-abc123.scope",
			wantUID:    "1234abcd-5678-90ef-ab12-cdef34567890",
			wantOK:     true,
		},
		{
			name:       "besteffort QoS",
			cgroupPath: "kubepods.slice/kubepods-besteffort.slice/kubepods-besteffort-podaaaaaaaa_bbbb_cccc_dddd_eeeeeeeeeeee.slice",
			wantUID:    "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			wantOK:     true,
		},
		{
			name:       "guaranteed QoS has no qos infix",
			cgroupPath: "kubepods.slice/kubepods-pod11111111_2222_3333_4444_555555555555.slice",
			wantUID:    "11111111-2222-3333-4444-555555555555",
			wantOK:     true,
		},
		{
			name:       "non-kubepods cgroup",
			cgroupPath: "system.slice/ssh.service",
			wantUID:    "",
			wantOK:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotUID, gotOK := PodUID(tc.cgroupPath)
			if gotUID != tc.wantUID || gotOK != tc.wantOK {
				t.Errorf("PodUID(%q) = (%q, %v), want (%q, %v)", tc.cgroupPath, gotUID, gotOK, tc.wantUID, tc.wantOK)
			}
		})
	}
}

func TestContainerID(t *testing.T) {
	cases := []struct {
		name       string
		cgroupPath string
		wantID     string
		wantOK     bool
	}{
		{
			name:       "containerd",
			cgroupPath: "kubepods.slice/.../cri-containerd-abcdef1234567890.scope",
			wantID:     "abcdef1234567890",
			wantOK:     true,
		},
		{
			name:       "crio",
			cgroupPath: "kubepods.slice/.../crio-fedcba0987654321.scope",
			wantID:     "fedcba0987654321",
			wantOK:     true,
		},
		{
			name:       "docker",
			cgroupPath: "kubepods.slice/.../docker-1111222233334444.scope",
			wantID:     "1111222233334444",
			wantOK:     true,
		},
		{
			name:       "not a container scope",
			cgroupPath: "kubepods.slice/kubepods-burstable.slice",
			wantID:     "",
			wantOK:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotID, gotOK := ContainerID(tc.cgroupPath)
			if gotID != tc.wantID || gotOK != tc.wantOK {
				t.Errorf("ContainerID(%q) = (%q, %v), want (%q, %v)", tc.cgroupPath, gotID, gotOK, tc.wantID, tc.wantOK)
			}
		})
	}
}
