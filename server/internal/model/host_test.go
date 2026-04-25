package model

import (
	"reflect"
	"strings"
	"testing"
)

func TestHostOSTypeUsesExistingOSTypeColumn(t *testing.T) {
	field, ok := reflect.TypeOf(Host{}).FieldByName("OSType")
	if !ok {
		t.Fatal("Host.OSType field is missing")
	}
	if tag := string(field.Tag.Get("gorm")); !strings.Contains(tag, "os_type") {
		t.Fatalf("expected Host.OSType to map to os_type column, got %q", tag)
	}
}
