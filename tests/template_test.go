package tests

import "testing"

func TestTemplate(t *testing.T) {
	setup(t)
	defer cleanup(t)

	t.Log("This is a template test.")
}
