package util

import (
	"fmt"
	"os/user"
	"strconv"
	"syscall"
)

// dropPrivileges drops root privileges and runs the program as the specified user and group.
func dropPrivileges(uid, gid int) error {
	// Set the group ID
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("failed to drop group privileges: %w", err)
	}
	// Set the user ID
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("failed to drop user privileges: %w", err)
	}
	return nil
}

func DropToUser(username string) error {
	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("user.Lookup: %w", err)
	}

	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return fmt.Errorf("strconv.Atoi(Uid): %w", err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return fmt.Errorf("strconv.Atoi(Gid): %w", err)
	}

	if err := dropPrivileges(uid, gid); err != nil {
		return fmt.Errorf("dropPrivileges: %w", err)
	}

	return nil
}
