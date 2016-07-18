package main

import (
	"testing"
)

func TestLoadApi(t *testing.T) {
	a := api{}

	if err := a.read("etc/weft_api.toml"); err != nil {
		t.Error(err)
	}

	if err := a.writeHandlers("etc/handlers_auto.go"); err != nil {
		t.Error(err)
	}

	if err := a.writeDocs("etc/index.html"); err != nil {
		t.Error(err)
	}
}
