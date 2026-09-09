package cdp

import (
	"fmt"
	"time"

	"surf-backend/internal/process"
)

// CloseBrowser never force-kills on a timeout. The caller must obtain an
// explicit, generation-bound force confirmation before passing force=true.
func CloseBrowser(client *Client, child *process.Started, force bool, timeout time.Duration) error {
	if client == nil || child == nil {
		return nil
	}
	expires := time.Now().Add(timeout)
	if force {
		if err := child.Kill(); err != nil {
			return err
		}
	} else {
		// Dispatch only: Browser.close may tear down CDP before acknowledging.
		_ = client.Dispatch("", "Browser.close", nil)
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	select {
	case <-client.Closed():
	case <-deadline.C:
		return fmt.Errorf("browser did not close; try again or explicitly force close")
	}
	select {
	case <-child.Done:
	case <-deadline.C:
		return fmt.Errorf("browser process is still closing; try again or explicitly force close")
	}
	if err := child.WaitOwned(time.Until(expires)); err != nil {
		return err
	}
	child.ReleaseOwned()
	return nil
}
