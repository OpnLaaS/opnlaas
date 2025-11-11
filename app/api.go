package app

import (
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
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

			if c.Query("no_redirect") == "1" {
				return c.SendStatus(fiber.StatusOK)
			}

			return c.Redirect("/dashboard")
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
	auth.Logout(user.LDAPConn.Username)

	c.Cookie(&fiber.Cookie{
		Name:    "Authorization",
		Value:   "",
		Expires: time.Now().Add(-time.Hour),
	})
	return
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
		return c.SendStatus(500)
	} else {
		defer newHost.Management.Close()
	}

	if err = newHost.Management.UpdateSystemInfo(); err != nil {
		return
	}

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
		hostID         string = c.Params("management_ip")
		powerActionStr string = c.Params("action")
		powerAction    db.PowerAction
		// ok             bool
		host *db.Host
	)

	if host, err = db.Hosts.Select(hostID); err != nil {
		err = fiber.NewError(fiber.StatusInternalServerError, "failed to retrieve host")
		return
	} else if host == nil {
		err = fiber.NewError(fiber.StatusNotFound, "host not found")
		return
	}

	powerActionInt, err := strconv.ParseInt(powerActionStr, 0, 16)

	if err != nil {

		return fiber.NewError(fiber.StatusInternalServerError, "bad power action")
	}

	powerAction = db.PowerAction(powerActionInt)

	if host.Management == nil {
		if host.Management, err = db.NewHostManagementClient(host); err != nil {
			return
		} else {
			defer host.Management.Close()
		}
	}

	switch powerAction {
	case db.PowerActionPowerOn:
		err = host.Management.SetPowerState(db.PowerStateOn, false)
	case db.PowerActionGracefulShutdown:
		err = host.Management.SetPowerState(db.PowerStateOff, false)
	case db.PowerActionPowerOff:
		err = host.Management.SetPowerState(db.PowerStateOff, true)
	case db.PowerActionGracefulRestart:
		err = host.Management.ResetPowerState(false)
	case db.PowerActionForceRestart:
		err = host.Management.ResetPowerState(true)
	default:
		err = fiber.NewError(fiber.StatusBadRequest, "unsupported power action")
	}

	return
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
