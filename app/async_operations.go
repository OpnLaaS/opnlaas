package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/opnlaas/opnlaas/auth"
	"github.com/opnlaas/opnlaas/db"
)

const (
	asyncOperationKindHostPower = "host_power"

	asyncOperationStatusQueued  = "queued"
	asyncOperationStatusRunning = "running"
	asyncOperationStatusSuccess = "success"
	asyncOperationStatusWarning = "warning"
	asyncOperationStatusError   = "error"
)

var (
	asyncOperationRetention = 12 * time.Hour

	asyncOperationLock sync.RWMutex
	asyncOperations    = map[string]*apiAsyncOperation{}

	hostPowerOperationLock    sync.Mutex
	hostPowerOperationsByHost = map[string]string{}
)

type apiAsyncOperation struct {
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	Owner       string         `json:"-"`
	Status      string         `json:"status"`
	Message     string         `json:"message"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Result      map[string]any `json:"result,omitempty"`
}

func (o *apiAsyncOperation) clone() (out *apiAsyncOperation) {
	if o == nil {
		return nil
	}

	out = &apiAsyncOperation{
		ID:        o.ID,
		Kind:      o.Kind,
		Owner:     o.Owner,
		Status:    o.Status,
		Message:   o.Message,
		CreatedAt: o.CreatedAt,
		UpdatedAt: o.UpdatedAt,
	}

	if o.CompletedAt != nil {
		at := *o.CompletedAt
		out.CompletedAt = &at
	}

	if len(o.Result) > 0 {
		out.Result = make(map[string]any, len(o.Result))
		for key, value := range o.Result {
			out.Result[key] = value
		}
	}

	return out
}

func asyncOperationIsTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case asyncOperationStatusSuccess, asyncOperationStatusWarning, asyncOperationStatusError:
		return true
	default:
		return false
	}
}

func newAsyncOperationID() (id string) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("op_%d", time.Now().UnixNano())
	}
	return "op_" + hex.EncodeToString(buf)
}

func cleanupAsyncOperationsLocked(now time.Time) {
	cutoff := now.Add(-asyncOperationRetention)
	for id, op := range asyncOperations {
		if op == nil {
			delete(asyncOperations, id)
			continue
		}

		if op.CompletedAt != nil && op.CompletedAt.Before(cutoff) {
			delete(asyncOperations, id)
		}
	}
}

func asyncOperationCreate(owner string, kind string, message string, result map[string]any) (op *apiAsyncOperation) {
	now := time.Now()
	op = &apiAsyncOperation{
		ID:        newAsyncOperationID(),
		Kind:      strings.TrimSpace(kind),
		Owner:     strings.TrimSpace(owner),
		Status:    asyncOperationStatusQueued,
		Message:   strings.TrimSpace(message),
		CreatedAt: now,
		UpdatedAt: now,
		Result:    map[string]any{},
	}

	for key, value := range result {
		op.Result[key] = value
	}

	asyncOperationLock.Lock()
	cleanupAsyncOperationsLocked(now)
	asyncOperations[op.ID] = op
	asyncOperationLock.Unlock()

	return op.clone()
}

func asyncOperationDelete(operationID string) {
	id := strings.TrimSpace(operationID)
	if id == "" {
		return
	}

	asyncOperationLock.Lock()
	delete(asyncOperations, id)
	asyncOperationLock.Unlock()
}

func asyncOperationSetRunning(operationID string, message string, result map[string]any) {
	id := strings.TrimSpace(operationID)
	if id == "" {
		return
	}

	now := time.Now()
	asyncOperationLock.Lock()
	op := asyncOperations[id]
	if op != nil {
		op.Status = asyncOperationStatusRunning
		op.Message = strings.TrimSpace(message)
		op.UpdatedAt = now
		if len(result) > 0 {
			if op.Result == nil {
				op.Result = map[string]any{}
			}
			for key, value := range result {
				op.Result[key] = value
			}
		}
	}
	asyncOperationLock.Unlock()
}

func asyncOperationComplete(operationID string, status string, message string, result map[string]any) {
	id := strings.TrimSpace(operationID)
	if id == "" {
		return
	}

	normalizedStatus := strings.ToLower(strings.TrimSpace(status))
	if normalizedStatus == "" {
		normalizedStatus = asyncOperationStatusError
	}

	now := time.Now()
	asyncOperationLock.Lock()
	op := asyncOperations[id]
	if op != nil {
		op.Status = normalizedStatus
		op.Message = strings.TrimSpace(message)
		op.UpdatedAt = now
		if asyncOperationIsTerminal(normalizedStatus) {
			finishedAt := now
			op.CompletedAt = &finishedAt
		}

		if len(result) > 0 {
			if op.Result == nil {
				op.Result = map[string]any{}
			}
			for key, value := range result {
				op.Result[key] = value
			}
		}
	}
	asyncOperationLock.Unlock()
}

func asyncOperationByID(operationID string) (op *apiAsyncOperation, exists bool) {
	id := strings.TrimSpace(operationID)
	if id == "" {
		return nil, false
	}

	now := time.Now()
	asyncOperationLock.Lock()
	cleanupAsyncOperationsLocked(now)
	stored := asyncOperations[id]
	asyncOperationLock.Unlock()
	if stored == nil {
		return nil, false
	}

	return stored.clone(), true
}

func reserveHostPowerOperation(hostID string, operationID string) (existingOperationID string, alreadyReserved bool) {
	hostKey := strings.TrimSpace(hostID)
	operationKey := strings.TrimSpace(operationID)
	if hostKey == "" || operationKey == "" {
		return "", false
	}

	hostPowerOperationLock.Lock()
	defer hostPowerOperationLock.Unlock()
	if existing, exists := hostPowerOperationsByHost[hostKey]; exists && existing != "" {
		return existing, true
	}

	hostPowerOperationsByHost[hostKey] = operationKey
	return "", false
}

func releaseHostPowerOperation(hostID string, operationID string) {
	hostKey := strings.TrimSpace(hostID)
	operationKey := strings.TrimSpace(operationID)
	if hostKey == "" || operationKey == "" {
		return
	}

	hostPowerOperationLock.Lock()
	defer hostPowerOperationLock.Unlock()
	if current := hostPowerOperationsByHost[hostKey]; current == operationKey {
		delete(hostPowerOperationsByHost, hostKey)
	}
}

func runHostPowerOperation(operationID string, hostID string, action db.PowerAction) {
	defer releaseHostPowerOperation(hostID, operationID)

	result := map[string]any{
		"host_management_ip": strings.TrimSpace(hostID),
		"action":             action.String(),
		"action_value":       int(action),
	}

	asyncOperationSetRunning(operationID, fmt.Sprintf("Running %s for host %s", action.String(), hostID), result)

	host, err := db.Hosts.Select(hostID)
	if err != nil {
		appLog.Errorf("host power operation select failed host=%s err=%v", hostID, err)
		asyncOperationComplete(operationID, asyncOperationStatusError, "Failed to retrieve host", result)
		return
	}
	if host == nil {
		asyncOperationComplete(operationID, asyncOperationStatusError, "Host not found", result)
		return
	}

	managementOwned := false
	if host.Management == nil {
		if host.Management, err = db.NewHostManagementClient(host); err != nil {
			appLog.Errorf("host power operation management client failed host=%s err=%v", hostID, err)
			asyncOperationComplete(operationID, asyncOperationStatusError, "Failed to create management client", result)
			return
		}
		managementOwned = true
	}
	if managementOwned {
		defer func() {
			host.Management.Close()
			host.Management = nil
		}()
	}

	currentPowerState, err := host.Management.PowerState(false)
	if err != nil {
		appLog.Warningf("host power operation read state failed host=%s err=%v", hostID, err)
		asyncOperationComplete(operationID, asyncOperationStatusError, "Failed to read current power state", result)
		return
	}
	result["previous_power_state"] = int(currentPowerState)
	result["previous_power_state_label"] = currentPowerState.String()

	waitPowerState := db.PowerStateUnknown
	applyActionErr := ""

	switch action {
	case db.PowerActionPowerOn:
		waitPowerState = db.PowerStateOn
		if currentPowerState == db.PowerStateOn {
			result["power_state"] = int(currentPowerState)
			result["power_state_label"] = currentPowerState.String()
			asyncOperationComplete(operationID, asyncOperationStatusWarning, "Host already powered on", result)
			return
		}
		if err = host.Management.SetPowerState(db.PowerStateOn, false); err != nil {
			applyActionErr = "Failed to power on host"
		}
	case db.PowerActionGracefulShutdown:
		waitPowerState = db.PowerStateOff
		if currentPowerState == db.PowerStateOff {
			result["power_state"] = int(currentPowerState)
			result["power_state_label"] = currentPowerState.String()
			asyncOperationComplete(operationID, asyncOperationStatusWarning, "Host already powered off", result)
			return
		}
		if err = host.Management.SetPowerState(db.PowerStateOff, false); err != nil {
			applyActionErr = "Failed to gracefully shut down host"
		}
	case db.PowerActionPowerOff:
		waitPowerState = db.PowerStateOff
		if currentPowerState == db.PowerStateOff {
			result["power_state"] = int(currentPowerState)
			result["power_state_label"] = currentPowerState.String()
			asyncOperationComplete(operationID, asyncOperationStatusWarning, "Host already powered off", result)
			return
		}
		if err = host.Management.SetPowerState(db.PowerStateOff, true); err != nil {
			applyActionErr = "Failed to force power off host"
		}
	case db.PowerActionGracefulRestart:
		waitPowerState = db.PowerStateOn
		if err = host.Management.ResetPowerState(false); err != nil {
			applyActionErr = "Failed to gracefully restart host"
		}
	case db.PowerActionForceRestart:
		waitPowerState = db.PowerStateOn
		if err = host.Management.ResetPowerState(true); err != nil {
			applyActionErr = "Failed to force restart host"
		}
	default:
		asyncOperationComplete(operationID, asyncOperationStatusError, "Unsupported power action", result)
		return
	}

	if applyActionErr != "" {
		appLog.Warningf("host power operation action failed host=%s action=%s err=%v", hostID, action.String(), err)
		asyncOperationComplete(operationID, asyncOperationStatusError, applyActionErr, result)
		return
	}

	if err = host.Management.WaitSystemPowerState(waitPowerState, 120); err != nil {
		result["wait_target_state"] = int(waitPowerState)
		result["wait_target_state_label"] = waitPowerState.String()
		appLog.Warningf("host power operation wait timeout host=%s target=%s err=%v", hostID, waitPowerState.String(), err)
		asyncOperationComplete(operationID, asyncOperationStatusWarning, fmt.Sprintf("Timed out waiting for host to reach %s power state", waitPowerState.String()), result)
		return
	}

	if host.LastKnownPowerState, err = host.Management.PowerState(true); err != nil {
		appLog.Warningf("host power operation read final state failed host=%s err=%v", hostID, err)
		host.LastKnownPowerState = waitPowerState
	} else {
		host.LastKnownPowerStateTime = time.Now()
		if updateErr := db.Hosts.Update(host); updateErr != nil {
			appLog.Warningf("host power operation db update failed host=%s err=%v", hostID, updateErr)
		}
	}

	result["power_state"] = int(host.LastKnownPowerState)
	result["power_state_label"] = host.LastKnownPowerState.String()
	asyncOperationComplete(operationID, asyncOperationStatusSuccess, "Power action completed successfully", result)
}

func apiAsyncOperationByID(c *fiber.Ctx) (err error) {
	user := auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	operationID := strings.TrimSpace(c.Params("operation_id"))
	if operationID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "operation id is required"})
	}

	op, exists := asyncOperationByID(operationID)
	if !exists || op == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "operation not found"})
	}

	if op.Owner != user.Username && user.Permissions() < auth.AuthPermsAdministrator {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "not allowed to view this operation"})
	}

	return c.JSON(op)
}
