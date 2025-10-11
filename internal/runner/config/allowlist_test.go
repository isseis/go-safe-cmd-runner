package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

func TestDetermineEffectiveAllowlist_Inherit(t *testing.T) {
	// Test that Group.EnvAllowlist == nil results in inheritance from Global.EnvAllowlist
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME", "PATH", "USER"},
	}
	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		EnvAllowlist: nil, // Should inherit from global
	}

	result := DetermineEffectiveAllowlist(group, global)

	expected := []string{"HOME", "PATH", "USER"}
	if len(result) != len(expected) {
		t.Errorf("Expected %d items in allowlist, got %d", len(expected), len(result))
		return
	}

	for i, item := range expected {
		if result[i] != item {
			t.Errorf("Expected allowlist[%d] = %q, got %q", i, item, result[i])
		}
	}
}

func TestDetermineEffectiveAllowlist_Override(t *testing.T) {
	// Test that Group.EnvAllowlist != nil overrides Global.EnvAllowlist
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME", "PATH", "USER"},
	}
	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		EnvAllowlist: []string{"CUSTOM_VAR", "ANOTHER_VAR"}, // Should override global
	}

	result := DetermineEffectiveAllowlist(group, global)

	expected := []string{"CUSTOM_VAR", "ANOTHER_VAR"}
	if len(result) != len(expected) {
		t.Errorf("Expected %d items in allowlist, got %d", len(expected), len(result))
		return
	}

	for i, item := range expected {
		if result[i] != item {
			t.Errorf("Expected allowlist[%d] = %q, got %q", i, item, result[i])
		}
	}
}

func TestDetermineEffectiveAllowlist_Reject(t *testing.T) {
	// Test that Group.EnvAllowlist == [] (empty slice) rejects all system environment variables
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME", "PATH", "USER"},
	}
	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		EnvAllowlist: []string{}, // Empty slice should reject all
	}

	result := DetermineEffectiveAllowlist(group, global)

	if len(result) != 0 {
		t.Errorf("Expected empty allowlist (all rejected), got %v", result)
	}
}

func TestDetermineEffectiveAllowlist_GlobalNil(t *testing.T) {
	// Test that Global.EnvAllowlist == nil still works for inheritance
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: nil,
	}
	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		EnvAllowlist: nil, // Should inherit nil from global
	}

	result := DetermineEffectiveAllowlist(group, global)

	if result != nil {
		t.Errorf("Expected nil allowlist (inherit nil), got %v", result)
	}
}

func TestDetermineEffectiveAllowlist_GlobalEmpty(t *testing.T) {
	// Test that Global.EnvAllowlist == [] (empty slice) can be inherited
	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{},
	}
	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		EnvAllowlist: nil, // Should inherit empty slice from global
	}

	result := DetermineEffectiveAllowlist(group, global)

	if len(result) != 0 {
		t.Errorf("Expected empty allowlist (inherit empty), got %v", result)
	}
}
