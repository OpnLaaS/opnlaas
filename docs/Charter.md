# OpnLaaS Project Charter

## 1. Essentials

### **Project Name**

OpnLaaS (Open Lab as a Service)

### **Team Members**

* Evan Parker – (System Architecture & Backend)
* Dan McCarthy – (Frontend Development)
* Matt Gee - (Full Stack Development)
* Kestutis Biskis – (Frontend Development😭 & Database Systems)
* Alex Houle – (Backend API Development & Frontend Integration)

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

### **Persona 2 - Dan -**

* **Bio:**
* **Motivation:**
* **Goals:**

### **Persona 3 - Matt -**

* **Bio:**
* **Motivation:** 
* **Goals:**

### **Persona 4 - Kestutis -**

* **Bio:** Student interested in networking programming looking to create a network communication protocol that will require multiple hosts on a network.
* **Motivation:** wants to have control of multiple hosts on a network for testing with minimal setup.
* **Goals:** enable users to easily create multiple hosts on a network that can communicate with eachother.

### **Persona 5 - Alex -**

* **Bio:**
* **Motivation:** 
* **Goals:**

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