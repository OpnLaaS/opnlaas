# OpnLaaS Project Charter

## 1. Essentials

### **Project Name**

OpnLaaS (Open Lab as a Service)

### **Team Members**

* Evan Parker – BS Computer Science – (System Architecture & Backend)
* Dan McCarthy – BS IT – (Frontend Development)
* Matt Gee – BS Computer Science – (Full Stack Development)
* Kestutis Biskis – BS Computer Science – (Frontend Development & Database Systems)
* Alex Houle – BS Computer Science – (Backend API Development & Frontend Integration)

### **Target Audience**

Students, faculty, and researchers at the University of New Hampshire who need rapid access to lab infrastructure (bare metal servers, VMs, containers) for coursework, experimentation, and cybersecurity training.

## 2. Long-Term Vision

### **Vision**

Empower students and administrators with an open, self-service platform to deploy and manage infrastructure resources in a unified and automated way, lowering technical barriers for hands-on learning and research.

### **Value**

OpnLaaS streamlines provisioning for both users and admins:

* **For students:** enables quick, self-service VM and server deployments.
* **For admins:** centralizes management, reduces manual setup, and integrates with existing systems (FreeIPA, Proxmox, etc.).

## 3. Personas

### **Persona 1 - Evan - System Architect**

* **Bio:** The System Architect responsible for designing and maintaining the overall infrastructure. Primary system administrator for the cybersecurity lab, oversees FreeIPA, Proxmox, Bare-Metal servers and their BMCs. Interested in automation and security while providing robust solutions.
* **Motivation:** Wants to automate resource allocation to save time on repetitive setup, additionally wants to enable students and faculty to independently manage their lab resources while promoting projects.
* **Goals:** Streamline user onboarding, reduce provisioning errors, and ensure uptime across lab infrastructure. Create a safe and effective application.

### **Persona 2 - Dan - Student**

* **Bio:** A student who wants to deploy there personal projects on real hardware. Isn't as focused on the technical details or computation power of the systems, is more focused on being able to easily get there projects running and accessible to the public. Doesns't want to deal with complicated UI or deployment process for there projects, and would prefer if software was intuitive to use.
* **Motivation:** Wants to use a service that makes iteracting with the underlying systems as simple as possible without needing a lot of technical skills. Wants to focus on there projects, not dealing with infrastructure.
* **Goals:** Setup there projects on the provided systems, and share the things they've made publicly.

### **Persona 3 - Matt - Researcher**

* **Bio:** A researcher in need of a reserved system in which they can utilized computation power they do not have access to or are unable to procure. Interested in being able to reserve / book resources from the service with the ability to manage booked resources as if the resource was on their network.
* **Motivation:** Wants to be able to book resources to be used for approved personalized projects, additionally may need additional resources on-demand with control over the systems.  
* **Goals:** Streamline ability for onboarded users to book hosts and provide interfaces in order to execute Redfish or IPMI commands on the allocated systems. 

### **Persona 4 - Kestutis -**

* **Bio:**
* **Motivation:**
* **Goals:**

### **Persona 5 - Alex -**

* **Bio:** A student who challenges a friend to create a better chess engine, Student who wants to learn to make a large scale project for the first time in a friendly competition with their friends. 
* **Motivation:** Wants to test which of the engines are better by simulating many games against each other. 
* **Goals:** Provide compute to students to simulate hundreds of chess games.


## 4. Epics and Features

### **Epic: Infrastructure Automation and Self-Service Management**

#### **Feature 1 – User & Resource Dashboard**

* Unified dashboard for users to log in, view resources, and submit booking requests.
* Admins can view and approve bookings, manage infrastructure, and monitor server states.

#### **Feature 2 – Virtualization Integration**

* Connects OpnLaaS to Proxmox API for automated VM/CT creation.
* Includes control actions: start, stop, reboot, and delete.

#### **Feature 3 – Bare Metal Provisioning**

* Integrates with IPMI and BMC interfaces for bare metal server management.
* Supports automated ISO repository for automated OS installation.
* Supports automated OS deployment and hardware monitoring.

## 5. Stories

### **Feature 1 – User & Resource Dashboard**

### **Feature 2 – Virtualization Integration**

## 6. Product Backlog (GitLab)

Please See: https://github.com/orgs/OpnLaaS/projects/1

## 7. Notes / Future Work

* Add Bare Metal Provisioning System and Booking System integration.
* Expand test coverage and monitoring.