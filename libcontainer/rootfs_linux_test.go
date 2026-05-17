// +build linux

package libcontainer

import (
	"testing"

	"github.com/opencontainers/runc/libcontainer/configs"
)

func TestNeedsSetupDev(t *testing.T) {
	config := &configs.Config{
		Mounts: []*configs.Mount{
			{
				Device:      "bind",
				Source:      "/dev",
				Destination: "/dev",
			},
		},
	}
	if needsSetupDev(config) {
		t.Fatal("expected needsSetupDev to be false, got true")
	}
}

func TestNeedsSetupDevStrangeSource(t *testing.T) {
	config := &configs.Config{
		Mounts: []*configs.Mount{
			{
				Device:      "bind",
				Source:      "/devx",
				Destination: "/dev",
			},
		},
	}
	if needsSetupDev(config) {
		t.Fatal("expected needsSetupDev to be false, got true")
	}
}

func TestNeedsSetupDevStrangeDest(t *testing.T) {
	config := &configs.Config{
		Mounts: []*configs.Mount{
			{
				Device:      "bind",
				Source:      "/dev",
				Destination: "/devx",
			},
		},
	}
	if !needsSetupDev(config) {
		t.Fatal("expected needsSetupDev to be true, got false")
	}
}

func TestNeedsSetupDevStrangeSourceDest(t *testing.T) {
	config := &configs.Config{
		Mounts: []*configs.Mount{
			{
				Device:      "bind",
				Source:      "/devx",
				Destination: "/devx",
			},
		},
	}
	if !needsSetupDev(config) {
		t.Fatal("expected needsSetupDev to be true, got false")
	}
}

// sysbox-runc: isSysboxfsOvermount is used to classify mounts whose
// destination is under a sysbox-fs managed path (e.g. /proc/sys/fs/binfmt_misc).
// doMounts uses the result to partition mounts into two phases: phase 1
// (pre-pivot) handles non-overmount entries, phase 2 (post-pivot) handles
// the overmounts on top of sysbox-fs paths.
func TestIsSysboxfsOvermount(t *testing.T) {
	tests := []struct {
		name string
		dest string
		want bool
	}{
		{"binfmt_misc under /proc/sys", "/proc/sys/fs/binfmt_misc", true},
		{"nested path under /proc/sys", "/proc/sys/kernel/foo", true},
		{"exact /proc/sys is not an overmount", "/proc/sys", false},
		{"sibling of sysboxfs path", "/proc/sysrq-trigger", false},
		{"unrelated path", "/etc/hosts", false},
		{"root", "/", false},
		{"empty destination", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isSysboxfsOvermount(&configs.Mount{Destination: tc.dest})
			if got != tc.want {
				t.Fatalf("isSysboxfsOvermount(%q) = %v, want %v", tc.dest, got, tc.want)
			}
		})
	}
}
