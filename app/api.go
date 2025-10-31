package app

import (
	"github.com/gofiber/fiber/v2"
	"github.com/opnlaas/opnlaas/auth"
	"github.com/opnlaas/opnlaas/hosts"
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

	return c.Render("login", fiber.Map{
		"Title":      "Login",
		"LoginError": err.Error(),
	}, "layout")
}

func apiLogout(c *fiber.Ctx) (err error) {
	c.ClearCookie("Authorization")
	return
}

// Enums API

func apiEnumsVendorNames(c *fiber.Ctx) (err error) {
	return c.JSON(hosts.VendorNameReverses)
}

func apiEnumsFormFactorNames(c *fiber.Ctx) (err error) {
	return c.JSON(hosts.FormFactorNameReverses)
}

func apiEnumsManagementTypeNames(c *fiber.Ctx) (err error) {
	return c.JSON(hosts.ManagementTypeNameReverses)
}

func apiEnumsPowerStateNames(c *fiber.Ctx) (err error) {
	return c.JSON(hosts.PowerStateNameReverses)
}

func apiEnumsBootModeNames(c *fiber.Ctx) (err error) {
	return c.JSON(hosts.BootModeNameReverses)
}

// Hosts API

func apiHostsAll(c *fiber.Ctx) (err error) {
	var hostList []*hosts.Host = make([]*hosts.Host, 0)

	if hostList, err = hosts.Hosts.SelectAll(); err == nil {
		err = c.JSON(hostList)
	}

	return
}

func apiHostByManagementIP(c *fiber.Ctx) (err error) {
	var (
		hostID string = c.Params("management_ip")
		host   *hosts.Host
	)

	if host, err = hosts.Hosts.Select(hostID); err == nil {
		err = c.JSON(host)
	}

	return
}

func apiHostCreate(c *fiber.Ctx) (err error) {
	var (
		newHost *hosts.Host
		body    struct {
			ManagementIP   string               `json:"management_ip"`
			ManagementType hosts.ManagementType `json:"management_type"`
		}
	)

	if err = c.BodyParser(&body); err != nil {
		return
	}

	if existingHost, _ := hosts.Hosts.Select(body.ManagementIP); existingHost != nil {
		err = fiber.NewError(fiber.StatusConflict, "host with the same management IP already exists")
		return
	}

	newHost = &hosts.Host{
		ManagementIP:   body.ManagementIP,
		ManagementType: body.ManagementType,
	}

	if newHost.Management, err = hosts.NewHostManagementClient(newHost); err != nil {
		return
	}

	if err = newHost.Management.UpdateSystemInfo(); err != nil {
		return
	}

	if err = hosts.Hosts.Insert(newHost); err == nil {
		err = c.JSON(newHost)
	}

	return
}

func apiHostDelete(c *fiber.Ctx) (err error) {
	var (
		hostID string = c.Params("management_ip")
	)

	err = hosts.Hosts.Delete(hostID)
	return
}

func apiHostPowerControl(c *fiber.Ctx) (err error) {
	var (
		hostID         string = c.Params("management_ip")
		powerActionStr string = c.Params("action")
		powerAction    hosts.PowerAction
		ok             bool
		host           *hosts.Host
	)

	if host, err = hosts.Hosts.Select(hostID); err != nil {
		err = fiber.NewError(fiber.StatusInternalServerError, "failed to retrieve host")
		return
	} else if host == nil {
		err = fiber.NewError(fiber.StatusNotFound, "host not found")
		return
	}

	if powerAction, ok = hosts.PowerActionNameReverses[powerActionStr]; !ok {
		err = fiber.NewError(fiber.StatusBadRequest, "invalid power action")
		return
	}

	if host.Management == nil {
		if host.Management, err = hosts.NewHostManagementClient(host); err != nil {
			return
		}
	}

	switch powerAction {
	case hosts.PowerActionPowerOn:
		err = host.Management.SetPowerState(hosts.PowerStateOn, false)
	case hosts.PowerActionGracefulShutdown:
		err = host.Management.SetPowerState(hosts.PowerStateOff, false)
	case hosts.PowerActionPowerOff:
		err = host.Management.SetPowerState(hosts.PowerStateOff, true)
	case hosts.PowerActionGracefulRestart:
		err = host.Management.ResetPowerState(false)
	case hosts.PowerActionForceRestart:
		err = host.Management.ResetPowerState(true)
	default:
		err = fiber.NewError(fiber.StatusBadRequest, "unsupported power action")
	}

	return
}
