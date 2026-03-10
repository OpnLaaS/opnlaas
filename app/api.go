package app

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"mime/multipart"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/opnlaas/opnlaas/auth"
	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/db"
	"github.com/opnlaas/opnlaas/host/iso"
	"github.com/opnlaas/opnlaas/host/pxe"
)

func apiLogin(c *fiber.Ctx) (err error) {
	var (
		username, password string = c.FormValue("username"), c.FormValue("password")
		user               *auth.AuthUser
		token              string
	)

	if user, err = auth.Authenticate(username, password); err == nil {
		if token, err = user.Token.SignedString(jwtSigningKey); err == nil {
			c.Cookie(&fiber.Cookie{
				Name:  "Authorization",
				Value: token,
			})

			// "no_redirect" is used to prevent JS api errors and to keep api and form action URIs the same
			if c.Query("no_redirect") == "1" {
				return c.SendStatus(fiber.StatusOK)
			}

			return c.Redirect("/")
		}
	}

	if c.Query("no_redirect") == "1" {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	return c.Render("login", fiber.Map{
		"Title":      "Login",
		"LoginError": err.Error(),
	}, "layout")
}

func apiLogout(c *fiber.Ctx) (err error) {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	if user != nil {
		auth.Logout(user.Username)
	}

	// Must replace cookie as some browsers require a valid replacement before deletion
	c.Cookie(&fiber.Cookie{
		Name:    "Authorization",
		Value:   "",
		Expires: time.Now().Add(-time.Hour),
	})
	return
}

type authProfile struct {
	Username    string   `json:"username"`
	DisplayName string   `json:"display_name"`
	Email       string   `json:"email,omitempty"`
	Groups      []string `json:"groups,omitempty"`
	Permissions string   `json:"permissions"`
	IsAdmin     bool     `json:"is_admin"`
}

func apiAuthMe(c *fiber.Ctx) (err error) {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	perms := user.Permissions()
	profile := authProfile{
		Username:    user.Username,
		DisplayName: user.Username,
		Permissions: perms.String(),
		IsAdmin:     perms >= auth.AuthPermsAdministrator,
	}

	if user.LDAPConn != nil {
		if displayName, err := user.LDAPConn.DisplayName(); err == nil && displayName != "" {
			profile.DisplayName = displayName
		}

		if email, err := user.LDAPConn.Email(); err == nil && email != "" {
			profile.Email = email
		}

		if groups, err := user.LDAPConn.Groups(); err == nil && len(groups) > 0 {
			profile.Groups = groups
		}
	}

	return c.JSON(profile)
}

type apiBookingCreateBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Duration    int    `json:"duration_days"`
	CIDRBlock   string `json:"cidr_block,omitempty"`
}

type apiBookingView struct {
	Booking         *db.Booking                    `json:"booking"`
	PermissionLevel db.BookingPermissionLevel      `json:"permission_level"`
	Provisioning    *apiBookingProvisioningSummary `json:"provisioning,omitempty"`
	Hosts           []*db.Host                     `json:"hosts"`
	Credentials     apiProvisioningCredentials     `json:"credentials"`
}

