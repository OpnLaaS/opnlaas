# OpnLaaS Sprint Planning

## Sprint Goal

**Goal:** Enable students and faculty to manage resources through a web interface, and create virtual machines on proxmox.

**Specific Goal:** Create a resource management dashboard in the web UI, and allow users to interface with proxmox so they are able to create/manage VMs.

**Stretch Goal:** Implement logging to keep track of changes to virtual machines.

### Selected Stories

-   Feature 1: User & Resource Dashboard
    -   As a System Architect, I want to be able to view existing resources, so that I can make sure systems are running correctly.
    -   As a System Architect, I want to be able to add new resources so that I am able to update the system with new equipment.
-   Feature 2: Virtualization Integration
    -   As a Researcher, I want to create new virtual machines, so that I can run my projects.
    -   As a Researcher, I want to edit my existing virtual machines, so that I can make changes to my projects as I go.
    -   As a Researcher, I want to delete my virtual machines, so that I can free up resources when I finish my projects.

### Story Decomposition

#### Feature 1: User & Resource Dashboard

**Story 1:** - As a System Architect, I want to be able to view existing resources, so that I can make sure systems are running correctly.

Tasks:

-   frontend:
    -   Dashboard needs to get current resources from API and display them to users.
    -   UI should include details of each system, and wether the system is running or not.
    -   app routes:
        -   GET: /dashboard
-   backend:
    -   Needs to get current resources from DB, and make that available to the frontend.
    -   api routes:
        -   GET: /
-   integration:
    -   Test should be created for the frontend to make sure resources are displayed correctly.
    -

**Story 2:** - As a System Architect, I want to be able to add new resources so that I am able to update the system with new equipment.

Tasks:

-   frontend:
    -   Dashboard should include an method to add a new host/resource.
    -   This could be either a separate page or a modal on the dashboard.
    -   app routes:
        -   GET: /
-   backend:
    -   d
-   integration:
    -   d

#### Feature 2: Virtualization Integration

**Story 1:** -

Tasks:

-   frontend:
    -   d
-   backend:
    -   d
-   integration:
    -   d

**Story 2:** -

Tasks:

-   frontend:
    -   d
-   backend:
    -   d
-   integration:
    -   d

### Data Models & API Spec

...

### Responsibilities

-   Feature 1: User & Resource Dashboard
    -   Dan McCarthy
    -   .
-   Feature 2: Virtualization Integration
    -   .

## Gitlab

Below is the link to our issue board. Our board has been populated with issues and everyone has been assigned atleast one task. (Note: our project is on Github)

https://github.com/orgs/OpnLaaS/projects/1
