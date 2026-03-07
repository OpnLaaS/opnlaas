package pxe

import (
	"fmt"
	"sync"
)

// ProvisioningInstallCallback captures a post-install completion signal emitted by an installer.
type ProvisioningInstallCallback struct {
	BookingID    int
	ManagementIP string
	Token        string
	Stage        string
	RemoteAddr   string
	UserAgent    string
}

var (
	provisioningInstallCallbackMu sync.RWMutex
	provisioningInstallCallback   func(event ProvisioningInstallCallback) error
)

// SetProvisioningInstallCallback registers a callback for install completion signals.
func SetProvisioningInstallCallback(fn func(event ProvisioningInstallCallback) error) {
	provisioningInstallCallbackMu.Lock()
	defer provisioningInstallCallbackMu.Unlock()
	provisioningInstallCallback = fn
}

func emitProvisioningInstallCallback(event ProvisioningInstallCallback) (err error) {
	provisioningInstallCallbackMu.RLock()
	fn := provisioningInstallCallback
	provisioningInstallCallbackMu.RUnlock()
	if fn == nil {
		return fmt.Errorf("provisioning install callback handler is not configured")
	}

	return fn(event)
}
