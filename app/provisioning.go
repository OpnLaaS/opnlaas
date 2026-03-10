package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/db"
	"github.com/opnlaas/opnlaas/host/pxe"
)

const (
	provisioningStatusQueued          = "queued"
	provisioningStatusRunning         = "running"
	provisioningStatusAwaitingInstall = "awaiting_install"
	provisioningStatusCompleted       = "completed"
	provisioningStatusFailed          = "failed"
	provisioningStatusPartialFailed   = "partial_failed"
	provisioningStatusCanceled        = "canceled"
	provisioningStatusDestroyed       = "destroyed"
)

var (
	errProvisioningCanceled      = errors.New("provisioning canceled")
	errProvisioningNotFound      = errors.New("provisioning state not found")
	errProvisioningNotCancelable = errors.New("provisioning is not active")
)

type bookingDeployBasicConfig struct {
	Timezone        string   `json:"timezone"`
	Locale          string   `json:"locale"`
	KeyboardLayout  string   `json:"keyboard_layout"`
	Mirror          string   `json:"mirror"`
	Packages        []string `json:"packages"`
	LateStageScript string   `json:"late_stage_script"`
}

func (cfg bookingDeployBasicConfig) toTemplateData() (values map[string]string) {
	values = map[string]string{}

	if v := strings.TrimSpace(cfg.Timezone); v != "" {
		values["template.timezone"] = v
	}

	if v := strings.TrimSpace(cfg.Locale); v != "" {
		values["template.locale"] = v
	}

	if v := strings.TrimSpace(cfg.KeyboardLayout); v != "" {
		values["template.keyboard_layout"] = v
	}

	if v := strings.TrimSpace(cfg.Mirror); v != "" {
		values["template.mirror"] = v
	}

	if len(cfg.Packages) > 0 {
		values["template.packages"] = strings.Join(cfg.Packages, ",")
	}

	if v := strings.TrimSpace(cfg.LateStageScript); v != "" {
		values["template.late_script"] = v
	}

	if len(values) == 0 {
		values = nil
	}

	return
}

type apiProvisioningCredentials struct {
	GivenUserUsername   string `json:"given_user_username"`
	GivenUserPassword   string `json:"given_user_password"`
	ManagedUserUsername string `json:"managed_user_username"`
	ManagedUserPassword string `json:"managed_user_password"`
}

