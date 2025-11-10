# Sprint planning

## Rubric and submission:

### Rubric

- document:
	- [4] selection
		- sprint goal (selected features)
		- selected stories, grouped by feature
	- [4] story decomposition (tasks)
	- [4] data model and API specs
	- [4] responsibilities
- [4] Gitlab: issue board, tasks, assignments
	- all team members are assigned to issues / tasks

### Submission

- Submit document that summarizes the items below.
- Add your data model and API files to your group repository and provide a link to those files in your submitted document.

## Sprint planning

### Sprint goal

- Identify sprint goal.
	- The goal should span two features and clearly describe the result.
	- (see examples)
- Select stories that contribute to that goal.
	- this will be a subset of your priority stories.

### Story decomposition

For each chosen story:

- identify the "definition of done"
- Identify the work required to complete the story.

Examples of tasks:

Work items / Gitlab issues:

- Design:
	- Data models
	- API specification
- Frontend:
	- UI
	- routing
	- API access
- Backend:
	- API (internal or external)
	- Business logic
	- Database operations
- Integration
	- backend deployment
	- connect API to UI
	- frontend deployment

Notes:

- Unit testing is expected for all stories, so it doesn't need to be a separate issue.
- Backend can test independently.
- Frontend can test independently of by using mocking.

### Designs

- For each feature, design a data model and API.
- TODO:
	- data model: py files with pydantic model(s)
	- API:
		- internal: py files with function signatures (akin to our user_api.py or user_client.py)
		- external: API endpoints (method, routes), parameters, and returns (akin to your fastAPI main.py)
		- you can produce skeleton files with just the function definitions.

### Responsibilities

- Identify team member responsibilities using the matrix below.

|          | Feature 1 | Feature 2 |
|----------|-----------|-----------|
| Frontend | Person A  | Person B  |
| Backend  | Person C  | Person D  |

## Gitlab

### Gitlab setup issue board

- Create iteration (done for you)
- Setup issue board
	- plan > issue boards
	- [create list]
	- iteration > select iteration
	- move iteration in between "Open" and "Closed"

### Gitlab sprint planning

- decomposition: create issues for the work tasks you've identified.
	- use labels for:
		- feature
		- discipline (design, frontend, backend, integration)
	- (optional) use tasks for:
		- specific tasks (second-order bullets above)
- design: create designs and add to your repo.
- selection: Move selected stories into current iteration.
- assignments: team members should self-assign issues appropriate to them.