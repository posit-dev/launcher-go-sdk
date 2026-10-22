package reloaddispatch

import (
	"context"
	"fmt"
	"testing"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/settings"
)

// neitherPlugin implements neither SettingsReloadablePlugin nor
// ConfigReloadablePlugin.
type neitherPlugin struct{}

func TestHandle_NeitherInterfaceImplemented(t *testing.T) {
	errType, errMsg, applied, pendingRestart, generation := Handle(context.Background(), neitherPlugin{}, nil, 42)

	if errType != api.ReloadErrorRequestNotSupported {
		t.Errorf("errorType = %v, want RequestNotSupported", errType)
	}
	if errMsg == "" {
		t.Error("errorMessage is empty, want a non-empty explanation")
	}
	if applied != nil {
		t.Errorf("applied = %v, want nil", applied)
	}
	if pendingRestart != nil {
		t.Errorf("pendingRestart = %v, want nil", pendingRestart)
	}
	if generation != 42 {
		t.Errorf("echoedGeneration = %d, want 42 (echoed back verbatim)", generation)
	}
}

// configReloadPlugin implements ConfigReloadablePlugin.
type configReloadPlugin struct{ err error }

func (p configReloadPlugin) ReloadConfig(context.Context) error { return p.err }

func TestHandle_ConfigReloadablePlugin_Success(t *testing.T) {
	errType, errMsg, applied, pendingRestart, generation := Handle(context.Background(), configReloadPlugin{}, nil, 7)

	if errType != api.ReloadErrorNone {
		t.Errorf("errorType = %v, want None", errType)
	}
	if errMsg != "" {
		t.Errorf("errorMessage = %q, want empty", errMsg)
	}
	if applied != nil || pendingRestart != nil {
		t.Errorf("applied = %v, pendingRestart = %v, want both nil (ConfigReloadablePlugin does not report either)", applied, pendingRestart)
	}
	if generation != 7 {
		t.Errorf("echoedGeneration = %d, want 7", generation)
	}
}

func TestHandle_ConfigReloadablePlugin_ClassifiedError(t *testing.T) {
	p := configReloadPlugin{err: &ConfigReloadError{Type: api.ReloadErrorLoad, Message: "config file syntax error"}}
	errType, errMsg, _, _, _ := Handle(context.Background(), p, nil, 1)

	if errType != api.ReloadErrorLoad {
		t.Errorf("errorType = %v, want Load", errType)
	}
	if errMsg != "config file syntax error" {
		t.Errorf("errorMessage = %q, want %q", errMsg, "config file syntax error")
	}
}

func TestHandle_ConfigReloadablePlugin_UnclassifiedError(t *testing.T) {
	p := configReloadPlugin{err: fmt.Errorf("something broke")}
	errType, errMsg, _, _, _ := Handle(context.Background(), p, nil, 1)

	if errType != api.ReloadErrorUnknown {
		t.Errorf("errorType = %v, want Unknown", errType)
	}
	if errMsg != "something broke" {
		t.Errorf("errorMessage = %q, want %q", errMsg, "something broke")
	}
}

// settingsReloadPlugin implements SettingsReloadablePlugin.
type settingsReloadPlugin struct{ reloader *settings.Reloader }

func (p settingsReloadPlugin) SettingsReloader() *settings.Reloader { return p.reloader }

func testInheritedSettings() api.InheritedSettings {
	return api.InheritedSettings{
		ServerUser:                          "rstudio-server",
		ScratchPath:                         "/var/lib/rstudio-launcher/local",
		LoggingDir:                          "/var/log/rstudio/launcher",
		HeartbeatIntervalSeconds:            5,
		JobExpiryHours:                      24,
		PluginMetricsIntervalSeconds:        60,
		IncludePluginMetricsIntervalSeconds: true,
	}
}

func TestHandle_SettingsReloadablePlugin_Success(t *testing.T) {
	inherited := testInheritedSettings()
	reloader := settings.NewReloader(settings.Registry, settings.StaticOwnConfSource{}, inherited, 24, nil)
	p := settingsReloadPlugin{reloader: reloader}

	pushed := inherited
	pushed.JobExpiryHours = 48

	errType, errMsg, applied, _, generation := Handle(context.Background(), p, &pushed, 9)

	if errType != api.ReloadErrorNone {
		t.Errorf("errorType = %v (%s), want None", errType, errMsg)
	}
	found := false
	for _, k := range applied {
		if k == "job-expiry-hours" {
			found = true
		}
	}
	if !found {
		t.Errorf("applied = %v, want to contain job-expiry-hours", applied)
	}
	if generation != 9 {
		t.Errorf("echoedGeneration = %d, want 9", generation)
	}
}

func TestHandle_SettingsReloadablePlugin_ValidationError(t *testing.T) {
	inherited := testInheritedSettings()
	reloader := settings.NewReloader(settings.Registry, settings.StaticOwnConfSource{}, inherited, 24, nil)
	p := settingsReloadPlugin{reloader: reloader}

	pushed := inherited
	pushed.JobExpiryHours = -1

	errType, errMsg, applied, pendingRestart, generation := Handle(context.Background(), p, &pushed, 3)

	if errType != api.ReloadErrorValidate {
		t.Errorf("errorType = %v, want Validate", errType)
	}
	if errMsg == "" {
		t.Error("errorMessage is empty, want a validation message")
	}
	if applied != nil || pendingRestart != nil {
		t.Errorf("applied = %v, pendingRestart = %v, want both nil on a validation failure", applied, pendingRestart)
	}
	if generation != 3 {
		t.Errorf("echoedGeneration = %d, want 3 (echoed back even on error)", generation)
	}
}

