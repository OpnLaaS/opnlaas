# OPN LaaS Product and Developer Manual

## Final Sprint Recap

### Responsibilities

Below is a breakdown of the responsibilites for the features we worked on during this sprint.

- Feature 1: Resource Dashboard
	- Dan McCarthy
	- Kestutis Biskis
- Feature 2: Virtualization Integration
	- Evan Parker
	- Matt Gee
	- Alex Houle

### Backlog Review

stuff here............

## Developer Documentation

Our project can be accessed at

[e](e)

Our repo can be found below:

[https://github.com/OpnLaaS/opnlaas](https://github.com/OpnLaaS/opnlaas)

...

### Repository Overview

# OPN LaaS Product and Developer Manual

## Repository Structure

### App

Contains the backend code that immediately talks to the frontend and receives web requests, contains:
- Defined routes on the backend
- Primary functions called by the routes
  - Api routes and views are in separate files
- Any needed middleware for the routes

### Auth

Responsible for managing the authentication pipeline between the remote (network) ldap and the browser.
- Contains ability to "inject" a user to become authenticated for testing, but this is not used for production code

### Config

Loads, types, and verifies initial configuration values from our config.toml file. 

### DB

The database management for our codebase, contains defined database schemas and functions to initialize the local database when the backend starts.

### Docs

Your already here!

### Host

Software to interface with, and manage hosts; whether they are virtual or physical. 

#### Iso
Files and scripts for parsing and extracting relevant content from uploaded iso files, these files are then stored to be used when a host is attempting to pxe boot.

#### PXE
Contains the code to automate the provisioning process of pxe booting a server with an automate install of an operating system. Contains files to run necessary services other than http and templating functions and files in order to create auto-configuration files such as kickstart, autoinstall, and grub commands. 

### Public

Contains content served to the browser, including CSS, Javascript, images, and html templates. The content is primarily served from the app directory. 

### Scripting

Script utilities that are not run automatically as they server "one time" functionality for developers and maintainers.

### SSH

Files for managing ssh connections and keys automatically to allow the backend to create ssh sessions and use them as needed. 

### Tests

Contains all of the backend tests for the project, these are running automatically by the github actions runner configured in .github. 

### VM

Files for management, creation, and deletion of virtual resources through the Proxmox api to the local proxmox session in the Cybersecurity lab.



### Running the Project Locally

...

## Testing

### Running Backend Tests

add instructions here:

### Running Frontend Tests

Due to the complexity of some of our frontend features we didn't implement automated testing for them. Below is documentation on how to use the 2 primary features we worked on during this spint.

#### Feature 1: Resource Dashboard

#### Feature 2: Virtualization Integration