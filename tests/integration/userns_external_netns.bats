#!/usr/bin/env bats

# This test reproduces the K8s + containerd v2 scenario in which the
# container's network namespace is owned by an outer user namespace
# (typically the kubelet's initial user-ns, set up via CNI before the
# OCI runtime is invoked), while sysbox-runc creates the container in
# its own user namespace. The kernel rejects in-container sysfs mounts
# under this asymmetry; the patches in this PR work around it by
# delegating sysfs/proc mounts to the parent sysbox-runc, which stays
# in the initial user-ns and has CAP_SYS_ADMIN over the net-ns it owns.
#
# Without the patches sysbox-runc fails the container start with:
#
#   rootfs_linux.go: mounting "sysfs" to rootfs ...
#     caused: mount through procfd: operation not permitted
#
# With the patches the container starts and `sysfs on /sys` is visible
# in /proc/<pid>/mountinfo.
#
# Reproduction without K8s: create a persistent net-ns owned by the
# initial user-ns and tell sysbox-runc to share it via the OCI
# namespaces config (.linux.namespaces[].path).

load helpers

EXT_NETNS_PATH=""

function setup() {
	teardown_busybox
	setup_busybox

	# Persistent net-ns owned by the initial user-ns. Use unshare so we
	# don't depend on iproute2.
	mkdir -p /run/netns
	EXT_NETNS_PATH="/run/netns/sysbox-bats-$$"
	touch "$EXT_NETNS_PATH"
	unshare --net="$EXT_NETNS_PATH" true
}

function teardown() {
	if [ -n "$EXT_NETNS_PATH" ] && [ -e "$EXT_NETNS_PATH" ]; then
		umount "$EXT_NETNS_PATH" 2>/dev/null || true
		rm -f "$EXT_NETNS_PATH"
	fi
	teardown_busybox
}

@test "sysfs mounts when sharing an externally-created net-ns" {
	# Replace the default network namespace entry (no path = create
	# new) with a "join existing" entry pointing at the externally
	# created net-ns.
	update_config '.linux.namespaces |= (
		map(select(.type != "network")) +
		[{"type":"network","path":"'"$EXT_NETNS_PATH"'"}]
	)' "$BUSYBOX_BUNDLE"

	# A long-lived process so we can exec into the container after
	# start without racing teardown.
	update_config '.process.args = ["sleep","60"]' "$BUSYBOX_BUNDLE"

	# Without the patches in this PR `runc run` fails here with:
	#   rootfs_linux.go: mounting "sysfs" to rootfs ...
	#     caused: mount through procfd: operation not permitted
	runc run -d --console-socket "$CONSOLE_SOCKET" test_external_netns
	[ "$status" -eq 0 ]

	testcontainer test_external_netns running

	# Verify sysfs is actually mounted inside the container.
	runc exec test_external_netns cat /proc/self/mountinfo
	[ "$status" -eq 0 ]
	# mountinfo line: "<id> <parent> <maj:min> <root> /sys <opts> - sysfs ..."
	echo "$output" | grep -E ' /sys [^-]+ - sysfs '

	runc delete --force test_external_netns
}
