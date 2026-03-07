package app

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

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
	Booking         *db.Booking                `json:"booking"`
	PermissionLevel db.BookingPermissionLevel  `json:"permission_level"`
	Hosts           []*db.Host                 `json:"hosts"`
	Credentials     apiProvisioningCredentials `json:"credentials"`
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
}

type apiBookingCartHostUpsertBody struct {
	ManagementIP     string            `json:"management_ip"`
	ISOSelection     string            `json:"iso_selection"`
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

func createBookingForOwner(owner string, body apiBookingCreateBody) (booking *db.Booking, ownerRecord *db.BookingPerson, err error) {
	if strings.TrimSpace(body.Name) == "" {
		err = fmt.Errorf("name is required")
		return
	}

	requestedCIDR := strings.TrimSpace(body.CIDRBlock)
	autoAllocateCIDR := requestedCIDR == ""
	if requestedCIDR, err = resolveBookingCIDRForCreate(requestedCIDR); err != nil {
		err = fmt.Errorf("resolve booking cidr: %w", err)
		return
	}

	now := time.Now()
	for attempt := 1; attempt <= 3; attempt++ {
		booking = &db.Booking{
			Name:        strings.TrimSpace(body.Name),
			Description: body.Description,
			Status:      db.BookingStatusActive,
			StartTime:   now,
			CIDRBlock:   requestedCIDR,
		}

		if body.Duration > 0 {
			booking.EndTime = now.Add(time.Duration(body.Duration) * 24 * time.Hour)
		}

		if err = db.CreateBooking(booking); err == nil {
			break
		}

		if !(autoAllocateCIDR && isBookingCIDRUniqueError(err) && attempt < 3) {
			err = fmt.Errorf("create booking: %w", err)
			return
		}

		if requestedCIDR, err = resolveBookingCIDRForCreate(""); err != nil {
			err = fmt.Errorf("resolve booking cidr retry: %w", err)
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

func assignedIPv4FromHostBits(owner string, hostBits int) (ip string, err error) {
	if hostBits <= 0 {
		return "", fmt.Errorf("assigned_host_bits must be greater than zero")
	}

	cart, err := db.BookingCartSnapshot(owner)
	if err != nil {
		if errors.Is(err, db.ErrCartNotFound) {
			return "", fmt.Errorf("booking cart has no network assigned")
		}
		return "", fmt.Errorf("load booking cart: %w", err)
	}

	if strings.TrimSpace(cart.NetworkCIDR) == "" {
		return "", fmt.Errorf("booking cart has no network assigned")
	}

	cidr, err := parseIPv4Prefix(cart.NetworkCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid cart network: %w", err)
	}

	addr, err := hostOffsetToIPv4(cidr, hostBits)
	if err != nil {
		return "", err
	}

	return addr.String(), nil
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

func apiHostPowerControl(c *fiber.Ctx) (err error) {
	var (
		user                  *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		hostID                string         = c.Params("management_ip")
		powerActionStr        string         = c.Params("action")
		powerActionInt        int64
		powerAction           db.PowerAction
		host                  *db.Host
		hostCurrentPowerState db.PowerState
		waitPowerState        db.PowerState
	)

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

	if host.Management == nil {
		if host.Management, err = db.NewHostManagementClient(host); err != nil {
			return sendPowerError(fiber.StatusBadGateway, "Failed to create management client", err)
		} else {
			defer host.Management.Close()
		}
	}

	if hostCurrentPowerState, err = host.Management.PowerState(false); err != nil {
		return sendPowerError(fiber.StatusBadGateway, "Failed to read current power state", err)
	}

	switch powerAction {
	case db.PowerActionPowerOn:
		waitPowerState = db.PowerStateOn
		if hostCurrentPowerState == db.PowerStateOn {
			return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"message": "Host already powered on"})
		}

		if err = host.Management.SetPowerState(db.PowerStateOn, false); err != nil {
			return sendPowerError(fiber.StatusBadGateway, "Failed to power on host", err)
		}
	case db.PowerActionGracefulShutdown:
		waitPowerState = db.PowerStateOff
		if hostCurrentPowerState == db.PowerStateOff {
			return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"message": "Host already powered off"})
		}

		if err = host.Management.SetPowerState(db.PowerStateOff, false); err != nil {
			return sendPowerError(fiber.StatusBadGateway, "Failed to gracefully shut down host", err)
		}
	case db.PowerActionPowerOff:
		waitPowerState = db.PowerStateOff
		if hostCurrentPowerState == db.PowerStateOff {
			return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"message": "Host already powered off"})
		}

		if err = host.Management.SetPowerState(db.PowerStateOff, true); err != nil {
			return sendPowerError(fiber.StatusBadGateway, "Failed to force power off host", err)
		}
	case db.PowerActionGracefulRestart:
		waitPowerState = db.PowerStateOn
		if err = host.Management.ResetPowerState(false); err != nil {
			return sendPowerError(fiber.StatusBadGateway, "Failed to gracefully restart host", err)
		}
	case db.PowerActionForceRestart:
		waitPowerState = db.PowerStateOn
		if err = host.Management.ResetPowerState(true); err != nil {
			return sendPowerError(fiber.StatusBadGateway, "Failed to force restart host", err)
		}
	default:
		return sendPowerError(fiber.StatusBadRequest, "Unsupported power action", nil)
	}

	// Wait for desired power state
	if err = host.Management.WaitSystemPowerState(waitPowerState, 120); err != nil {
		return sendPowerError(fiber.StatusGatewayTimeout, fmt.Sprintf("Timed out waiting for host to reach %s power state", waitPowerState.String()), err)
	}

	// Update host power state in DB
	if host.LastKnownPowerState, err = host.Management.PowerState(true); err != nil {
		log.Warnf("failed to update last known power state for host %s: %v", host.ManagementIP, err)
	} else {
		host.LastKnownPowerStateTime = time.Now()
		if err = db.Hosts.Update(host); err != nil {
			log.Warnf("failed to save updated power state for host %s: %v", host.ManagementIP, err)
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Power action completed successfully", "power_state": host.LastKnownPowerState})
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
	var (
		isoName string = c.Params("iso_name")
	)

	err = db.StoredISOImages.Delete(isoName)
	return
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

	booking, _, err := createBookingForOwner(user.Username, body)
	if err != nil {
		if strings.Contains(err.Error(), "name is required") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "name is required"})
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

	return c.JSON(apiBookingNetworkPrefillView{
		SupernetCIDR:      defaults.Supernet.String(),
		BookingPrefix:     defaults.BookingPrefix,
		HostNetworkPrefix: defaults.HostNetworkPrefix,
		SuggestedCIDR:     suggested.String(),
		GatewayIPv4:       gateway.String(),
		DNSServers:        defaults.DNSServers,
		HostStartOffset:   defaults.HostStartOffset,
		DisableOtherNICs:  defaults.DisableOtherNICs,
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

func apiBookingDeploy(c *fiber.Ctx) (err error) {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	var body struct {
		Name         string                   `json:"name"`
		Description  string                   `json:"description"`
		Duration     int                      `json:"duration_days"`
		BootMode     string                   `json:"boot_mode"`
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

	bootMode, err := parseDeployBootMode(body.BootMode)
	if err != nil {
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

	if hosts, err = validateAndAssignHostIPs(cidrPrefix, gatewayAddr, hosts, defaults.HostStartOffset); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}

	mask := prefixMaskStringFromBits(defaults.HostNetworkPrefix)
	dnsCSV := strings.Join(defaults.DNSServers, ",")
	for i := range hosts {
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
		if strings.Contains(err.Error(), "name is required") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "name is required"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to create booking"})
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
	startProvisioningWorkflow(booking.ID, user.Username, hosts, body.BasicConfig, bootMode, forceRestart)

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

	if err = db.DeleteBookingCascade(bookingID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to delete booking"})
	}

	provisioningAppendEvent(bookingID, "warn", "Booking destroyed by user action")
	provisioningDeleteJob(bookingID)

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
	body.AssignedIPv4 = strings.TrimSpace(body.AssignedIPv4)
	if len(body.ManagementIP) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "management_ip is required"})
	}
	if body.AssignedHostBits == nil && body.AssignedIPv4 != "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "assigned_ipv4 is not allowed; use assigned_host_bits"})
	}

	if body.AssignedHostBits != nil {
		defaults, defaultsErr := loadBookingNetworkDefaults()
		if defaultsErr != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": defaultsErr.Error()})
		}

		if _, _, err = ensureCartNetworkForUser(user.Username, defaults); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}

		if body.AssignedIPv4, err = assignedIPv4FromHostBits(user.Username, *body.AssignedHostBits); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
	}

	host := db.BookingRequestHost{
		ManagementIP: body.ManagementIP,
		ISOSelection: body.ISOSelection,
		AssignedIPv4: body.AssignedIPv4,
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
