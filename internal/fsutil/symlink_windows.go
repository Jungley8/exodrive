//go:build windows

package fsutil

import (
	"fmt"
	"os/exec"
	"strings"
)

// makeLink creates a directory junction. A junction needs no administrator
// rights, where a symbolic link needs Developer Mode or elevation. Junctions
// live on NTFS only, which is the one filesystem the allowlist accepts there.
func makeLink(target, linkPath string) error {
	out, err := exec.Command("cmd", "/c", "mklink", "/J", linkPath, target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("create junction %s -> %s: %v: %s", linkPath, target, err, strings.TrimSpace(string(out)))
	}
	return nil
}