func TestHandle_SettingsReloadablePlugin_NilReloader(t *testing.T) {
	p := settingsReloadPlugin{reloader: nil}

	errType, errMsg, _, _, generation := Handle(context.Background(), p, nil, 5)

	if errType != api.ReloadErrorUnknown {
		t.Errorf("errorType = %v, want Unknown", errType)
	}
	if errMsg == "" {
		t.Error("errorMessage is empty, want an explanation naming the nil SettingsReloader")
	}
	if generation != 5 {
		t.Errorf("echoedGeneration = %d, want 5", generation)
	}
}

// bothPlugin implements both SettingsReloadablePlugin and
// ConfigReloadablePlugin, so Handle's additive dispatch (I1) can be
// exercised directly: this is the untested combination the whole-branch
// review flagged - before the fix, a plugin implementing both would never
// have ReloadConfig called at all.
type bothPlugin struct {
	reloader        *settings.Reloader
	reloadConfigErr error
	reloadConfigRan bool
}

func (p *bothPlugin) SettingsReloader() *settings.Reloader { return p.reloader }

func (p *bothPlugin) ReloadConfig(context.Context) error {
	p.reloadConfigRan = true
	return p.reloadConfigErr
}

func TestHandle_BothInterfaces_BothRunOnSuccess(t *testing.T) {
	inherited := testInheritedSettings()
	reloader := settings.NewReloader(settings.Registry, settings.StaticOwnConfSource{}, inherited, 24, nil)
	p := &bothPlugin{reloader: reloader}

	pushed := inherited
	pushed.JobExpiryHours = 48

	errType, errMsg, applied, _, generation := Handle(context.Background(), p, &pushed, 9)

	if errType != api.ReloadErrorNone {
		t.Fatalf("errorType = %v (%s), want None", errType, errMsg)
	}
	if !p.reloadConfigRan {
		t.Error("ReloadConfig was not called - implementing SettingsReloadablePlugin must not silently disable ConfigReloadablePlugin.ReloadConfig")
	}
	found := false
	for _, k := range applied {
		if k == "job-expiry-hours" {
			found = true
		}
	}
	if !found {
		t.Errorf("applied = %v, want to contain job-expiry-hours (the settings reload's own work must still be reported)", applied)
	}
	if generation != 9 {
		t.Errorf("echoedGeneration = %d, want 9", generation)
	}
}

func TestHandle_BothInterfaces_SettingsSucceedsReloadConfigFails(t *testing.T) {
	inherited := testInheritedSettings()
	reloader := settings.NewReloader(settings.Registry, settings.StaticOwnConfSource{}, inherited, 24, nil)
	p := &bothPlugin{
		reloader:        reloader,
		reloadConfigErr: &ConfigReloadError{Type: api.ReloadErrorLoad, Message: "profiles corrupt"},
	}

	pushed := inherited
	pushed.JobExpiryHours = 48

	errType, errMsg, applied, _, _ := Handle(context.Background(), p, &pushed, 1)

	if !p.reloadConfigRan {
		t.Fatal("ReloadConfig was not called")
	}
	// The Launcher must never be told the reload as a whole succeeded when
	// ReloadConfig failed, even though the settings half genuinely worked.
	if errType != api.ReloadErrorLoad {
		t.Errorf("errorType = %v, want Load (ReloadConfig's classified error)", errType)
	}
	if errMsg == "" {
		t.Error("errorMessage is empty, want an explanation mentioning the ReloadConfig failure")
	}
	found := false
	for _, k := range applied {
		if k == "job-expiry-hours" {
			found = true
		}
	}
	if !found {
		t.Errorf("applied = %v, want to still contain job-expiry-hours - the settings reload genuinely applied it before ReloadConfig failed", applied)
	}
}

func TestHandle_BothInterfaces_SettingsFailureSkipsReloadConfig(t *testing.T) {
	inherited := testInheritedSettings()
	reloader := settings.NewReloader(settings.Registry, settings.StaticOwnConfSource{}, inherited, 24, nil)
	p := &bothPlugin{reloader: reloader}

	pushed := inherited
	pushed.JobExpiryHours = -1 // triggers a settings validation failure

	errType, errMsg, applied, pendingRestart, _ := Handle(context.Background(), p, &pushed, 1)

	if errType != api.ReloadErrorValidate {
		t.Errorf("errorType = %v (%s), want Validate", errType, errMsg)
	}
	if applied != nil || pendingRestart != nil {
		t.Errorf("applied = %v, pendingRestart = %v, want both nil on a settings validation failure", applied, pendingRestart)
	}
	if p.reloadConfigRan {
		t.Error("ReloadConfig was called despite the settings reload failing - there is nothing well-defined to apply it on top of")
	}
}
