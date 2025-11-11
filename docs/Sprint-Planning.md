# OpnLaaS Sprint Planning

## Sprint Goal

**Goal:** Enable students and faculty to manage resources through a web interface, and create virtual machines on proxmox.

**Specific Goal:** Create a resource management dashboard in the web UI, and allow users to interface with proxmox so they are able to create/manage VMs.

**Stretch Goal:** Implement logging to keep track of changes to virtual machines.

## Selected Stories

-   Feature 1: User & Resource Dashboard
    -   As a System Architect, I want to be able to view existing resources, so that I can make sure systems are running correctly.
    -   As a System Architect, I want to be able to add new resources so that I am able to update the system with new equipment.
-   Feature 2: Virtualization Integration
    -   As a Researcher, I want to create new virtual machines, so that I can run my projects.
    -   As a Researcher, I want to edit my existing virtual machines, so that I can make changes to my projects as I go.

## Story Decomposition

### Feature 1: User & Resource Dashboard

**Story 1:** - As a System Architect, I want to be able to view existing resources, so that I can make sure systems are running correctly.

Tasks:

-   Data Model (Host Resource):

```go
Host struct {
	ManagementIP        string                `gomysql:"management_ip,primary,unique" json:"management_ip"`
	Vendor              VendorID              `gomysql:"vendor" json:"vendor"`
	FormFactor          FormFactor            `gomysql:"form_factor" json:"form_factor"`
	ManagementType      ManagementType        `gomysql:"management_type" json:"management_type"`
	Model               string                `gomysql:"model" json:"model"`
	LastKnownPowerState PowerState            `gomysql:"last_known_power_state" json:"last_known_power_state"`
	Specs               HostSpecs             `gomysql:"specs" json:"specs"`
	Management          *HostManagementClient `json:"-"`
}
```

-   Frontend:
    -   Dashboard needs to get current resources from API and display them to users.
    -   UI should include details of each system, and wether the system is running or not.
    -   app routes:
        -   GET: /dashboard
-   Backend:
    -   Needs to get current resources from DB, and make that available to the frontend.
    -   Should include all hosts currently in use.
    -   api routes:
        -   GET: /api/hosts
-   Integration:
    -   Test should be created for the frontend to make sure resources are displayed correctly.
    -   Tests for the backend should ensure the API properly authenticates users, and prevents possible SQL injections.

**Story 2:** - As a System Architect, I want to be able to add new resources so that I am able to update the system with new equipment.

Tasks:

-   Data Model (Host Resource):

    _(Same as Story 1)_

-   Frontend:
    -   Dashboard should include an method to add a new host/resource.
    -   This could be either a separate page or a modal on the dashboard.
    -   app routes:
        -   POST: /dashboard/create
-   Backend:
    -   Needs to add new hosts to the database.
    -   Should authenticate users before allowing them to add new host resources.
    -   api routes:
        -   POST: /api/hosts
-   Integration:
    -   Testing should be done to ensure backend has proper error handling for bad inputs. (especially since there are many fields, some of which optional for each host)

### Feature 2: Virtualization Integration

**Story 1:** - As a Researcher, I want to create new virtual machines, so that I can run my projects.

Tasks:

-   Data Model (VM/ISO):

```go
StoredISOImage struct {
	Name         string           `gomysql:"name,primary,unique" json:"name"`
	DistroName   string           `gomysql:"distro_name" json:"distro_name"`
	Version      string           `gomysql:"version" json:"version"`
	Size         int64            `gomysql:"size" json:"size"`
	FullISOPath  string           `gomysql:"full_iso_path" json:"full_iso_path"`
	KernelPath   string           `gomysql:"kernel_path" json:"kernel_path"`
	InitrdPath   string           `gomysql:"initrd_path" json:"initrd_path"`
	Architecture Architecture     `gomysql:"architecture" json:"architecture"`
	DistroType   DistroType       `gomysql:"distro_type" json:"distro_type"`
	PreConfigure PreConfigureType `gomysql:"preconfigure_type" json:"preconfigure_type"`
}
```

-   Frontend:
    -   Users should have an easy way of selecting an operating system image (ISO) and creating a new VM from it.
    -
-   Backend:
    -   d
-   Integration:
    -   New VMs should automatically be deployed and visible from the dashboard.
    -

**Story 2:** - As a Researcher, I want to edit my existing virtual machines, so that I can make changes to my projects as I go.

Tasks:

-   Data Model (VM/ISO):

    _(Same as Story 1)_

-   Frontend:
    -   d
-   Backend:
    -   d
-   Integration:
    -   d

## Data Models & API Spec

Data models where included with each story as needed. Below is a overview of the API/structure of our application:

```go
// Pages
app.Static("/static", "./public/static")

app.Get("/", showLanding)
app.Get("/login", showLogin)
app.Get("/logout", mustBeLoggedIn, showLogout)
app.Get("/dashboard", showDashboard)

// Auth API
app.Post("/api/auth/login", apiLogin)
app.Post("/api/auth/logout", mustBeLoggedIn, apiLogout)

// Enums API
app.Get("/api/enums/vendors", apiEnumsVendorNames)
app.Get("/api/enums/form-factors", apiEnumsFormFactorNames)
app.Get("/api/enums/management-types", apiEnumsManagementTypeNames)
app.Get("/api/enums/power-states", apiEnumsPowerStateNames)
app.Get("/api/enums/boot-modes", apiEnumsBootModeNames)
app.Get("/api/enums/power-actions", apiEnumsPowerActionNames)
app.Get("/api/enums/architectures", apiEnumsArchitectureNames)
app.Get("/api/enums/distro-types", apiEnumsDistroTypeNames)
app.Get("/api/enums/preconfigure-types", apiEnumsPreConfigureTypeNames)

// Hosts API
app.Get("/api/hosts", apiHostsAll)
app.Get("/api/hosts/:management_ip", apiHostByManagementIP)
app.Post("/api/hosts", mustBeLoggedIn, mustBeAdmin, apiHostCreate)
app.Delete("/api/hosts/:management_ip", mustBeLoggedIn, mustBeAdmin, apiHostDelete)
app.Post("/api/hosts/:management_ip/power/:action", mustBeLoggedIn, mustBeAdmin, apiHostPowerControl)

// ISO Images API
app.Post("/api/iso-images", mustBeLoggedIn, mustBeAdmin, apiISOImagesCreate)
app.Get("/api/iso-images", mustBeLoggedIn, mustBeAdmin, apiISOImagesList)
```

## Responsibilities

-   Feature 1: User & Resource Dashboard
    -   Dan McCarthy
    -   Kestutis Biskis
-   Feature 2: Virtualization Integration
    -   Evan Parker
    -   Matt Gee
    -   Alex Houle

## Gitlab

Below is the link to our issue board. Our board has been populated with issues and everyone has been assigned atleast one task. (Note: our project is on Github)

https://github.com/orgs/OpnLaaS/projects/1
