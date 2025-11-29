package app

import (
	"errors"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/opnlaas/opnlaas/auth"
	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/db"
	"github.com/opnlaas/opnlaas/iso"
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
		hostID                string = c.Params("management_ip")
		powerActionStr        string = c.Params("action")
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
		fmt.Println(err)
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

	if fileHeader, err = c.FormFile("iso_image"); err != nil {
		return
	}

	// Save file to temp location
	var tempFilePath string = fmt.Sprintf("%s/%s", os.TempDir(), filepath.Base(fileHeader.Filename))
	if err = c.SaveFile(fileHeader, tempFilePath); err != nil {
		return
	}

	// Extract ISO
	var isoFS *db.StoredISOImage
	if isoFS, err = iso.ExtractISO(tempFilePath, config.Config.ISOs.StorageDir); err != nil {
		return
	}

	if err = db.StoredISOImages.Insert(isoFS); err != nil {
		return
	}

	return c.JSON(isoFS)
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
		body struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Duration    int    `json:"duration_days"`
		}
		newBooking db.Booking
	)

	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	if err = c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid body"})
	}

	if len(body.Name) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "name is required"})
	}

	now := time.Now()
	newBooking = db.Booking{
		Name:        body.Name,
		Description: body.Description,
		Status:      db.BookingStatusActive,
		StartTime:   now,
	}

	if body.Duration > 0 {
		newBooking.EndTime = now.Add(time.Duration(body.Duration) * 24 * time.Hour)
	}

	if err = db.CreateBooking(&newBooking); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to create booking"})
	}

	owner := &db.BookingPerson{
		Username:        user.Username,
		BookingID:       newBooking.ID,
		PermissionLevel: db.BookingPermissionLevelOwner,
	}

	if err = db.AddBookingPerson(owner); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "failed to set booking owner"})
	}

	newBooking.People = append(newBooking.People, owner.ID)

	return c.JSON(newBooking)
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

func apiBookingCartSnapshot(c *fiber.Ctx) (err error) {
	var (
		user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		cart *db.BookingCart
	)

	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	if cart, err = db.BookingCartSnapshot(user.Username); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "cart not found"})
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
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "cart not found"})
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
		body db.BookingRequestHost
	)

	if user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	if err = c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "invalid body"})
	}

	if len(body.ManagementIP) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "management_ip is required"})
	}

	if err = db.AddHostToCart(user.Username, body); err != nil {
		status := fiber.StatusInternalServerError
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