type apiBookingProvisioningSummary struct {
	Status         string     `json:"status"`
	UpdatedAt      time.Time  `json:"updated_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	HostsTotal     int        `json:"hosts_total"`
	HostsCompleted int        `json:"hosts_completed"`
	HostsFailed    int        `json:"hosts_failed"`
	Cancelable     bool       `json:"cancelable"`
}

type apiBookingNetworkPrefillView struct {
	SupernetCIDR      string   `json:"supernet_cidr"`
	BookingPrefix     int      `json:"booking_prefix"`
	HostNetworkPrefix int      `json:"host_network_prefix"`
	SuggestedCIDR     string   `json:"suggested_cidr"`
	GatewayIPv4       string   `json:"gateway_ipv4"`
	DNSServers        []string `json:"dns_servers"`
	HostStartOffset   int      `json:"host_start_offset"`
	DisableOtherNICs  bool     `json:"disable_other_nics"`
	DefaultDuration   int      `json:"default_duration_days"`
	MaxDuration       int      `json:"max_duration_days"`
	ServerDNSName     string   `json:"server_dns_name"`
}

type apiBookingCartHostUpsertBody struct {
	ManagementIP     string            `json:"management_ip"`
	ISOSelection     string            `json:"iso_selection"`
	BootMode         string            `json:"boot_mode,omitempty"`
	Hostname         string            `json:"hostname,omitempty"`
	AssignedIPv4     string            `json:"assigned_ipv4,omitempty"`
	AssignedHostBits *int              `json:"assigned_host_bits,omitempty"`
	TemplateData     map[string]string `json:"template_data,omitempty"`
}

func parseBookingIDParam(c *fiber.Ctx) (bookingID int, err error) {
	var bookingID64 int64
	if bookingID64, err = strconv.ParseInt(c.Params("booking_id"), 10, 32); err != nil {
		return 0, fmt.Errorf("invalid booking id")
	}

	bookingID = int(bookingID64)
	return
}

func bookingDurationBounds() (defaultDays int, maxDays int) {
	defaultDays = config.Config.Booking.DefaultDurationDays
	maxDays = config.Config.Booking.MaxDurationDays

	if defaultDays < 1 {
		defaultDays = 32
	}
	if maxDays < 1 {
		maxDays = defaultDays
	}
	if defaultDays > maxDays {
		defaultDays = maxDays
	}

	return
}

func bookingServerDNSName() (dnsName string) {
	dnsName = strings.ToLower(strings.TrimSpace(config.Config.Booking.ServerDNSName))
	dnsName = strings.Trim(dnsName, ".")
	if dnsName == "" {
		dnsName = "laas.cyber.lab"
	}
	return
}

func normalizeBookingDuration(raw int) (days int, err error) {
	defaultDays, maxDays := bookingDurationBounds()
	if raw == 0 {
		return defaultDays, nil
	}

	if raw < 1 || raw > maxDays {
		return 0, fmt.Errorf("duration_days must be between 1 and %d", maxDays)
	}

	return raw, nil
}

func sanitizeBookingDNSLabel(name string) string {
	raw := strings.TrimSpace(strings.ToLower(name))
	if raw == "" {
		return "booking"
	}

	var builder strings.Builder
	lastWasDash := true
	for _, r := range raw {
		isLowerLetter := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		if isLowerLetter || isDigit {
			builder.WriteRune(r)
			lastWasDash = false
			continue
		}

		// Collapse apostrophes instead of adding separators (e.g. "Evan's" -> "evans").
		if r == '\'' || r == '’' {
			continue
		}

		if !lastWasDash {
			builder.WriteByte('-')
			lastWasDash = true
		}
	}

	label := strings.Trim(builder.String(), "-")
	if label == "" {
		label = "booking"
	}

	if len(label) > 48 {
		label = strings.Trim(label[:48], "-")
	}
	if label == "" {
		label = "booking"
	}

	return label
}

func sanitizeHostnameLabel(label string) string {
	raw := strings.TrimSpace(strings.ToLower(label))
	if raw == "" {
		return ""
	}

	var builder strings.Builder
	lastWasDash := true
	for _, r := range raw {
		isLowerLetter := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		if isLowerLetter || isDigit {
			builder.WriteRune(r)
			lastWasDash = false
			continue
		}

		if r == '\'' || r == '’' {
			continue
		}

		if !lastWasDash {
			builder.WriteByte('-')
			lastWasDash = true
		}
	}

	cleaned := strings.Trim(builder.String(), "-")
	if len(cleaned) > 48 {
		cleaned = strings.Trim(cleaned[:48], "-")
	}
	return cleaned
}

func bookingDNSCandidate(base string, collisionIndex int) (candidate string) {
	base = strings.TrimSpace(strings.Trim(base, "-"))
	if base == "" {
		base = "booking"
	}

	if collisionIndex <= 0 {
		if len(base) > 63 {
			base = strings.Trim(base[:63], "-")
		}
		if base == "" {
			return "booking"
		}
		return base
	}

	suffix := fmt.Sprintf("-%d", collisionIndex+1)
	allowed := 63 - len(suffix)
	if allowed < 1 {
		allowed = 1
	}

	if len(base) > allowed {
		base = strings.Trim(base[:allowed], "-")
	}
	if base == "" {
		base = "booking"
	}

	return base + suffix
}

var bookingHostnameWordList = []string{
	"amber", "apex", "arc", "atlas", "beacon", "binary", "bolt", "bravo", "byte", "cinder",
	"cobalt", "comet", "copper", "core", "crane", "crest", "dash", "delta", "drift", "echo",
	"ember", "falcon", "flare", "flux", "forge", "frost", "gale", "gamma", "glint", "graph",
	"haven", "helix", "horizon", "hydra", "ion", "jade", "jet", "jolt", "kilo", "lattice",
	"level", "lumen", "lynx", "magnet", "matrix", "merit", "metric", "mint", "mirage", "mosaic",
	"nexus", "node", "nova", "nyx", "onyx", "orbit", "origin", "otter", "oxide", "pacer",
	"patch", "phoenix", "pixel", "plasma", "pulse", "quantum", "quartz", "radar", "ranger", "reactor",
	"reef", "rivet", "rocket", "rogue", "rune", "sable", "sage", "saturn", "scout", "sector",
	"shade", "signal", "skyline", "slate", "solstice", "spark", "spire", "spruce", "stack", "starling",
	"stride", "summit", "switch", "talon", "tango", "thunder", "topaz", "torch", "tracer", "vector",
	"verge", "vertex", "viper", "vista", "vivid", "warp", "whisper", "willow", "xenon", "yonder",
	"zephyr", "zeta",
}

func randomBookingHostnameWord() (word string, err error) {
	if len(bookingHostnameWordList) == 0 {
		return "", fmt.Errorf("booking hostname words are not configured")
	}

	max := big.NewInt(int64(len(bookingHostnameWordList)))
	idx, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}

	word = bookingHostnameWordList[idx.Int64()]
	return
}

func generateBookingHostnames(hosts []db.BookingRequestHost, bookingDNSName string) (updated []db.BookingRequestHost, err error) {
	bookingDNSName = strings.TrimSpace(bookingDNSName)
	if bookingDNSName == "" {
		return nil, fmt.Errorf("booking dns name is required")
	}
	serverDNSName := bookingServerDNSName()

	usedLabels := map[string]struct{}{}
	for i := range hosts {
		baseLabel := sanitizeHostnameLabel(hosts[i].Hostname)
		if baseLabel == "" {
			for attempt := 0; attempt < 12; attempt++ {
				if baseLabel, err = randomBookingHostnameWord(); err != nil {
					return nil, err
				}
				if _, exists := usedLabels[baseLabel]; !exists {
					break
				}
				baseLabel = ""
			}
		}

		if baseLabel == "" {
			baseLabel = fmt.Sprintf("node%d", i+1)
		}

		label := baseLabel
		for collisionIndex := 0; ; collisionIndex++ {
			candidate := bookingDNSCandidate(baseLabel, collisionIndex)
			if _, exists := usedLabels[candidate]; !exists {
				label = candidate
				break
			}
		}
		usedLabels[label] = struct{}{}

		hostname := fmt.Sprintf("%s.%s.%s", label, bookingDNSName, serverDNSName)
		hosts[i].Hostname = hostname
		if hosts[i].TemplateData == nil {
			hosts[i].TemplateData = map[string]string{}
		}
		hosts[i].TemplateData["template.identifiers.hostname"] = hostname
	}

	return hosts, nil
}

func buildBookingProvisioningSummary(snapshot *apiProvisioningStatus) *apiBookingProvisioningSummary {
	if snapshot == nil {
		return nil
	}

	summary := &apiBookingProvisioningSummary{
		Status:     snapshot.Status,
		UpdatedAt:  snapshot.UpdatedAt,
		HostsTotal: len(snapshot.Hosts),
		Cancelable: provisioningStatusIsCancelable(snapshot.Status),
	}

	if snapshot.FinishedAt != nil {
		finished := *snapshot.FinishedAt
		summary.FinishedAt = &finished
	}

	for _, host := range snapshot.Hosts {
		if host == nil {
			continue
		}

		switch strings.ToLower(strings.TrimSpace(host.Status)) {
		case "completed":
			summary.HostsCompleted++
		case "failed":
			summary.HostsFailed++
		}
	}

	return summary
}

func createBookingForOwner(owner string, body apiBookingCreateBody) (booking *db.Booking, ownerRecord *db.BookingPerson, err error) {
	body.Name = strings.TrimSpace(body.Name)
	body.Description = strings.TrimSpace(body.Description)
	if body.Name == "" {
		err = newInputValidationError("name is required")
		return
	}
	if err = validateNoProfanity("name", body.Name); err != nil {
		return
	}
	if err = validateNoProfanity("description", body.Description); err != nil {
		return
	}

	durationDays, err := normalizeBookingDuration(body.Duration)
	if err != nil {
		return nil, nil, err
	}

	requestedCIDR := strings.TrimSpace(body.CIDRBlock)
	autoAllocateCIDR := requestedCIDR == ""
	if requestedCIDR, err = resolveBookingCIDRForCreate(requestedCIDR); err != nil {
		err = fmt.Errorf("resolve booking cidr: %w", err)
		return
	}

	dnsBase := sanitizeBookingDNSLabel(body.Name)
	dnsCollisionIndex := 0
	now := time.Now()
	for attempt := 1; attempt <= 12; attempt++ {
		dnsCandidate := bookingDNSCandidate(dnsBase, dnsCollisionIndex)
		var existingDNSBooking *db.Booking
		if existingDNSBooking, err = db.BookingByDNSName(dnsCandidate); err != nil {
			err = fmt.Errorf("check booking dns uniqueness: %w", err)
			return
		}
		if existingDNSBooking != nil {
			dnsCollisionIndex += 1
			continue
		}

		booking = &db.Booking{
			Name:        strings.TrimSpace(body.Name),
			Description: body.Description,
			Status:      db.BookingStatusActive,
			StartTime:   now,
			DNSName:     dnsCandidate,
			CIDRBlock:   requestedCIDR,
		}

		booking.EndTime = now.Add(time.Duration(durationDays) * 24 * time.Hour)

		if err = db.CreateBooking(booking); err == nil {
			break
		}

		shouldRetry := false
		if autoAllocateCIDR && isBookingCIDRUniqueError(err) {
			shouldRetry = true
			if requestedCIDR, err = resolveBookingCIDRForCreate(""); err != nil {
				err = fmt.Errorf("resolve booking cidr retry: %w", err)
				return
			}
		}
		if isBookingDNSUniqueError(err) {
			shouldRetry = true
			dnsCollisionIndex += 1
		}

		if !shouldRetry || attempt >= 12 {
			err = fmt.Errorf("create booking: %w", err)
			return
		}
	}

	ownerRecord = &db.BookingPerson{
		Username:        owner,
		BookingID:       booking.ID,
		PermissionLevel: db.BookingPermissionLevelOwner,
	}

	if err = db.AddBookingPerson(ownerRecord); err != nil {
		_ = db.DeleteBooking(booking.ID)
		err = fmt.Errorf("set booking owner: %w", err)
		return
	}

	booking.People = append(booking.People, ownerRecord.ID)
	return
}

func resolveBookingCIDRForCreate(raw string) (cidr string, err error) {
	defaults, err := loadBookingNetworkDefaults()
	if err != nil {
		return "", err
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		var suggested netip.Prefix
		if suggested, err = suggestNextBookingCIDR(defaults); err != nil {
			return "", err
		}
		return suggested.String(), nil
	}

	prefix, _, err := validateBookingCIDR(defaults, raw, 0)
	if err != nil {
		return "", err
	}

	return prefix.String(), nil
}

func isBookingCIDRUniqueError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") && strings.Contains(msg, "cidr_block")
}

func isBookingDNSUniqueError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "unique") {
		return false
	}

	return strings.Contains(msg, "dns_name") ||
		strings.Contains(msg, "dns name") ||
		strings.Contains(msg, "dnsname")
}

func ensureCartNetworkForUser(username string, defaults bookingNetworkDefaults) (cidr netip.Prefix, gateway netip.Addr, err error) {
	targetCIDR := ""
	if cart, cartErr := db.BookingCartSnapshot(username); cartErr == nil && cart != nil {
		targetCIDR = strings.TrimSpace(cart.NetworkCIDR)
	} else if cartErr != nil && !errors.Is(cartErr, db.ErrCartNotFound) {
		return cidr, gateway, fmt.Errorf("load cart: %w", cartErr)
	}

	if targetCIDR == "" {
		var suggested netip.Prefix
		if suggested, err = suggestNextBookingCIDR(defaults); err != nil {
			return cidr, gateway, err
		}
		targetCIDR = suggested.String()
	}

	if cidr, gateway, err = validateBookingCIDR(defaults, targetCIDR, 0); err != nil {
		return cidr, gateway, err
	}

	if err = db.SetCartNetwork(username, cidr.String(), gateway.String(), defaults.DNSServers); err != nil {
		return cidr, gateway, err
	}

	return cidr, gateway, nil
}

func bookingPermissionForUser(username string, bookingID int) (level db.BookingPermissionLevel, allowed bool, err error) {
	var people []*db.BookingPerson
	if people, err = db.BookingPeopleForBooking(bookingID); err != nil {
		return
	}

	for _, person := range people {
		if person == nil || person.Username != username {
			continue
		}

		level = person.PermissionLevel
		allowed = true
		return
	}

	level = db.BookingPermissionLevelNone
	return
}

func userCanControlHost(user *auth.AuthUser, host *db.Host) (allowed bool, err error) {
	if user == nil || host == nil {
		return false, nil
	}

	if user.Permissions() >= auth.AuthPermsAdministrator {
		return true, nil
	}

	if !host.IsBooked || host.ActiveBookingID == 0 {
		return false, nil
	}

	level, ok, err := bookingPermissionForUser(user.Username, host.ActiveBookingID)
	if err != nil {
		return false, err
	}

	return ok && level >= db.BookingPermissionLevelOperator, nil
}

func userCanViewBooking(user *auth.AuthUser, bookingID int) (allowed bool, level db.BookingPermissionLevel, err error) {
	if user == nil {
		return false, db.BookingPermissionLevelNone, nil
	}

	if user.Permissions() >= auth.AuthPermsAdministrator {
		return true, db.BookingPermissionLevelOwner, nil
	}

	level, ok, err := bookingPermissionForUser(user.Username, bookingID)
	if err != nil {
		return false, db.BookingPermissionLevelNone, err
	}

	if !ok {
		return false, db.BookingPermissionLevelNone, nil
	}

	return true, level, nil
}

func userCanDestroyBooking(user *auth.AuthUser, bookingID int) (allowed bool, err error) {
	if user == nil {
		return false, nil
	}

	if user.Permissions() >= auth.AuthPermsAdministrator {
		return true, nil
	}

	level, ok, err := bookingPermissionForUser(user.Username, bookingID)
	if err != nil {
		return false, err
	}

	return ok && level >= db.BookingPermissionLevelOwner, nil
}

// Enums API

func apiEnumsVendorNames(c *fiber.Ctx) (err error) {
	return c.JSON(db.VendorNameReverses)
}

func apiEnumsFormFactorNames(c *fiber.Ctx) (err error) {
	return c.JSON(db.FormFactorNameReverses)
}

func apiEnumsManagementTypeNames(c *fiber.Ctx) (err error) {
	return c.JSON(db.ManagementTypeNameReverses)
}

func apiEnumsPowerStateNames(c *fiber.Ctx) (err error) {
	return c.JSON(db.PowerStateNameReverses)
}

func apiEnumsBootModeNames(c *fiber.Ctx) (err error) {
	return c.JSON(db.BootModeNameReverses)
}

func apiEnumsPowerActionNames(c *fiber.Ctx) (err error) {
	return c.JSON(db.PowerActionNameReverses)
}

func apiEnumsArchitectureNames(c *fiber.Ctx) (err error) {
	return c.JSON(db.ArchitectureNameReverses)
}

func apiEnumsDistroTypeNames(c *fiber.Ctx) (err error) {
	return c.JSON(db.DistroTypeNameReverses)
}

func apiEnumsPreConfigureTypeNames(c *fiber.Ctx) (err error) {
	return c.JSON(db.PreConfigureTypeNameReverses)
}

func apiEnumsBookingPermissionLevelNames(c *fiber.Ctx) (err error) {
	return c.JSON(db.BookingPermissionLevelNameReverses)
}

func apiEnumsBookingStatusNames(c *fiber.Ctx) (err error) {
	return c.JSON(db.BookingStatusNameReverses)
}

func apiEnumsBookingRequestStatusNames(c *fiber.Ctx) (err error) {
	return c.JSON(db.BookingRequestStatusNameReverses)
}

// Hosts API

func apiHostsAll(c *fiber.Ctx) (err error) {
	var hostList []*db.Host = make([]*db.Host, 0)

	if hostList, err = db.Hosts.SelectAll(); err == nil {
		err = c.JSON(hostList)
	}

	return
}

func apiHostByManagementIP(c *fiber.Ctx) (err error) {
	var (
		hostID string = c.Params("management_ip")
		host   *db.Host
	)

	if host, err = db.Hosts.Select(hostID); err == nil {
		err = c.JSON(host)
	}

	return
}

func apiHostCreate(c *fiber.Ctx) (err error) {
	var (
		newHost *db.Host
		body    struct {
			ManagementIP   string            `json:"management_ip"`
			ManagementType db.ManagementType `json:"management_type"`
		}
	)

	if err = c.BodyParser(&body); err != nil {
		return
	}

	if existingHost, _ := db.Hosts.Select(body.ManagementIP); existingHost != nil {
		err = fiber.NewError(fiber.StatusConflict, "host with the same management IP already exists")
		return c.SendStatus(409)
	}

	newHost = &db.Host{
		ManagementIP:   body.ManagementIP,
		ManagementType: body.ManagementType,
	}

	if newHost.Management, err = db.NewHostManagementClient(newHost); err != nil {
		err = fiber.NewError(fiber.StatusInternalServerError, "failed to create management client: "+err.Error())
		log.Errorf("failed to create management client for host %s: %v", newHost.ManagementIP, err)
		return c.SendStatus(500)
	} else {
		defer newHost.Management.Close()
	}

	if err = newHost.Management.UpdateSystemInfo(); err != nil {
		return
	}

	if newHost.LastKnownPowerState, err = newHost.Management.PowerState(true); err != nil {
		return
	}

	newHost.LastKnownPowerStateTime = time.Now()

	if err = db.Hosts.Insert(newHost); err == nil {
		err = c.JSON(newHost)
	}

	return
}

func apiHostDelete(c *fiber.Ctx) (err error) {
	var (
		hostID string = c.Params("management_ip")
	)

	err = db.Hosts.Delete(hostID)
	return
}

func apiHostReprobeSystemInfo(c *fiber.Ctx) (err error) {
	hostID := c.Params("management_ip")
	host, err := db.Hosts.Select(hostID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to retrieve host"})
	}
	if host == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "host not found"})
	}

	managementOwned := false
	if host.Management == nil {
		if host.Management, err = db.NewHostManagementClient(host); err != nil {
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"message": "failed to create management client"})
		}
		managementOwned = true
	}
	if managementOwned {
		defer func() {
			host.Management.Close()
			host.Management = nil
		}()
	}

	if err = host.Management.UpdateSystemInfo(); err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"message": "failed to update host system info"})
	}

	if host.LastKnownPowerState, err = host.Management.PowerState(true); err == nil {
		host.LastKnownPowerStateTime = time.Now()
	}

	if err = db.Hosts.Update(host); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to save host updates"})
	}

	return c.JSON(fiber.Map{
		"message": "host system info refreshed",
		"host":    host,
	})
}

func apiHostPowerControl(c *fiber.Ctx) (err error) {
	var (
		user           *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		hostID         string         = c.Params("management_ip")
		powerActionStr string         = c.Params("action")
		powerActionInt int64
		powerAction    db.PowerAction
		host           *db.Host
	)

	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	sendPowerError := func(status int, msg string, logErr error) error {
		if logErr != nil {
			log.Errorf("power control error for host %s: %v", hostID, logErr)
		}
		return c.Status(status).JSON(fiber.Map{"message": msg})
	}

	if host, err = db.Hosts.Select(hostID); err != nil {
		return sendPowerError(fiber.StatusInternalServerError, "Failed to retrieve host", err)
	} else if host == nil {
		return sendPowerError(fiber.StatusNotFound, "Host not found", nil)
	}

	var canControl bool
	if canControl, err = userCanControlHost(user, host); err != nil {
		return sendPowerError(fiber.StatusInternalServerError, "Failed to authorize power action", err)
	} else if !canControl {
		return sendPowerError(fiber.StatusForbidden, "You are not allowed to control this host", nil)
	}

	if powerActionInt, err = strconv.ParseInt(powerActionStr, 0, 16); err != nil {
		return sendPowerError(fiber.StatusBadRequest, "Invalid power action", err)
	}

	powerAction = db.PowerAction(powerActionInt)

	switch powerAction {
	case db.PowerActionPowerOn:
	case db.PowerActionGracefulShutdown:
	case db.PowerActionPowerOff:
	case db.PowerActionGracefulRestart:
	case db.PowerActionForceRestart:
	default:
		return sendPowerError(fiber.StatusBadRequest, "Unsupported power action", nil)
	}

	operation := asyncOperationCreate(user.Username, asyncOperationKindHostPower, fmt.Sprintf("Power action queued for host %s", hostID), map[string]any{
		"host_management_ip": hostID,
		"action":             powerAction.String(),
		"action_value":       int(powerAction),
	})

	if existingOperationID, busy := reserveHostPowerOperation(hostID, operation.ID); busy {
		asyncOperationDelete(operation.ID)
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"message":      "A power action is already running for this host",
			"operation_id": existingOperationID,
		})
	}

	go runHostPowerOperation(operation.ID, hostID, powerAction)

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"message":   "Power action queued",
		"operation": operation,
	})
}

// ISO Images API

func apiISOImagesCreate(c *fiber.Ctx) (err error) {
	var (
		fileHeader *multipart.FileHeader
	)
	started := time.Now()

	if fileHeader, err = c.FormFile("iso_image"); err != nil {
		log.Errorf("iso upload missing form file: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Missing ISO file in form field iso_image"})
	}
	log.Infof("iso upload received filename=%s size_bytes=%d remote=%s", fileHeader.Filename, fileHeader.Size, c.IP())

	// Save file to temp location (remove temp file after function completes)
	var tempFilePath string = fmt.Sprintf("%s/%s", os.TempDir(), filepath.Base(fileHeader.Filename))
	defer func() {
		if remErr := os.Remove(tempFilePath); remErr != nil && !os.IsNotExist(remErr) {
			log.Warnf("iso upload temp cleanup failed path=%s error=%v", tempFilePath, remErr)
		}
	}()

	if err = c.SaveFile(fileHeader, tempFilePath); err != nil {
		log.Errorf("iso upload save temp failed filename=%s path=%s error=%v", fileHeader.Filename, tempFilePath, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Failed to save uploaded ISO"})
	}
	log.Infof("iso upload temp file saved filename=%s path=%s", fileHeader.Filename, tempFilePath)

	// Extract ISO
	var isoFS *db.StoredISOImage
	if isoFS, err = iso.ExtractISO(tempFilePath, config.Config.ISOs.StorageDir); err != nil {
		log.Errorf("iso upload extract failed filename=%s temp=%s storage_dir=%s error=%v", fileHeader.Filename, tempFilePath, config.Config.ISOs.StorageDir, err)
		status := fiber.StatusInternalServerError
		message := "Failed to process ISO image"
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "could not find kernel") ||
			strings.Contains(lower, "could not find initrd") ||
			strings.Contains(lower, "no udf-capable lister produced results") ||
			strings.Contains(lower, "path not found in iso image") {
			status = fiber.StatusUnprocessableEntity
			message = err.Error()
		}
		return c.Status(status).JSON(fiber.Map{"message": message})
	}
	log.Infof(
		"iso upload extract success filename=%s name=%s distro=%s version=%s arch=%s kernel=%s initrd=%s",
		fileHeader.Filename,
		isoFS.Name,
		isoFS.DistroName,
		isoFS.Version,
		isoFS.Architecture,
		isoFS.KernelPath,
		isoFS.InitrdPath,
	)

	if err = db.StoredISOImages.Insert(isoFS); err != nil {
		log.Errorf("iso upload db insert failed filename=%s iso_name=%s error=%v", fileHeader.Filename, isoFS.Name, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Failed to store ISO metadata"})
	}
	log.Infof("iso upload completed filename=%s iso_name=%s duration=%s", fileHeader.Filename, isoFS.Name, time.Since(started))

	return c.JSON(isoFS)
}

func apiISOImagesDelete(c *fiber.Ctx) (err error) {
	var isoName string = strings.TrimSpace(c.Params("iso_name"))
	if isoName == "" {
		isoName = strings.TrimSpace(c.Query("name"))
	}
	if isoName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "iso name is required"})
	}

	var report *iso.PurgeReport
	if report, err = iso.PurgeStoredISOByName(isoName); err != nil {
		if errors.Is(err, iso.ErrStoredISONotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "ISO not found"})
		}

		log.Errorf("iso purge failed iso_name=%s error=%v", isoName, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Failed to purge ISO artifacts"})
	}

	log.Infof(
		"iso purge complete iso_name=%s removed_paths=%d updated_profiles=%d",
		report.ISOName,
		len(report.RemovedPaths),
		len(report.UpdatedPXEProfileIPs),
	)
	return c.JSON(fiber.Map{
		"message":               "ISO purged successfully",
		"iso_name":              report.ISOName,
		"removed_paths":         report.RemovedPaths,
		"updated_profile_hosts": report.UpdatedPXEProfileIPs,
	})
}

func apiISOImagesList(c *fiber.Ctx) (err error) {
	var isoList []*db.StoredISOImage

	if isoList, err = db.StoredISOImages.SelectAll(); err != nil {
		return
	}

	return c.JSON(isoList)
}

// Booking API

func apiBookingCreate(c *fiber.Ctx) (err error) {
	var (
		user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		body apiBookingCreateBody
	)

	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	if err = c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid body"})
	}
	// Subnets are system-allocated; ignore client-specified CIDR on direct booking create.
	body.CIDRBlock = ""
	body.Description = strings.TrimSpace(body.Description)
	if body.Duration, err = normalizeBookingDuration(body.Duration); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}

	booking, _, err := createBookingForOwner(user.Username, body)
	if err != nil {
		if message, ok := inputValidationMessage(err); ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": message})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to create booking"})
	}

	return c.JSON(booking)
}

func apiBookingList(c *fiber.Ctx) (err error) {
	var (
		bookings []*db.Booking
		user     *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	)

	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	if bookings, err = db.BookingList(); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	return c.JSON(bookings)
}

func buildBookingView(booking *db.Booking, permission db.BookingPermissionLevel) (view *apiBookingView, err error) {
	view = &apiBookingView{
		Booking:         booking,
		PermissionLevel: permission,
		Credentials:     provisioningCredentialsFromConfig(),
	}

	if snapshot, ok := provisioningSnapshot(booking.ID); ok {
		view.Provisioning = buildBookingProvisioningSummary(snapshot)
		if view.Provisioning != nil && permission < db.BookingPermissionLevelOwner {
			view.Provisioning.Cancelable = false
		}
	}

	for _, ip := range booking.OwnedHostManagementIPs {
		var host *db.Host
		if host, err = db.Hosts.Select(ip); err != nil {
			return nil, err
		}

		if host == nil {
			continue
		}

		view.Hosts = append(view.Hosts, host)
	}

	sort.Slice(view.Hosts, func(i, j int) bool { return view.Hosts[i].ManagementIP < view.Hosts[j].ManagementIP })
	return
}

func parseDeployBootMode(mode string) (bootMode db.BootMode, err error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "uefi":
		bootMode = db.BootModeUEFI
	case "legacy":
		bootMode = db.BootModeLegacy
	default:
		err = fmt.Errorf("invalid boot mode, expected UEFI or Legacy")
	}

	return
}

func apiBookingMyList(c *fiber.Ctx) (err error) {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	var (
		views []*apiBookingView
	)

	if user.Permissions() >= auth.AuthPermsAdministrator {
		var bookings []*db.Booking
		if bookings, err = db.BookingList(); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to load bookings"})
		}

		for _, booking := range bookings {
			if booking == nil {
				continue
			}

			var view *apiBookingView
			if view, err = buildBookingView(booking, db.BookingPermissionLevelOwner); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to load booking hosts"})
			}

			views = append(views, view)
		}
	} else {
		var people []*db.BookingPerson
		if people, err = db.BookingPersonsFor(user.Username); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to load your booking memberships"})
		}

		permissionByBooking := map[int]db.BookingPermissionLevel{}
		for _, person := range people {
			if person == nil {
				continue
			}

			level := permissionByBooking[person.BookingID]
			if person.PermissionLevel > level {
				permissionByBooking[person.BookingID] = person.PermissionLevel
			}
		}

		for bookingID, level := range permissionByBooking {
			var booking *db.Booking
			if booking, err = db.BookingByID(bookingID); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to load booking"})
			}

			if booking == nil {
				continue
			}

			var view *apiBookingView
			if view, err = buildBookingView(booking, level); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to load booking hosts"})
			}

			views = append(views, view)
		}
	}

	sort.Slice(views, func(i, j int) bool {
		return views[i].Booking.StartTime.After(views[j].Booking.StartTime)
	})

	return c.JSON(views)
}

func apiBookingNetworkPrefill(c *fiber.Ctx) (err error) {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	defaults, err := loadBookingNetworkDefaults()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}

	suggested, err := suggestNextBookingCIDR(defaults)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}

	_, gateway, err := validateBookingCIDR(defaults, suggested.String(), 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	defaultDurationDays, maxDurationDays := bookingDurationBounds()

	return c.JSON(apiBookingNetworkPrefillView{
		SupernetCIDR:      defaults.Supernet.String(),
		BookingPrefix:     defaults.BookingPrefix,
		HostNetworkPrefix: defaults.HostNetworkPrefix,
		SuggestedCIDR:     suggested.String(),
		GatewayIPv4:       gateway.String(),
		DNSServers:        defaults.DNSServers,
		HostStartOffset:   defaults.HostStartOffset,
		DisableOtherNICs:  defaults.DisableOtherNICs,
		DefaultDuration:   defaultDurationDays,
		MaxDuration:       maxDurationDays,
		ServerDNSName:     bookingServerDNSName(),
	})
}

func apiBookingCartSetNetwork(c *fiber.Ctx) (err error) {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	var body struct {
		CIDRBlock string `json:"cidr_block"`
	}
	if err = c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid body"})
	}

	defaults, err := loadBookingNetworkDefaults()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}

	if strings.TrimSpace(body.CIDRBlock) != "" {
		appLog.Warningf("manual cart subnet selection ignored for user=%s requested=%s", user.Username, strings.TrimSpace(body.CIDRBlock))
	}

	cidr, gateway, err := ensureCartNetworkForUser(user.Username, defaults)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}

	return c.JSON(fiber.Map{
		"network_cidr": cidr.String(),
		"gateway_ipv4": gateway.String(),
		"dns_servers":  defaults.DNSServers,
	})
}

func apiBookingByID(c *fiber.Ctx) (err error) {
	var (
		user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	)

	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	bookingID, err := parseBookingIDParam(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}

	var (
		allowed bool
		level   db.BookingPermissionLevel
		booking *db.Booking
	)

	if allowed, level, err = userCanViewBooking(user, bookingID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to evaluate booking permissions"})
	} else if !allowed {
		return c.SendStatus(fiber.StatusForbidden)
	}

	if booking, err = db.BookingByID(bookingID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to load booking"})
	} else if booking == nil {
		return c.SendStatus(fiber.StatusNotFound)
	}

	var view *apiBookingView
	if view, err = buildBookingView(booking, level); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to load booking hosts"})
	}

	return c.JSON(view)
}

func apiBookingProvisioningStatus(c *fiber.Ctx) (err error) {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	bookingID, err := parseBookingIDParam(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}

	if allowed, _, permErr := userCanViewBooking(user, bookingID); permErr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to evaluate booking permissions"})
	} else if !allowed {
		return c.SendStatus(fiber.StatusForbidden)
	}

	if snapshot, ok := provisioningSnapshot(bookingID); ok {
		return c.JSON(snapshot)
	}

	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "provisioning state not found"})
}

func apiBookingProvisioningCancel(c *fiber.Ctx) (err error) {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	bookingID, err := parseBookingIDParam(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}

	canDestroy, permErr := userCanDestroyBooking(user, bookingID)
	if permErr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to evaluate booking permissions"})
	}
	if !canDestroy {
		return c.SendStatus(fiber.StatusForbidden)
	}

	snapshot, cancelErr := cancelProvisioning(bookingID, user.Username)
	if cancelErr != nil {
		if errors.Is(cancelErr, errProvisioningNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "provisioning state not found"})
		}
		if errors.Is(cancelErr, errProvisioningNotCancelable) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"message": cancelErr.Error()})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to cancel provisioning"})
	}

	return c.JSON(fiber.Map{
		"message":      "provisioning canceled",
		"provisioning": snapshot,
	})
}

func apiBookingDeploy(c *fiber.Ctx) (err error) {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	var body struct {
		Name         string                   `json:"name"`
		Description  string                   `json:"description"`
		Duration     int                      `json:"duration_days"`
		ForceRestart *bool                    `json:"force_restart"`
		BasicConfig  bookingDeployBasicConfig `json:"basic_config"`
	}

	if err = c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid body"})
	}

	var cart *db.BookingCart
	if cart, err = db.BookingCartSnapshot(user.Username); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "cart is empty"})
	}

	if len(cart.Hosts) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "cart has no hosts"})
	}

	defaults, err := loadBookingNetworkDefaults()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Description = strings.TrimSpace(body.Description)
	if body.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "name is required"})
	}
	if err = validateNoProfanity("name", body.Name); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	if err = validateNoProfanity("description", body.Description); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	if utf8.RuneCountInString(body.Description) < 128 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "description must be at least 128 characters"})
	}
	if err = validateNoProfanity("basic_config.timezone", body.BasicConfig.Timezone); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	if err = validateNoProfanity("basic_config.locale", body.BasicConfig.Locale); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	if err = validateNoProfanity("basic_config.keyboard_layout", body.BasicConfig.KeyboardLayout); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	if err = validateNoProfanity("basic_config.mirror", body.BasicConfig.Mirror); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	if err = validateNoProfanity("basic_config.late_stage_script", body.BasicConfig.LateStageScript); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	for idx, pkg := range body.BasicConfig.Packages {
		if err = validateNoProfanity(fmt.Sprintf("basic_config.packages[%d]", idx), pkg); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
	}
	if body.Duration, err = normalizeBookingDuration(body.Duration); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}

	forceRestart := true
	if body.ForceRestart != nil {
		forceRestart = *body.ForceRestart
	}

	hosts := make([]db.BookingRequestHost, 0, len(cart.Hosts))
	for _, host := range cart.Hosts {
		hosts = append(hosts, host)
	}

	sort.Slice(hosts, func(i, j int) bool { return hosts[i].ManagementIP < hosts[j].ManagementIP })

	cidrPrefix, gatewayAddr, err := ensureCartNetworkForUser(user.Username, defaults)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}

	if hosts, err = validateAndAssignHostIPs(cidrPrefix, gatewayAddr, hosts); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}

	mask := prefixMaskStringFromBits(defaults.HostNetworkPrefix)
	dnsCSV := strings.Join(defaults.DNSServers, ",")
	for i := range hosts {
		hostBootMode, parseErr := parseDeployBootMode(hosts[i].BootMode)
		if parseErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": fmt.Sprintf("invalid boot mode for host %s", hosts[i].ManagementIP)})
		}
		hosts[i].BootMode = hostBootMode.String()
		if err = validateNoProfanity(fmt.Sprintf("host %s hostname", hosts[i].ManagementIP), hosts[i].Hostname); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}

		if hosts[i].TemplateData == nil {
			hosts[i].TemplateData = map[string]string{}
		}

		assignedAddr, addrErr := parseIPv4Address(hosts[i].AssignedIPv4)
		if addrErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": fmt.Sprintf("invalid assigned host ip for %s", hosts[i].ManagementIP)})
		}
		hostNetworkCIDR := netip.PrefixFrom(assignedAddr, defaults.HostNetworkPrefix).Masked().String()

		hosts[i].TemplateData["template.network.ipv4"] = hosts[i].AssignedIPv4
		hosts[i].TemplateData["template.network.cidr"] = hostNetworkCIDR
		hosts[i].TemplateData["template.network.netmask"] = mask
		hosts[i].TemplateData["template.network.gateway"] = gatewayAddr.String()
		hosts[i].TemplateData["template.network.disable_other_nics"] = strconv.FormatBool(defaults.DisableOtherNICs)
		if dnsCSV != "" {
			hosts[i].TemplateData["template.network.dns"] = dnsCSV
		}
	}

	bookingBody := apiBookingCreateBody{
		Name:        body.Name,
		Description: body.Description,
		Duration:    body.Duration,
		CIDRBlock:   cidrPrefix.String(),
	}

	booking, ownerRecord, err := createBookingForOwner(user.Username, bookingBody)
	if err != nil {
		if message, ok := inputValidationMessage(err); ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": message})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to create booking"})
	}

	if hosts, err = generateBookingHostnames(hosts, booking.DNSName); err != nil {
		if ownerRecord != nil {
			_ = db.RemoveBookingPerson(ownerRecord.ID)
		}
		_ = db.DeleteBooking(booking.ID)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to generate hostnames"})
	}

	assignedHosts := make([]string, 0, len(hosts))
	for _, host := range hosts {
		if err = db.AssignHostToBooking(booking.ID, host.ManagementIP); err != nil {
			for _, ip := range assignedHosts {
				_ = db.ReleaseHostFromBooking(booking.ID, ip)
			}

			if ownerRecord != nil {
				_ = db.RemoveBookingPerson(ownerRecord.ID)
			}
			_ = db.DeleteBooking(booking.ID)

			status := fiber.StatusInternalServerError
			if errors.Is(err, db.ErrHostAlreadyBooked) {
				status = fiber.StatusConflict
			}
			return c.Status(status).JSON(fiber.Map{"message": fmt.Sprintf("failed to reserve host %s: %v", host.ManagementIP, err)})
		}

		var dbHost *db.Host
		if dbHost, err = db.Hosts.Select(host.ManagementIP); err != nil || dbHost == nil {
			for _, ip := range assignedHosts {
				_ = db.ReleaseHostFromBooking(booking.ID, ip)
			}
			if ownerRecord != nil {
				_ = db.RemoveBookingPerson(ownerRecord.ID)
			}
			_ = db.DeleteBooking(booking.ID)
			if err == nil {
				err = fmt.Errorf("host %s not found after reservation", host.ManagementIP)
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
		}

		dbHost.AssignedIPv4 = host.AssignedIPv4
		dbHost.AssignedCIDR = cidrPrefix.String()
		if err = db.Hosts.Update(dbHost); err != nil {
			for _, ip := range assignedHosts {
				_ = db.ReleaseHostFromBooking(booking.ID, ip)
			}
			_ = db.ReleaseHostFromBooking(booking.ID, host.ManagementIP)
			if ownerRecord != nil {
				_ = db.RemoveBookingPerson(ownerRecord.ID)
			}
			_ = db.DeleteBooking(booking.ID)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": fmt.Sprintf("failed to persist host network for %s: %v", host.ManagementIP, err)})
		}

		assignedHosts = append(assignedHosts, host.ManagementIP)
	}

	db.ResetBookingCart(user.Username)

	provisioning := createProvisioningJob(booking.ID, user.Username, hosts)
	startProvisioningWorkflow(booking.ID, user.Username, hosts, body.BasicConfig, forceRestart)

	var view *apiBookingView
	if view, err = buildBookingView(booking, db.BookingPermissionLevelOwner); err != nil {
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
			"booking_id":    booking.ID,
			"provisioning":  provisioning,
			"warning":       "deployment started but booking view could not be fully loaded",
			"warning_error": err.Error(),
		})
	}

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"booking":      view,
		"provisioning": provisioning,
	})
}

func apiBookingDestroy(c *fiber.Ctx) (err error) {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	bookingID, err := parseBookingIDParam(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}

	canDestroy, err := userCanDestroyBooking(user, bookingID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to evaluate booking permissions"})
	}
	if !canDestroy {
		return c.SendStatus(fiber.StatusForbidden)
	}

	booking, err := db.BookingByID(bookingID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to load booking"})
	}
	if booking == nil {
		return c.SendStatus(fiber.StatusNotFound)
	}

	var releaseErrors []string
	for _, ip := range booking.OwnedHostManagementIPs {
		host, hostErr := db.Hosts.Select(ip)
		if hostErr != nil {
			releaseErrors = append(releaseErrors, fmt.Sprintf("host %s lookup failed: %v", ip, hostErr))
			continue
		}

		if host != nil {
			if clearErr := pxe.ClearHostProfileOverride(host); clearErr != nil {
				releaseErrors = append(releaseErrors, fmt.Sprintf("host %s PXE override clear failed: %v", ip, clearErr))
			}
		}

		if releaseErr := db.ReleaseHostFromBooking(bookingID, ip); releaseErr != nil {
			releaseErrors = append(releaseErrors, fmt.Sprintf("host %s release failed: %v", ip, releaseErr))
		}
	}

	provisioningSetStatus(bookingID, provisioningStatusDestroyed)
	provisioningAppendEvent(bookingID, "warn", "Booking destroyed by user action")
	provisioningDeleteJob(bookingID)

	if err = db.DeleteBookingCascade(bookingID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to delete booking"})
	}

	response := fiber.Map{
		"message": "booking destroyed and hosts released",
	}

	if len(releaseErrors) > 0 {
		appLog.Warningf("booking %d destroy completed with release warnings: %v\n", bookingID, releaseErrors)
		response["release_warnings"] = releaseErrors
	}

	return c.JSON(response)
}

func apiBookingCartSnapshot(c *fiber.Ctx) (err error) {
	var (
		user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		cart *db.BookingCart
	)

	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	if cart, err = db.BookingCartSnapshot(user.Username); err != nil {
		if errors.Is(err, db.ErrCartNotFound) {
			return c.JSON(&db.BookingCart{
				Owner:       user.Username,
				Hosts:       map[string]db.BookingRequestHost{},
				NetworkCIDR: "",
				GatewayIPv4: "",
				DNSServers:  []string{},
				UpdatedAt:   time.Now(),
			})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to load cart"})
	}

	return c.JSON(cart)
}

func apiBookingCartCounts(c *fiber.Ctx) (err error) {
	var (
		user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	)

	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	if hosts, virtual, errCount := db.CartCounts(user.Username); errCount != nil {
		if errors.Is(errCount, db.ErrCartNotFound) {
			return c.JSON(fiber.Map{
				"hosts":    0,
				"virtual":  0,
				"total":    0,
				"has_cart": false,
			})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to load cart counts"})
	} else {
		return c.JSON(fiber.Map{
			"hosts":    hosts,
			"virtual":  virtual,
			"total":    hosts + virtual,
			"has_cart": true,
		})
	}
}

func apiBookingCartAddHost(c *fiber.Ctx) (err error) {
	var (
		user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		body apiBookingCartHostUpsertBody
	)

	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	if err = c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid body"})
	}

	body.ManagementIP = strings.TrimSpace(body.ManagementIP)
	body.ISOSelection = strings.TrimSpace(body.ISOSelection)
	body.BootMode = strings.TrimSpace(body.BootMode)
	body.Hostname = strings.TrimSpace(body.Hostname)
	body.AssignedIPv4 = strings.TrimSpace(body.AssignedIPv4)
	if len(body.ManagementIP) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "management_ip is required"})
	}
	if body.AssignedHostBits != nil || body.AssignedIPv4 != "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "manual host IP assignment is disabled"})
	}

	parsedBootMode, parseBootModeErr := parseDeployBootMode(body.BootMode)
	if parseBootModeErr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": parseBootModeErr.Error()})
	}

	if err = validateNoProfanity("hostname", body.Hostname); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	for key, value := range body.TemplateData {
		if strings.EqualFold(strings.TrimSpace(key), "template.late_script") {
			continue
		}
		if err = validateNoProfanity("template_data."+strings.TrimSpace(key), value); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
	}

	hostnameLabel := sanitizeHostnameLabel(body.Hostname)
	if body.Hostname != "" && hostnameLabel == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "hostname must contain letters or numbers"})
	}
	if hostnameLabel == "" {
		if cart, cartErr := db.BookingCartSnapshot(user.Username); cartErr == nil && cart != nil {
			if existing, exists := cart.Hosts[body.ManagementIP]; exists {
				hostnameLabel = sanitizeHostnameLabel(existing.Hostname)
			}
		}
	}
	if hostnameLabel == "" {
		if hostnameLabel, err = randomBookingHostnameWord(); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to generate hostname label"})
		}
	}

	host := db.BookingRequestHost{
		ManagementIP: body.ManagementIP,
		ISOSelection: body.ISOSelection,
		BootMode:     parsedBootMode.String(),
		Hostname:     hostnameLabel,
		AssignedIPv4: "",
		TemplateData: body.TemplateData,
	}

	if err = db.AddHostToCart(user.Username, host); err != nil {
		status := fiber.StatusBadRequest
		if errors.Is(err, db.ErrHostAlreadyBooked) {
			status = fiber.StatusConflict
		} else if errors.Is(err, db.ErrCartNotFound) {
			status = fiber.StatusNotFound
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}

	return c.SendStatus(fiber.StatusOK)
}

func apiBookingCartRemoveHost(c *fiber.Ctx) (err error) {
	var (
		user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	)

	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	db.RemoveHostFromCart(user.Username, c.Params("management_ip"))
	return c.SendStatus(fiber.StatusOK)
}

func apiBookingCartAvailableHosts(c *fiber.Ctx) (err error) {
	var (
		user  *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		hosts []*db.Host
	)

	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	if hosts, err = db.AvailableHostsForCart(user.Username); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	return c.JSON(hosts)
}

func apiBookingCreateRequest(c *fiber.Ctx) (err error) {
	var (
		user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		body struct {
			Justification string                `json:"justification"`
			Containers    []db.BookingRequestCT `json:"containers"`
			VMs           []db.BookingRequestVM `json:"vms"`
		}
		bookingID int
		request   *db.BookingRequest
	)

	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	if bookingID64, errConv := strconv.ParseInt(c.Params("booking_id"), 10, 32); errConv != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid booking id"})
	} else {
		bookingID = int(bookingID64)
	}

	if allowed, _, permErr := userCanViewBooking(user, bookingID); permErr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to evaluate booking permissions"})
	} else if !allowed {
		return c.SendStatus(fiber.StatusForbidden)
	}

	if err = c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid body"})
	}
	body.Justification = strings.TrimSpace(body.Justification)
	if err = validateNoProfanity("justification", body.Justification); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	for i := range body.Containers {
		body.Containers[i].Name = strings.TrimSpace(body.Containers[i].Name)
		body.Containers[i].Template = strings.TrimSpace(body.Containers[i].Template)
		if err = validateNoProfanity(fmt.Sprintf("containers[%d].name", i), body.Containers[i].Name); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
		if err = validateNoProfanity(fmt.Sprintf("containers[%d].template", i), body.Containers[i].Template); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
	}
	for i := range body.VMs {
		body.VMs[i].Name = strings.TrimSpace(body.VMs[i].Name)
		body.VMs[i].ISOSelection = strings.TrimSpace(body.VMs[i].ISOSelection)
		if err = validateNoProfanity(fmt.Sprintf("vms[%d].name", i), body.VMs[i].Name); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
		if err = validateNoProfanity(fmt.Sprintf("vms[%d].iso_selection", i), body.VMs[i].ISOSelection); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
	}

	if request, err = db.BuildBookingRequestFromCart(user.Username, bookingID, body.Justification, user.Username, body.Containers, body.VMs); err != nil {
		status := fiber.StatusInternalServerError
		if errors.Is(err, db.ErrCartNotFound) {
			status = fiber.StatusNotFound
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}

	if err = db.AddBookingRequest(request); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to add booking request"})
	}

	db.ResetBookingCart(user.Username)
	return c.JSON(request)
}
