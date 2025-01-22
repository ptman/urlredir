// Copyright © Paul Tötterman <paul.totterman@gmail.com>. All rights reserved.

package main

import (
	"html/template"
	"testing"
)

func TestParseAdminPage(t *testing.T) {
	t.Parallel()

	_, err := template.New("adminPage").Parse(adminPage)
	if err != nil {
		t.Errorf("Error parsing template: %v", err)
	}
}
