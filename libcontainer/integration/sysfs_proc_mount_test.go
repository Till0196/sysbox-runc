package integration

import (
	"strings"
	"testing"
)

// sysbox-runc: TestSysfsProcMountWithUserns verifies that sysfs and proc are
// successfully mounted inside a sys container that runs in its own user
// namespace.
//
// Background: in nested user-ns environments (e.g. containerd v2's CRI path
// where the shim itself runs in a user namespace), the container init process
// ends up in a level-2 user namespace, and the kernel rejects sysfs/proc
// mounts (EPERM from kobj_ns_current_may_mount / proc_fs_context). The fix
// delegates the sysfs/proc mount to the parent sysbox-runc, which retains
// initial user-ns root privileges.
//
// This test exercises the positive path: a userns container should still see
// sysfs and proc mounted in its mount table after the change. The Docker-based
// CI does not reproduce the nested user-ns scenario itself, so this is a
// non-regression guard rather than a direct repro of the K8s scenario.
func TestSysfsProcMountWithUserns(t *testing.T) {
	if testing.Short() {
		return
	}

	rootfs, err := newRootfs()
	ok(t, err)
	defer remove(rootfs)

	config := newTemplateConfig(&tParam{
		rootfs: rootfs,
		userns: true,
	})

	buffers, exitCode, err := runContainer(config, "", "cat", "/proc/self/mountinfo")
	if err != nil {
		t.Fatalf("%s: %s", buffers, err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code not 0. code %d stderr %q", exitCode, buffers.Stderr)
	}

	mountinfo := buffers.Stdout.String()

	// Each mountinfo line: "<id> <parent> <maj:min> <root> <mountpoint> ..."
	// followed by " - <fstype> <source> <opts>". We scan for sysfs at /sys
	// and proc at /proc.
	var sawSysfs, sawProc bool
	for _, line := range strings.Split(mountinfo, "\n") {
		fields := strings.Split(line, " ")
		if len(fields) < 5 {
			continue
		}
		mountpoint := fields[4]
		dash := -1
		for i, f := range fields {
			if f == "-" {
				dash = i
				break
			}
		}
		if dash < 0 || dash+1 >= len(fields) {
			continue
		}
		fstype := fields[dash+1]

		if mountpoint == "/sys" && fstype == "sysfs" {
			sawSysfs = true
		}
		if mountpoint == "/proc" && fstype == "proc" {
			sawProc = true
		}
	}

	if !sawSysfs {
		t.Fatalf("expected sysfs mounted at /sys inside userns container; mountinfo:\n%s", mountinfo)
	}
	if !sawProc {
		t.Fatalf("expected proc mounted at /proc inside userns container; mountinfo:\n%s", mountinfo)
	}
}

// TestSysfsProcMountReadOnly verifies that the read-only flag on the sysfs
// mount is honored when the mount is delegated to the parent sysbox-runc.
// The template config sets MS_RDONLY on /sys; if the parent-side helper drops
// flags during delegation, this test will catch it.
func TestSysfsProcMountReadOnly(t *testing.T) {
	if testing.Short() {
		return
	}

	rootfs, err := newRootfs()
	ok(t, err)
	defer remove(rootfs)

	config := newTemplateConfig(&tParam{
		rootfs: rootfs,
		userns: true,
	})

	buffers, exitCode, err := runContainer(config, "", "cat", "/proc/self/mountinfo")
	if err != nil {
		t.Fatalf("%s: %s", buffers, err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code not 0. code %d stderr %q", exitCode, buffers.Stderr)
	}

	for _, line := range strings.Split(buffers.Stdout.String(), "\n") {
		fields := strings.Split(line, " ")
		if len(fields) < 6 {
			continue
		}
		if fields[4] != "/sys" {
			continue
		}
		// Field 5 is the per-mount options (vfs flags). It should include "ro".
		opts := strings.Split(fields[5], ",")
		ro := false
		for _, o := range opts {
			if o == "ro" {
				ro = true
				break
			}
		}
		if !ro {
			t.Fatalf("expected /sys mount to be read-only, got opts=%q", fields[5])
		}
		return
	}
	t.Fatalf("did not find /sys in mountinfo:\n%s", buffers.Stdout.String())
}
