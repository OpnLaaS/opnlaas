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
    -   d
    -   api routes:
        -   POST: /api/hosts
-   Integration:
    -   d

### Feature 2: Virtualization Integration

**Story 1:** - As a Researcher, I want to create new virtual machines, so that I can run my projects.

Tasks:

-   Data Model (Host Resource):

-   frontend:
    -   d
-   backend:
    -   d
-   integration:
    -   d

**Story 2:** - As a Researcher, I want to edit my existing virtual machines, so that I can make changes to my projects as I go.

Tasks:

-   Data Model (Host Resource):

-   frontend:
    -   d
-   backend:
    -   d
-   integration:
    -   d

## Data Models & API Spec

Data models where included with each story as needed. Below is a overview of the API of our application:

## Responsibilities

-   Feature 1: User & Resource Dashboard
    -   Dan McCarthy
    -   .
-   Feature 2: Virtualization Integration
    -   .

## Gitlab

Below is the link to our issue board. Our board has been populated with issues and everyone has been assigned atleast one task. (Note: our project is on Github)

https://github.com/orgs/OpnLaaS/projects/1
