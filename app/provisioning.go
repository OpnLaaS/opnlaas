package app

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
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
	provisioningStatusDestroyed       = "destroyed"
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

var (
	provisioningLock sync.RWMutex
	provisioningJobs = map[int]*apiProvisioningStatus{}
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
		job.Hosts = append(job.Hosts, &apiProvisioningHostState{
			ManagementIP:    host.ManagementIP,
			ISOSelection:    host.ISOSelection,
			Status:          "reserved",
			Message:         "Host reserved for booking",
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
	provisioningLock.Unlock()

	return job.clone()
}

func provisioningSnapshot(bookingID int) (snapshot *apiProvisioningStatus, exists bool) {
	provisioningLock.RLock()
	job, ok := provisioningJobs[bookingID]
	provisioningLock.RUnlock()
	if !ok || job == nil {
		return nil, false
	}

	return job.clone(), true
}

func provisioningSetStatus(bookingID int, status string) {
	provisioningLock.Lock()
	defer provisioningLock.Unlock()

	job, ok := provisioningJobs[bookingID]
	if !ok || job == nil {
		return
	}

	now := time.Now()
	job.Status = status
	job.UpdatedAt = now
	if status == provisioningStatusCompleted || status == provisioningStatusFailed || status == provisioningStatusPartialFailed || status == provisioningStatusDestroyed {
		job.FinishedAt = &now
	}
}

func provisioningAppendEvent(bookingID int, level string, message string) {
	provisioningLock.Lock()
	defer provisioningLock.Unlock()

	job, ok := provisioningJobs[bookingID]
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
}

func provisioningUpdateHost(bookingID int, managementIP string, status string, message string) {
	provisioningLock.Lock()
	defer provisioningLock.Unlock()

	job, ok := provisioningJobs[bookingID]
	if !ok || job == nil {
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
		return
	}
}

func provisioningDeleteJob(bookingID int) {
	provisioningLock.Lock()
	defer provisioningLock.Unlock()
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
	provisioningLock.RLock()
	defer provisioningLock.RUnlock()

	job, ok := provisioningJobs[bookingID]
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

	job, ok := provisioningJobs[event.BookingID]
	if !ok || job == nil {
		return fmt.Errorf("booking provisioning state not found")
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

	return nil
}

func startProvisioningWorkflow(bookingID int, owner string, hosts []db.BookingRequestHost, basicConfig bookingDeployBasicConfig, bootMode db.BootMode, forceRestart bool) {
	go runProvisioningWorkflow(bookingID, owner, hosts, basicConfig, bootMode, forceRestart)
}

func runProvisioningWorkflow(bookingID int, owner string, hosts []db.BookingRequestHost, basicConfig bookingDeployBasicConfig, bootMode db.BootMode, forceRestart bool) {
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
			results <- runProvisioningHost(bookingID, hostRequest, globalTemplateData, bootMode, forceRestart)
		}()
	}
	wg.Wait()
	close(results)

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

func retryProvisioningAction(
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
		if err = action(); err == nil {
			return nil
		}

		if attempt >= attempts {
			break
		}

		if onRetry != nil {
			onRetry(attempt, err, delay)
		}

		time.Sleep(delay)
		if delay < 30*time.Second {
			delay *= 2
		}
	}

	return err
}

func runProvisioningHost(
	bookingID int,
	hostRequest db.BookingRequestHost,
	globalTemplateData map[string]string,
	bootMode db.BootMode,
	forceRestart bool,
) (success bool) {
	ip := hostRequest.ManagementIP

	provisioningUpdateHost(bookingID, ip, "configuring_pxe", "Applying PXE host profile")
	provisioningAppendEvent(bookingID, "info", fmt.Sprintf("Applying PXE override for host %s", ip))

	templateData := mergeTemplateData(globalTemplateData, hostRequest.TemplateData)
	if templateData == nil {
		templateData = map[string]string{}
	}
	templateData["template.boot.mode"] = bootMode.String()
	templateData = provisioningInjectCompletionTemplateData(templateData, bookingID, ip)
	assignedIP := strings.TrimSpace(hostRequest.AssignedIPv4)
	provisioningAppendEvent(
		bookingID,
		"info",
		fmt.Sprintf("Host %s deployment config iso=%q assigned_ip=%q template_keys=%d", ip, hostRequest.ISOSelection, assignedIP, len(templateData)),
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
		provisioningUpdateHost(bookingID, ip, "setting_boot", fmt.Sprintf("Attempt %d/2: setting one-time PXE boot", attempt))
		provisioningAppendEvent(bookingID, "info", fmt.Sprintf("Host %s attempt %d/2 setting one-time PXE boot", ip, attempt))

		if err = retryProvisioningAction(
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
			provisioningAppendEvent(bookingID, "error", fmt.Sprintf("Host %s attempt %d/2 set PXE boot failed: %v", ip, attempt, err))
			continue
		}

		provisioningUpdateHost(bookingID, ip, "restarting", fmt.Sprintf("Attempt %d/2: restarting host into PXE", attempt))
		provisioningAppendEvent(bookingID, "info", fmt.Sprintf("Host %s attempt %d/2 restarting (force=%t)", ip, attempt, forceRestart))
		if err = retryProvisioningAction(
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
			provisioningAppendEvent(bookingID, "error", fmt.Sprintf("Host %s attempt %d/2 restart failed: %v", ip, attempt, err))
			continue
		}

		// Restart does not always expose an OFF transition; log best effort and continue.
		if err = waitForPowerStateWithLogs(bookingID, ip, host.Management, db.PowerStateOff, 90*time.Second); err != nil {
			provisioningAppendEvent(bookingID, "warn", fmt.Sprintf("Host %s attempt %d/2 did not report OFF: %v", ip, attempt, err))
		}

		provisioningUpdateHost(bookingID, ip, "waiting_power", fmt.Sprintf("Attempt %d/2: waiting for host power on", attempt))
		if err = waitForPowerStateWithLogs(bookingID, ip, host.Management, db.PowerStateOn, 12*time.Minute); err != nil {
			provisioningAppendEvent(bookingID, "warn", fmt.Sprintf("Host %s attempt %d/2 did not reach ON: %v", ip, attempt, err))
			continue
		}

		if rebootDetected, monitorErr := detectEarlyRebootAfterPowerOn(bookingID, ip, host.Management, 2*time.Minute); monitorErr != nil {
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
		state, readErr := management.PowerState(true)
		if readErr != nil {
			provisioningAppendEvent(bookingID, "warn", fmt.Sprintf("Host %s power poll error: %v", managementIP, readErr))
			time.Sleep(5 * time.Second)
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

		time.Sleep(5 * time.Second)
	}

	err = fmt.Errorf("timeout after %s waiting for %s", timeout.Round(time.Second), desired.String())
	return
}

func detectEarlyRebootAfterPowerOn(
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
		state, stateErr := management.PowerState(true)
		if stateErr != nil {
			provisioningAppendEvent(bookingID, "warn", fmt.Sprintf("Host %s reboot monitor poll error: %v", managementIP, stateErr))
			time.Sleep(5 * time.Second)
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

		time.Sleep(10 * time.Second)
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