type apiProvisioningEvent struct {
	At      time.Time `json:"at"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

type apiProvisioningHostState struct {
	ManagementIP    string    `json:"management_ip"`
	Model           string    `json:"model"`
	Hostname        string    `json:"hostname"`
	ISOSelection    string    `json:"iso_selection"`
	Status          string    `json:"status"`
	Message         string    `json:"message"`
	UpdatedAt       time.Time `json:"updated_at"`
	CompletionToken string    `json:"-"`
}

type apiProvisioningStatus struct {
	BookingID   int                         `json:"booking_id"`
	Owner       string                      `json:"owner"`
	Status      string                      `json:"status"`
	StartedAt   time.Time                   `json:"started_at"`
	UpdatedAt   time.Time                   `json:"updated_at"`
	FinishedAt  *time.Time                  `json:"finished_at,omitempty"`
	Hosts       []*apiProvisioningHostState `json:"hosts"`
	Events      []apiProvisioningEvent      `json:"events"`
	Credentials apiProvisioningCredentials  `json:"credentials"`
}

func (s *apiProvisioningStatus) clone() (out *apiProvisioningStatus) {
	if s == nil {
		return nil
	}

	cloned := *s
	if len(s.Hosts) > 0 {
		cloned.Hosts = make([]*apiProvisioningHostState, 0, len(s.Hosts))
		for _, host := range s.Hosts {
			if host == nil {
				continue
			}

			copyHost := *host
			cloned.Hosts = append(cloned.Hosts, &copyHost)
		}
	}

	cloned.Events = append([]apiProvisioningEvent(nil), s.Events...)
	out = &cloned
	return
}

func dbProvisioningStatusFromAPI(s *apiProvisioningStatus) *db.BookingProvisioningStatus {
	if s == nil {
		return nil
	}

	record := &db.BookingProvisioningStatus{
		BookingID: s.BookingID,
		Owner:     s.Owner,
		Status:    s.Status,
		StartedAt: s.StartedAt,
		UpdatedAt: s.UpdatedAt,
		Credentials: db.BookingProvisioningCredentials{
			GivenUserUsername:   s.Credentials.GivenUserUsername,
			GivenUserPassword:   s.Credentials.GivenUserPassword,
			ManagedUserUsername: s.Credentials.ManagedUserUsername,
			ManagedUserPassword: s.Credentials.ManagedUserPassword,
		},
	}

	if s.FinishedAt != nil {
		record.HasFinished = true
		record.FinishedAt = *s.FinishedAt
	}

	for _, host := range s.Hosts {
		if host == nil {
			continue
		}

		record.Hosts = append(record.Hosts, db.BookingProvisioningHostState{
			ManagementIP:    host.ManagementIP,
			Model:           host.Model,
			Hostname:        host.Hostname,
			ISOSelection:    host.ISOSelection,
			Status:          host.Status,
			Message:         host.Message,
			UpdatedAt:       host.UpdatedAt,
			CompletionToken: host.CompletionToken,
		})
	}

	for _, event := range s.Events {
		record.Events = append(record.Events, db.BookingProvisioningEvent{
			At:      event.At,
			Level:   event.Level,
			Message: event.Message,
		})
	}

	return record
}

func apiProvisioningStatusFromDB(record *db.BookingProvisioningStatus) *apiProvisioningStatus {
	if record == nil {
		return nil
	}

	status := &apiProvisioningStatus{
		BookingID: record.BookingID,
		Owner:     record.Owner,
		Status:    record.Status,
		StartedAt: record.StartedAt,
		UpdatedAt: record.UpdatedAt,
		Credentials: apiProvisioningCredentials{
			GivenUserUsername:   record.Credentials.GivenUserUsername,
			GivenUserPassword:   record.Credentials.GivenUserPassword,
			ManagedUserUsername: record.Credentials.ManagedUserUsername,
			ManagedUserPassword: record.Credentials.ManagedUserPassword,
		},
	}

	if record.HasFinished {
		finished := record.FinishedAt
		status.FinishedAt = &finished
	}

	for _, host := range record.Hosts {
		copyHost := host
		status.Hosts = append(status.Hosts, &apiProvisioningHostState{
			ManagementIP:    copyHost.ManagementIP,
			Model:           copyHost.Model,
			Hostname:        copyHost.Hostname,
			ISOSelection:    copyHost.ISOSelection,
			Status:          copyHost.Status,
			Message:         copyHost.Message,
			UpdatedAt:       copyHost.UpdatedAt,
			CompletionToken: copyHost.CompletionToken,
		})
	}

	for _, event := range record.Events {
		status.Events = append(status.Events, apiProvisioningEvent{
			At:      event.At,
			Level:   event.Level,
			Message: event.Message,
		})
	}

	return status
}

func persistProvisioningJobLocked(job *apiProvisioningStatus) {
	if job == nil {
		return
	}

	if err := db.UpsertBookingProvisioningStatus(dbProvisioningStatusFromAPI(job)); err != nil {
		appLog.Warningf("[PROV] booking=%d failed to persist provisioning state: %v\n", job.BookingID, err)
	}
}

func hydrateProvisioningJobLocked(bookingID int) (*apiProvisioningStatus, bool) {
	job, ok := provisioningJobs[bookingID]
	if ok && job != nil {
		return job, true
	}

	record, err := db.BookingProvisioningStatusByBookingID(bookingID)
	if err != nil {
		appLog.Warningf("[PROV] booking=%d failed to load provisioning state from db: %v\n", bookingID, err)
		return nil, false
	}
	if record == nil {
		return nil, false
	}

	job = apiProvisioningStatusFromDB(record)
	if job == nil {
		return nil, false
	}

	provisioningJobs[bookingID] = job
	return job, true
}

var (
	provisioningLock        sync.RWMutex
	provisioningJobs        = map[int]*apiProvisioningStatus{}
	provisioningCancelFuncs = map[int]context.CancelFunc{}
)

func init() {
	pxe.SetProvisioningInstallCallback(handleProvisioningInstallCallback)
}

func provisioningCredentialsFromConfig() (creds apiProvisioningCredentials) {
	creds = apiProvisioningCredentials{
		GivenUserUsername:   config.Config.Preconfigure.GivenUser.Username,
		GivenUserPassword:   config.Config.Preconfigure.GivenUser.Password,
		ManagedUserUsername: config.Config.Preconfigure.ManagedUser.Username,
		ManagedUserPassword: config.Config.Preconfigure.ManagedUser.Password,
	}
	return
}

func provisioningStatusIsTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case provisioningStatusCompleted, provisioningStatusFailed, provisioningStatusPartialFailed, provisioningStatusCanceled, provisioningStatusDestroyed:
		return true
	default:
		return false
	}
}

func provisioningStatusIsCancelable(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case provisioningStatusQueued, provisioningStatusRunning, provisioningStatusAwaitingInstall:
		return true
	default:
		return false
	}
}

func provisioningHostStatusIsTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "failed", "canceled":
		return true
	default:
		return false
	}
}

func setProvisioningCancelFuncLocked(bookingID int, cancel context.CancelFunc) {
	if cancel == nil {
		delete(provisioningCancelFuncs, bookingID)
		return
	}

	provisioningCancelFuncs[bookingID] = cancel
}

func consumeProvisioningCancelFuncLocked(bookingID int) (cancel context.CancelFunc) {
	cancel = provisioningCancelFuncs[bookingID]
	delete(provisioningCancelFuncs, bookingID)
	return
}

func createProvisioningJob(bookingID int, owner string, hosts []db.BookingRequestHost) (snapshot *apiProvisioningStatus) {
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].ManagementIP < hosts[j].ManagementIP })

	now := time.Now()
	job := &apiProvisioningStatus{
		BookingID:   bookingID,
		Owner:       owner,
		Status:      provisioningStatusQueued,
		StartedAt:   now,
		UpdatedAt:   now,
		Credentials: provisioningCredentialsFromConfig(),
	}

	for _, host := range hosts {
		model := ""
		if dbHost, hostErr := db.Hosts.Select(host.ManagementIP); hostErr != nil {
			appLog.Warningf("[PROV] booking=%d host=%s failed to load host model: %v\n", bookingID, host.ManagementIP, hostErr)
		} else if dbHost != nil {
			model = strings.TrimSpace(dbHost.Model)
		}

		hostname := strings.TrimSpace(host.Hostname)
		reservedMessage := "Host reserved for booking"
		if model != "" && hostname != "" {
			reservedMessage = fmt.Sprintf("%s -> %s reserved for booking", model, hostname)
		} else if hostname != "" {
			reservedMessage = fmt.Sprintf("Host %s reserved for booking", hostname)
		} else if model != "" {
			reservedMessage = fmt.Sprintf("%s reserved for booking", model)
		}

		job.Hosts = append(job.Hosts, &apiProvisioningHostState{
			ManagementIP:    host.ManagementIP,
			Model:           model,
			Hostname:        hostname,
			ISOSelection:    host.ISOSelection,
			Status:          "reserved",
			Message:         reservedMessage,
			UpdatedAt:       now,
			CompletionToken: newProvisioningCompletionToken(),
		})
	}

	job.Events = append(job.Events, apiProvisioningEvent{
		At:      now,
		Level:   "info",
		Message: fmt.Sprintf("Booking %d queued for provisioning", bookingID),
	})

	provisioningLock.Lock()
	provisioningJobs[bookingID] = job
	persistProvisioningJobLocked(job)
	provisioningLock.Unlock()

	return job.clone()
}

func provisioningSnapshot(bookingID int) (snapshot *apiProvisioningStatus, exists bool) {
	provisioningLock.Lock()
	job, ok := hydrateProvisioningJobLocked(bookingID)
	provisioningLock.Unlock()
	if !ok || job == nil {
		return nil, false
	}

	return job.clone(), true
}

func provisioningSetStatus(bookingID int, status string) {
	provisioningLock.Lock()
	defer provisioningLock.Unlock()

	job, ok := hydrateProvisioningJobLocked(bookingID)
	if !ok || job == nil {
		return
	}

	current := strings.ToLower(strings.TrimSpace(job.Status))
	target := strings.ToLower(strings.TrimSpace(status))
	if current == provisioningStatusDestroyed && target != provisioningStatusDestroyed {
		return
	}
	if provisioningStatusIsTerminal(current) && target != current && target != provisioningStatusDestroyed {
		return
	}

	now := time.Now()
	job.Status = status
	job.UpdatedAt = now
	if provisioningStatusIsTerminal(status) {
		job.FinishedAt = &now
	}
	persistProvisioningJobLocked(job)
}

func provisioningAppendEvent(bookingID int, level string, message string) {
	provisioningLock.Lock()
	defer provisioningLock.Unlock()

	job, ok := hydrateProvisioningJobLocked(bookingID)
	if !ok || job == nil {
		return
	}

	now := time.Now()
	job.Events = append(job.Events, apiProvisioningEvent{
		At:      now,
		Level:   level,
		Message: message,
	})
	job.UpdatedAt = now

	switch strings.ToLower(strings.TrimSpace(level)) {
	case "error":
		appLog.Errorf("[PROV] booking=%d %s\n", bookingID, message)
	case "warn", "warning":
		appLog.Warningf("[PROV] booking=%d %s\n", bookingID, message)
	default:
		appLog.Basicf("[PROV] booking=%d %s\n", bookingID, message)
	}

	persistProvisioningJobLocked(job)
}

func provisioningUpdateHost(bookingID int, managementIP string, status string, message string) {
	provisioningLock.Lock()
	defer provisioningLock.Unlock()

	job, ok := hydrateProvisioningJobLocked(bookingID)
	if !ok || job == nil {
		return
	}
	if job.Status == provisioningStatusCanceled || job.Status == provisioningStatusDestroyed {
		return
	}

	now := time.Now()
	for _, host := range job.Hosts {
		if host == nil || host.ManagementIP != managementIP {
			continue
		}

		host.Status = status
		host.Message = message
		host.UpdatedAt = now
		job.UpdatedAt = now
		persistProvisioningJobLocked(job)
		return
	}
}

func cancelProvisioning(bookingID int, requestedBy string) (snapshot *apiProvisioningStatus, err error) {
	requestedBy = strings.TrimSpace(requestedBy)
	if requestedBy == "" {
		requestedBy = "unknown"
	}

	provisioningLock.Lock()
	defer provisioningLock.Unlock()

	job, ok := hydrateProvisioningJobLocked(bookingID)
	if !ok || job == nil {
		return nil, errProvisioningNotFound
	}

	if !provisioningStatusIsCancelable(job.Status) {
		return nil, fmt.Errorf("%w: %s", errProvisioningNotCancelable, job.Status)
	}

	now := time.Now()
	job.Status = provisioningStatusCanceled
	job.UpdatedAt = now
	job.FinishedAt = &now
	for _, host := range job.Hosts {
		if host == nil || provisioningHostStatusIsTerminal(host.Status) {
			continue
		}

		host.Status = "canceled"
		host.Message = fmt.Sprintf("Provisioning canceled by %s", requestedBy)
		host.UpdatedAt = now
	}

	job.Events = append(job.Events, apiProvisioningEvent{
		At:      now,
		Level:   "warn",
		Message: fmt.Sprintf("Provisioning canceled by %s", requestedBy),
	})

	if cancel := consumeProvisioningCancelFuncLocked(bookingID); cancel != nil {
		cancel()
	}

	persistProvisioningJobLocked(job)
	return job.clone(), nil
}

func provisioningDeleteJob(bookingID int) {
	provisioningLock.Lock()
	defer provisioningLock.Unlock()
	if cancel := consumeProvisioningCancelFuncLocked(bookingID); cancel != nil {
		cancel()
	}
	delete(provisioningJobs, bookingID)
}

func newProvisioningCompletionToken() (token string) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}

	return hex.EncodeToString(buf[:])
}

func provisioningHostCompletionToken(bookingID int, managementIP string) (token string) {
	provisioningLock.Lock()
	defer provisioningLock.Unlock()

	job, ok := hydrateProvisioningJobLocked(bookingID)
	if !ok || job == nil {
		return ""
	}

	for _, host := range job.Hosts {
		if host == nil || host.ManagementIP != managementIP {
			continue
		}

		return host.CompletionToken
	}

	return ""
}

func provisioningInjectCompletionTemplateData(templateData map[string]string, bookingID int, managementIP string) map[string]string {
	token := provisioningHostCompletionToken(bookingID, managementIP)
	if token == "" {
		return templateData
	}

	if templateData == nil {
		templateData = map[string]string{}
	}

	templateData["template.provisioning.booking_id"] = strconv.Itoa(bookingID)
	templateData["template.provisioning.management_ip"] = managementIP
	templateData["template.provisioning.callback_token"] = token
	return templateData
}

func isTerminalProvisioningCallbackStage(stage string) bool {
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case "", "install_complete", "kickstart_post", "cloudinit_runcmd":
		return true
	default:
		return false
	}
}

func isFailureProvisioningCallbackStage(stage string) bool {
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case "autoinstall_error", "kickstart_error", "install_failed", "autoinstall_disk_wipe_failed", "kickstart_disk_wipe_failed", "autoinstall_disk_mismatch_failed", "kickstart_disk_mismatch_failed":
		return true
	default:
		return false
	}
}

func isCompletionPreparationProvisioningCallbackStage(stage string) bool {
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case "autoinstall_late":
		return true
	default:
		return false
	}
}

func shouldIncludeProvisioningCallbackDetailInEvent(stage string, stageFailure bool) bool {
	if stageFailure {
		return true
	}

	switch strings.ToLower(strings.TrimSpace(stage)) {
	case "autoinstall_disk_audit", "kickstart_disk_audit":
		return true
	default:
		return false
	}
}

func handleProvisioningInstallCallback(event pxe.ProvisioningInstallCallback) (err error) {
	if event.BookingID <= 0 {
		return fmt.Errorf("invalid booking id")
	}
	if strings.TrimSpace(event.ManagementIP) == "" {
		return fmt.Errorf("management_ip is required")
	}
	if strings.TrimSpace(event.Token) == "" {
		return fmt.Errorf("token is required")
	}

	provisioningLock.Lock()
	defer provisioningLock.Unlock()

	job, ok := hydrateProvisioningJobLocked(event.BookingID)
	if !ok || job == nil {
		return fmt.Errorf("booking provisioning state not found")
	}
	if job.Status == provisioningStatusCanceled || job.Status == provisioningStatusDestroyed {
		return nil
	}

	var hostState *apiProvisioningHostState
	for _, host := range job.Hosts {
		if host == nil || host.ManagementIP != event.ManagementIP {
			continue
		}

		hostState = host
		break
	}

	if hostState == nil {
		return fmt.Errorf("host not found in booking provisioning state")
	}

	if subtle.ConstantTimeCompare([]byte(hostState.CompletionToken), []byte(event.Token)) != 1 {
		return fmt.Errorf("invalid provisioning callback token")
	}

	now := time.Now()
	stage := strings.TrimSpace(event.Stage)
	if stage == "" {
		stage = "install_complete"
	}
	stageTerminal := isTerminalProvisioningCallbackStage(stage)
	stageFailure := isFailureProvisioningCallbackStage(stage)
	stageCompletionPreparation := isCompletionPreparationProvisioningCallbackStage(stage)
	stageDetail := strings.TrimSpace(event.Detail)
	if len(stageDetail) > 512 {
		stageDetail = stageDetail[:512]
	}

	if stageFailure {
		hostState.Status = "failed"
		if stageDetail != "" {
			hostState.Message = fmt.Sprintf("Installer failure callback received (%s): %s", stage, stageDetail)
		} else {
			hostState.Message = fmt.Sprintf("Installer failure callback received (%s)", stage)
		}
	} else if stageTerminal {
		if hostState.Status != "failed" {
			hostState.Status = "completed"
			hostState.Message = fmt.Sprintf("Installer completion callback received (%s)", stage)
		}
	} else if hostState.Status != "completed" && hostState.Status != "failed" {
		hostState.Status = "installing"
		hostState.Message = fmt.Sprintf("Installer progress callback received (%s)", stage)
	}

	hostState.UpdatedAt = now
	job.UpdatedAt = now
	eventLevel := "info"
	if stageFailure {
		eventLevel = "error"
	}

	callbackMessage := fmt.Sprintf(
		"Host %s reported install callback (stage=%s remote=%s terminal=%t)",
		event.ManagementIP,
		stage,
		strings.TrimSpace(event.RemoteAddr),
		stageTerminal,
	)
	if shouldIncludeProvisioningCallbackDetailInEvent(stage, stageFailure) {
		if stageFailure {
			callbackMessage = fmt.Sprintf("%s failure=%t", callbackMessage, stageFailure)
		}
		if stageDetail != "" {
			callbackMessage = fmt.Sprintf("%s detail=%q", callbackMessage, stageDetail)
		}
	}

	job.Events = append(job.Events, apiProvisioningEvent{
		At:      now,
		Level:   eventLevel,
		Message: callbackMessage,
	})

	if !stageTerminal && !stageFailure && !stageCompletionPreparation {
		persistProvisioningJobLocked(job)
		if stageDetail != "" {
			appLog.Basicf(
				"[PROV] booking=%d Host %s reported installer progress callback (stage=%s remote=%s detail=%q)\n",
				event.BookingID,
				event.ManagementIP,
				stage,
				strings.TrimSpace(event.RemoteAddr),
				stageDetail,
			)
		} else {
			appLog.Basicf(
				"[PROV] booking=%d Host %s reported installer progress callback (stage=%s remote=%s)\n",
				event.BookingID,
				event.ManagementIP,
				stage,
				strings.TrimSpace(event.RemoteAddr),
			)
		}
		return nil
	}

	if hostRecord, hostErr := db.Hosts.Select(event.ManagementIP); hostErr != nil {
		job.Events = append(job.Events, apiProvisioningEvent{
			At:      now,
			Level:   "warn",
			Message: fmt.Sprintf("Host %s completion: failed to load host for localboot guard: %v", event.ManagementIP, hostErr),
		})
	} else if hostRecord == nil {
		job.Events = append(job.Events, apiProvisioningEvent{
			At:      now,
			Level:   "warn",
			Message: fmt.Sprintf("Host %s completion: host record not found for localboot guard", event.ManagementIP),
		})
	} else if guardErr := pxe.ApplyHostProfileOverride(hostRecord, pxe.ProfileOverride{
		TemplateData: map[string]string{
			"template.pxe.localboot": "true",
		},
	}); guardErr != nil {
		job.Events = append(job.Events, apiProvisioningEvent{
			At:      now,
			Level:   "warn",
			Message: fmt.Sprintf("Host %s completion: failed to apply localboot guard: %v", event.ManagementIP, guardErr),
		})
		appLog.Warningf("[PROV] booking=%d Host %s completion: localboot guard apply failed: %v\n", event.BookingID, event.ManagementIP, guardErr)
	} else {
		job.Events = append(job.Events, apiProvisioningEvent{
			At:      now,
			Level:   "info",
			Message: fmt.Sprintf("Host %s completion: applied localboot PXE guard to prevent installer loops", event.ManagementIP),
		})
		appLog.Basicf("[PROV] booking=%d Host %s completion: localboot PXE guard applied\n", event.BookingID, event.ManagementIP)

		if mgmt, mgmtErr := db.NewHostManagementClient(hostRecord); mgmtErr != nil {
			job.Events = append(job.Events, apiProvisioningEvent{
				At:      now,
				Level:   "warn",
				Message: fmt.Sprintf("Host %s completion: failed to create management client for boot override clear: %v", event.ManagementIP, mgmtErr),
			})
			appLog.Warningf("[PROV] booking=%d Host %s completion: management client creation for boot override clear failed: %v\n", event.BookingID, event.ManagementIP, mgmtErr)
		} else {
			defer mgmt.Close()
			if clearErr := mgmt.ClearBootOverride(); clearErr != nil {
				job.Events = append(job.Events, apiProvisioningEvent{
					At:      now,
					Level:   "warn",
					Message: fmt.Sprintf("Host %s completion: failed to clear firmware boot override: %v", event.ManagementIP, clearErr),
				})
				appLog.Warningf("[PROV] booking=%d Host %s completion: firmware boot override clear failed: %v\n", event.BookingID, event.ManagementIP, clearErr)
			} else {
				job.Events = append(job.Events, apiProvisioningEvent{
					At:      now,
					Level:   "info",
					Message: fmt.Sprintf("Host %s completion: cleared firmware boot override (NoOverride)", event.ManagementIP),
				})
				appLog.Basicf("[PROV] booking=%d Host %s completion: firmware boot override cleared (NoOverride)\n", event.BookingID, event.ManagementIP)
			}
		}
	}

	anyInProgress := false
	anyFailed := false
	for _, host := range job.Hosts {
		if host == nil {
			continue
		}

		switch host.Status {
		case "failed":
			anyFailed = true
		case "completed":
		default:
			anyInProgress = true
		}
	}

	if !anyInProgress {
		if anyFailed {
			job.Status = provisioningStatusPartialFailed
			job.Events = append(job.Events, apiProvisioningEvent{
				At:      now,
				Level:   "warn",
				Message: "Provisioning finished with install callback(s), but some hosts failed",
			})
		} else {
			job.Status = provisioningStatusCompleted
			job.Events = append(job.Events, apiProvisioningEvent{
				At:      now,
				Level:   "info",
				Message: "Install completion callback received for all hosts",
			})
		}

		job.FinishedAt = &now
	}
	if stageFailure {
		appLog.Warningf("[PROV] booking=%d Host %s reported install failure callback (stage=%s remote=%s detail=%q)\n", event.BookingID, event.ManagementIP, stage, strings.TrimSpace(event.RemoteAddr), stageDetail)
	} else {
		if stageDetail != "" {
			appLog.Basicf("[PROV] booking=%d Host %s reported install completion callback (stage=%s remote=%s detail=%q)\n", event.BookingID, event.ManagementIP, stage, strings.TrimSpace(event.RemoteAddr), stageDetail)
		} else {
			appLog.Basicf("[PROV] booking=%d Host %s reported install completion callback (stage=%s remote=%s)\n", event.BookingID, event.ManagementIP, stage, strings.TrimSpace(event.RemoteAddr))
		}
	}
	if !anyInProgress {
		appLog.Basicf("[PROV] booking=%d Install completion state is terminal: %s\n", event.BookingID, job.Status)
	}
	persistProvisioningJobLocked(job)

	return nil
}

func startProvisioningWorkflow(bookingID int, owner string, hosts []db.BookingRequestHost, basicConfig bookingDeployBasicConfig, forceRestart bool) {
	ctx, cancel := context.WithCancel(context.Background())
	provisioningLock.Lock()
	setProvisioningCancelFuncLocked(bookingID, cancel)
	provisioningLock.Unlock()
	go runProvisioningWorkflow(ctx, bookingID, owner, hosts, basicConfig, forceRestart)
}

func runProvisioningWorkflow(ctx context.Context, bookingID int, owner string, hosts []db.BookingRequestHost, basicConfig bookingDeployBasicConfig, forceRestart bool) {
	defer func() {
		provisioningLock.Lock()
		consumeProvisioningCancelFuncLocked(bookingID)
		provisioningLock.Unlock()
	}()

	if err := provisioningContextErr(ctx); err != nil {
		return
	}

	globalTemplateData := basicConfig.toTemplateData()

	provisioningSetStatus(bookingID, provisioningStatusRunning)
	provisioningAppendEvent(bookingID, "info", fmt.Sprintf("Provisioning started by %s", owner))
	provisioningAppendEvent(bookingID, "info", fmt.Sprintf("Starting PXE handoff for %d host(s) in parallel", len(hosts)))

	results := make(chan bool, len(hosts))
	var wg sync.WaitGroup
	for _, hostRequest := range hosts {
		hostRequest := hostRequest
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- runProvisioningHost(ctx, bookingID, hostRequest, globalTemplateData, forceRestart)
		}()
	}
	wg.Wait()
	close(results)

	if err := provisioningContextErr(ctx); err != nil {
		return
	}

	failures := 0
	for success := range results {
		if !success {
			failures++
		}
	}

	switch {
	case failures == 0:
		provisioningSetStatus(bookingID, provisioningStatusAwaitingInstall)
		provisioningAppendEvent(bookingID, "info", "PXE handoff complete for all hosts; install completion tracking is still in progress")
	case failures >= len(hosts):
		provisioningSetStatus(bookingID, provisioningStatusFailed)
		provisioningAppendEvent(bookingID, "error", "Provisioning failed for all hosts")
	default:
		provisioningSetStatus(bookingID, provisioningStatusPartialFailed)
		provisioningAppendEvent(bookingID, "warn", "Provisioning completed with host-level failures")
	}
}

func provisioningContextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}

	select {
	case <-ctx.Done():
		return errProvisioningCanceled
	default:
		return nil
	}
}

func provisioningDoneChan(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return nil
	}

	return ctx.Done()
}

func retryProvisioningAction(
	ctx context.Context,
	attempts int,
	initialDelay time.Duration,
	action func() error,
	onRetry func(failedAttempt int, err error, nextDelay time.Duration),
) (err error) {
	if attempts < 1 {
		attempts = 1
	}
	if initialDelay <= 0 {
		initialDelay = time.Second
	}

	delay := initialDelay
	for attempt := 1; attempt <= attempts; attempt++ {
		if err = provisioningContextErr(ctx); err != nil {
			return err
		}

		if err = action(); err == nil {
			return nil
		}
		if errors.Is(err, context.Canceled) {
			return errProvisioningCanceled
		}

		if attempt >= attempts {
			break
		}

		if onRetry != nil {
			onRetry(attempt, err, delay)
		}

		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-provisioningDoneChan(ctx):
			if !timer.Stop() {
				<-timer.C
			}
			return errProvisioningCanceled
		}
		if delay < 30*time.Second {
			delay *= 2
		}
	}

	return err
}

func runProvisioningHost(
	ctx context.Context,
	bookingID int,
	hostRequest db.BookingRequestHost,
	globalTemplateData map[string]string,
	forceRestart bool,
) (success bool) {
	ip := hostRequest.ManagementIP
	hostname := strings.TrimSpace(hostRequest.Hostname)
	hostDisplay := hostname
	if hostDisplay == "" {
		hostDisplay = ip
	}

	if err := provisioningContextErr(ctx); err != nil {
		provisioningUpdateHost(bookingID, ip, "canceled", "Provisioning canceled before host workflow started")
		return false
	}

	bootMode := db.BootModeUEFI
	switch strings.ToLower(strings.TrimSpace(hostRequest.BootMode)) {
	case "legacy":
		bootMode = db.BootModeLegacy
	}

	provisioningUpdateHost(bookingID, ip, "configuring_pxe", "Applying PXE host profile")
	provisioningAppendEvent(bookingID, "info", fmt.Sprintf("Applying PXE override for host %s", ip))

	templateData := mergeTemplateData(globalTemplateData, hostRequest.TemplateData)
	if templateData == nil {
		templateData = map[string]string{}
	}
	templateData["template.boot.mode"] = bootMode.String()
	if hostname != "" {
		templateData["template.identifiers.hostname"] = hostname
	}
	templateData = provisioningInjectCompletionTemplateData(templateData, bookingID, ip)
	assignedIP := strings.TrimSpace(hostRequest.AssignedIPv4)
	provisioningAppendEvent(
		bookingID,
		"info",
		fmt.Sprintf("Host %s deployment config target=%s iso=%q assigned_ip=%q template_keys=%d", ip, hostDisplay, hostRequest.ISOSelection, assignedIP, len(templateData)),
	)

	host, err := db.Hosts.Select(ip)
	if err != nil || host == nil {
		if err == nil {
			err = fmt.Errorf("host not found")
		}
		provisioningUpdateHost(bookingID, ip, "failed", fmt.Sprintf("Failed to load host: %v", err))
		provisioningAppendEvent(bookingID, "error", fmt.Sprintf("Host %s failed: %v", ip, err))
		return false
	}

	override := pxe.ProfileOverride{
		ISOName:      hostRequest.ISOSelection,
		TemplateData: templateData,
	}
	if assignedIP != "" {
		override.IPv4Address = assignedIP
		override.SubnetMask = strings.TrimSpace(templateData["template.network.netmask"])
		override.Gateway = strings.TrimSpace(templateData["template.network.gateway"])
		if dnsCSV := strings.TrimSpace(templateData["template.network.dns"]); dnsCSV != "" {
			override.DNSServers = strings.Split(dnsCSV, ",")
		}
	}

	if err = pxe.ApplyHostProfileOverride(host, override); err != nil {
		provisioningUpdateHost(bookingID, ip, "failed", fmt.Sprintf("Failed to apply PXE profile: %v", err))
		provisioningAppendEvent(bookingID, "error", fmt.Sprintf("Host %s PXE override failed: %v", ip, err))
		return false
	}

	provisioningUpdateHost(bookingID, ip, "setting_boot", "Setting one-time PXE boot")
	provisioningAppendEvent(bookingID, "info", fmt.Sprintf("Setting one-time PXE boot for host %s", ip))

	managementOwned := false
	if host.Management == nil {
		var management *db.HostManagementClient
		if err = retryProvisioningAction(
			ctx,
			5,
			2*time.Second,
			func() error {
				var createErr error
				management, createErr = db.NewHostManagementClient(host)
				if createErr != nil {
					if management != nil {
						management.Close()
					}
					return createErr
				}
				return nil
			},
			func(failedAttempt int, retryErr error, nextDelay time.Duration) {
				provisioningAppendEvent(
					bookingID,
					"warn",
					fmt.Sprintf(
						"Host %s management client creation attempt %d/5 failed: %v (retrying in %s)",
						ip,
						failedAttempt,
						retryErr,
						nextDelay.Round(time.Second),
					),
				)
			},
		); err != nil {
			if errors.Is(err, errProvisioningCanceled) {
				provisioningUpdateHost(bookingID, ip, "canceled", "Provisioning canceled while creating management client")
				return false
			}
			provisioningUpdateHost(bookingID, ip, "failed", fmt.Sprintf("Failed to create management client: %v", err))
			provisioningAppendEvent(bookingID, "error", fmt.Sprintf("Host %s management client creation failed: %v", ip, err))
			return false
		}

		host.Management = management
		managementOwned = true
	}

	defer func() {
		if managementOwned && host.Management != nil {
			host.Management.Close()
			host.Management = nil
		}
	}()

	if currentPower, powerErr := host.Management.PowerState(true); powerErr != nil {
		provisioningAppendEvent(bookingID, "warn", fmt.Sprintf("Host %s initial power-state read failed: %v", ip, powerErr))
	} else {
		provisioningAppendEvent(bookingID, "info", fmt.Sprintf("Host %s initial power state: %s", ip, currentPower.String()))
	}

	successfulCycle := false
	for attempt := 1; attempt <= 2; attempt++ {
		if err = provisioningContextErr(ctx); err != nil {
			provisioningUpdateHost(bookingID, ip, "canceled", "Provisioning canceled during PXE handoff")
			return false
		}

		provisioningUpdateHost(bookingID, ip, "setting_boot", fmt.Sprintf("Attempt %d/2: setting one-time PXE boot", attempt))
		provisioningAppendEvent(bookingID, "info", fmt.Sprintf("Host %s attempt %d/2 setting one-time PXE boot", ip, attempt))

		if err = retryProvisioningAction(
			ctx,
			3,
			2*time.Second,
			func() error {
				return host.Management.SetPXEBoot(bootMode)
			},
			func(failedAttempt int, retryErr error, nextDelay time.Duration) {
				provisioningAppendEvent(
					bookingID,
					"warn",
					fmt.Sprintf(
						"Host %s attempt %d/2 set PXE boot transient failure %d/3: %v (retrying in %s)",
						ip,
						attempt,
						failedAttempt,
						retryErr,
						nextDelay.Round(time.Second),
					),
				)
			},
		); err != nil {
			if errors.Is(err, errProvisioningCanceled) {
				provisioningUpdateHost(bookingID, ip, "canceled", "Provisioning canceled while setting PXE boot")
				return false
			}
			provisioningAppendEvent(bookingID, "error", fmt.Sprintf("Host %s attempt %d/2 set PXE boot failed: %v", ip, attempt, err))
			continue
		}

		provisioningUpdateHost(bookingID, ip, "restarting", fmt.Sprintf("Attempt %d/2: restarting host into PXE", attempt))
		provisioningAppendEvent(bookingID, "info", fmt.Sprintf("Host %s attempt %d/2 restarting (force=%t)", ip, attempt, forceRestart))
		if err = retryProvisioningAction(
			ctx,
			3,
			2*time.Second,
			func() error {
				return host.Management.ResetPowerState(forceRestart)
			},
			func(failedAttempt int, retryErr error, nextDelay time.Duration) {
				provisioningAppendEvent(
					bookingID,
					"warn",
					fmt.Sprintf(
						"Host %s attempt %d/2 restart transient failure %d/3: %v (retrying in %s)",
						ip,
						attempt,
						failedAttempt,
						retryErr,
						nextDelay.Round(time.Second),
					),
				)
			},
		); err != nil {
			if errors.Is(err, errProvisioningCanceled) {
				provisioningUpdateHost(bookingID, ip, "canceled", "Provisioning canceled while restarting host")
				return false
			}
			provisioningAppendEvent(bookingID, "error", fmt.Sprintf("Host %s attempt %d/2 restart failed: %v", ip, attempt, err))
			continue
		}

		// Restart does not always expose an OFF transition; log best effort and continue.
		if err = waitForPowerStateWithLogs(ctx, bookingID, ip, host.Management, db.PowerStateOff, 90*time.Second); err != nil {
			if errors.Is(err, errProvisioningCanceled) {
				provisioningUpdateHost(bookingID, ip, "canceled", "Provisioning canceled while waiting for power cycle")
				return false
			}
			provisioningAppendEvent(bookingID, "warn", fmt.Sprintf("Host %s attempt %d/2 did not report OFF: %v", ip, attempt, err))
		}

		provisioningUpdateHost(bookingID, ip, "waiting_power", fmt.Sprintf("Attempt %d/2: waiting for host power on", attempt))
		if err = waitForPowerStateWithLogs(ctx, bookingID, ip, host.Management, db.PowerStateOn, 12*time.Minute); err != nil {
			if errors.Is(err, errProvisioningCanceled) {
				provisioningUpdateHost(bookingID, ip, "canceled", "Provisioning canceled while waiting for host power on")
				return false
			}
			provisioningAppendEvent(bookingID, "warn", fmt.Sprintf("Host %s attempt %d/2 did not reach ON: %v", ip, attempt, err))
			continue
		}

		if rebootDetected, monitorErr := detectEarlyRebootAfterPowerOn(ctx, bookingID, ip, host.Management, 2*time.Minute); monitorErr != nil {
			if errors.Is(monitorErr, errProvisioningCanceled) {
				provisioningUpdateHost(bookingID, ip, "canceled", "Provisioning canceled while monitoring reboot state")
				return false
			}
			provisioningAppendEvent(bookingID, "warn", fmt.Sprintf("Host %s reboot-monitor warning: %v", ip, monitorErr))
		} else if rebootDetected && attempt < 2 {
			provisioningAppendEvent(bookingID, "warn", fmt.Sprintf("Host %s rebooted again shortly after power-on; retrying PXE handoff", ip))
			continue
		}

		successfulCycle = true
		break
	}

	if !successfulCycle {
		provisioningUpdateHost(bookingID, ip, "failed", "PXE boot handoff failed after 2 attempts")
		provisioningAppendEvent(bookingID, "error", fmt.Sprintf("Host %s failed PXE handoff after retry attempts", ip))
		return false
	}

	host.LastKnownPowerState = db.PowerStateOn
	host.LastKnownPowerStateTime = time.Now()
	if err = db.Hosts.Update(host); err != nil {
		provisioningAppendEvent(bookingID, "warn", fmt.Sprintf("Host %s power-state update warning: %v", ip, err))
	}

	awaitingMessage := "PXE boot handoff complete; waiting for installer/runtime completion"
	if assignedIP != "" {
		awaitingMessage = fmt.Sprintf("%s (target_ip=%s)", awaitingMessage, assignedIP)
	}
	provisioningUpdateHost(bookingID, ip, "awaiting_install", awaitingMessage)
	provisioningAppendEvent(bookingID, "info", fmt.Sprintf("Host %s entered awaiting-install stage", ip))
	return true
}

func waitForPowerStateWithLogs(
	ctx context.Context,
	bookingID int,
	managementIP string,
	management *db.HostManagementClient,
	desired db.PowerState,
	timeout time.Duration,
) (err error) {
	if management == nil {
		return fmt.Errorf("management client is nil")
	}

	start := time.Now()
	deadline := start.Add(timeout)
	lastState := db.PowerStateUnknown
	lastProgressLog := time.Time{}

	for time.Now().Before(deadline) {
		if err = provisioningContextErr(ctx); err != nil {
			return err
		}

		state, readErr := management.PowerState(true)
		if readErr != nil {
			provisioningAppendEvent(bookingID, "warn", fmt.Sprintf("Host %s power poll error: %v", managementIP, readErr))
			timer := time.NewTimer(5 * time.Second)
			select {
			case <-timer.C:
			case <-provisioningDoneChan(ctx):
				if !timer.Stop() {
					<-timer.C
				}
				return errProvisioningCanceled
			}
			continue
		}

		if state != lastState {
			provisioningAppendEvent(bookingID, "info", fmt.Sprintf("Host %s power transition: %s", managementIP, state.String()))
			lastState = state
		}

		if state == desired {
			provisioningAppendEvent(bookingID, "info", fmt.Sprintf("Host %s reached power state %s after %s", managementIP, desired.String(), time.Since(start).Round(time.Second)))
			return nil
		}

		if lastProgressLog.IsZero() || time.Since(lastProgressLog) >= 30*time.Second {
			provisioningAppendEvent(
				bookingID,
				"info",
				fmt.Sprintf("Host %s still waiting for %s (current=%s elapsed=%s)", managementIP, desired.String(), state.String(), time.Since(start).Round(time.Second)),
			)
			lastProgressLog = time.Now()
		}

		timer := time.NewTimer(5 * time.Second)
		select {
		case <-timer.C:
		case <-provisioningDoneChan(ctx):
			if !timer.Stop() {
				<-timer.C
			}
			return errProvisioningCanceled
		}
	}

	err = fmt.Errorf("timeout after %s waiting for %s", timeout.Round(time.Second), desired.String())
	return
}

func detectEarlyRebootAfterPowerOn(
	ctx context.Context,
	bookingID int,
	managementIP string,
	management *db.HostManagementClient,
	window time.Duration,
) (detected bool, err error) {
	if management == nil {
		return false, fmt.Errorf("management client is nil")
	}

	deadline := time.Now().Add(window)
	sawOff := false
	for time.Now().Before(deadline) {
		if err = provisioningContextErr(ctx); err != nil {
			return false, err
		}

		state, stateErr := management.PowerState(true)
		if stateErr != nil {
			provisioningAppendEvent(bookingID, "warn", fmt.Sprintf("Host %s reboot monitor poll error: %v", managementIP, stateErr))
			timer := time.NewTimer(5 * time.Second)
			select {
			case <-timer.C:
			case <-provisioningDoneChan(ctx):
				if !timer.Stop() {
					<-timer.C
				}
				return false, errProvisioningCanceled
			}
			continue
		}

		if state == db.PowerStateOff && !sawOff {
			sawOff = true
			provisioningAppendEvent(bookingID, "warn", fmt.Sprintf("Host %s powered off shortly after power-on", managementIP))
		}

		if sawOff && state == db.PowerStateOn {
			provisioningAppendEvent(bookingID, "warn", fmt.Sprintf("Host %s powered back on during reboot monitor window", managementIP))
			return true, nil
		}

		timer := time.NewTimer(10 * time.Second)
		select {
		case <-timer.C:
		case <-provisioningDoneChan(ctx):
			if !timer.Stop() {
				<-timer.C
			}
			return false, errProvisioningCanceled
		}
	}

	return false, nil
}

func mergeTemplateData(global map[string]string, perHost map[string]string) (out map[string]string) {
	if len(global) == 0 && len(perHost) == 0 {
		return nil
	}

	out = map[string]string{}
	for key, value := range global {
		out[key] = value
	}

	for key, value := range perHost {
		out[key] = value
	}

	return
}
