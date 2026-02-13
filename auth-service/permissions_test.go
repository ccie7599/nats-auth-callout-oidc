package main

import (
	"reflect"
	"sort"
	"testing"
)

func TestResolvePermissions_Admin(t *testing.T) {
	p := ResolvePermissions([]string{"nats:admin"})
	if !reflect.DeepEqual(p.PubAllow, []string{">"}) {
		t.Errorf("expected pub [>], got %v", p.PubAllow)
	}
	if !reflect.DeepEqual(p.SubAllow, []string{">"}) {
		t.Errorf("expected sub [>], got %v", p.SubAllow)
	}
	if !p.HasPermissions() {
		t.Error("expected HasPermissions true")
	}
}

func TestResolvePermissions_Publisher(t *testing.T) {
	p := ResolvePermissions([]string{"nats:publish"})
	sort.Strings(p.PubAllow)
	if !reflect.DeepEqual(p.PubAllow, []string{"events.>", "orders.>"}) {
		t.Errorf("expected pub [events.> orders.>], got %v", p.PubAllow)
	}
	if !reflect.DeepEqual(p.SubAllow, []string{"_INBOX.>"}) {
		t.Errorf("expected sub [_INBOX.>], got %v", p.SubAllow)
	}
}

func TestResolvePermissions_Subscriber(t *testing.T) {
	p := ResolvePermissions([]string{"nats:subscribe"})
	if len(p.PubAllow) != 0 {
		t.Errorf("expected no pub, got %v", p.PubAllow)
	}
	sort.Strings(p.SubAllow)
	expected := []string{"_INBOX.>", "events.>", "orders.>"}
	if !reflect.DeepEqual(p.SubAllow, expected) {
		t.Errorf("expected sub %v, got %v", expected, p.SubAllow)
	}
}

func TestResolvePermissions_NoNATSScopes(t *testing.T) {
	p := ResolvePermissions([]string{"openid", "profile"})
	if p.HasPermissions() {
		t.Error("expected no permissions for non-NATS scopes")
	}
}

func TestResolvePermissions_Combined(t *testing.T) {
	p := ResolvePermissions([]string{"nats:publish", "nats:subscribe"})
	sort.Strings(p.PubAllow)
	sort.Strings(p.SubAllow)

	expectedPub := []string{"events.>", "orders.>"}
	if !reflect.DeepEqual(p.PubAllow, expectedPub) {
		t.Errorf("expected pub %v, got %v", expectedPub, p.PubAllow)
	}

	expectedSub := []string{"_INBOX.>", "events.>", "orders.>"}
	if !reflect.DeepEqual(p.SubAllow, expectedSub) {
		t.Errorf("expected sub %v, got %v", expectedSub, p.SubAllow)
	}
}

func TestResolvePermissions_Empty(t *testing.T) {
	p := ResolvePermissions(nil)
	if p.HasPermissions() {
		t.Error("expected no permissions for nil scopes")
	}
}
