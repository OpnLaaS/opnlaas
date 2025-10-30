# OpnLaaS GitHub Workflows

Workflows are important for automating tasks for testing, building, and releasing. This document provides an overview of the workflows defined in the `.github/workflows` directory.

## Pull Request Workflow (`pr.yml`)

This workflow is triggered on pull requests to the `main` branch. It runs tests and checks to ensure code quality before merging.

## Continuous Integration/Continuous Deployment Workflow (`opnlaas.yml`)

This workflow is triggered on pushes to the `main` branch. It handles building, testing, and deploying the OpnLaaS application.