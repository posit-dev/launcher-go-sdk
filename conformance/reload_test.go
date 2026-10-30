package conformance_test

import (
	"context"
	"testing"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/conformance"
	"github.com/posit-dev/launcher-go-sdk/settings"
)

// noReloadPlugin implements launcher.Plugin but neither reload interface -
// exercises RunReload's RequestNotSupportedWhenNoReloadInterfaceImplemented
// subtest.
type noReloadPlugin struct{ testPlugin }

func newNoReloadPlugin(t *testing.T) *noReloadPlugin {
	t.Helper()
	return &noReloadPlugin{testPlugin: *newTestPlugin(t)}
}

// settingsReloadTestPlugin implements launcher.SettingsReloadablePlugin,
// wired the way a real plugin author would: construct one *settings.Reloader
// at startup and return it from SettingsReloader().
type settingsReloadTestPlugin struct {
	testPlugin
	reloader *settings.Reloader
}

func newSettingsReloadTestPlugin(t *testing.T, seed api.InheritedSettings) *settingsReloadTestPlugin {
	t.Helper()
	return &settingsReloadTestPlugin{
		testPlugin: *newTestPlugin(t),
		reloader:   settings.NewReloader(settings.Registry, settings.StaticOwnConfSource{}, seed, seed.JobExpiryHours, nil),
	}
}

func (p *settingsReloadTestPlugin) SettingsReloader() *settings.Reloader {
	return p.reloader
}

func TestRunSettingsRegistry(t *testing.T) {
	conformance.RunSettingsRegistry(t)
}

func TestRunReload_NoReloadInterface(t *testing.T) {
	p := newNoReloadPlugin(t)
	conformance.RunReload(t, p, conformance.ReloadOpts{})
}

func TestRunReload_SettingsReloadablePlugin(t *testing.T) {
	seed := api.InheritedSettings{
		ServerUser:                          "rstudio-server",
		EnableDebugLogging:                  false,
		ScratchPath:                         "/var/lib/rstudio-launcher/local",
		LoggingDir:                          "/var/log/rstudio/launcher",
		HeartbeatIntervalSeconds:            5,
		JobExpiryHours:                      24,
		PluginMetricsIntervalSeconds:        60,
		IncludePluginMetricsIntervalSeconds: true,
	}
	p := newSettingsReloadTestPlugin(t, seed)
	conformance.RunReload(t, p, conformance.ReloadOpts{StartupInherited: seed})
}

// configReloadTestPlugin implements the older
// launcher.ConfigReloadablePlugin, to confirm RunReload's
// RequestNotSupported subtest correctly skips itself for a plugin that does
// declare reload support (even via the non-Settings interface).
type configReloadTestPlugin struct {
	testPlugin
}

func (p *configReloadTestPlugin) ReloadConfig(context.Context) error { return nil }

func TestRunReload_ConfigReloadablePlugin(t *testing.T) {
	p := &configReloadTestPlugin{testPlugin: *newTestPlugin(t)}
	conformance.RunReload(t, p, conformance.ReloadOpts{})
}

// TestRunReload_SettingsReloadablePlugin_OwnConfOwnsRestartRequiredKey is the
// regression test for I6: before the fix, RunReload's
// AppliedAndPendingRestartClassification and
// AbsentInheritedSettingsDoesNotClobberCache subtests assumed every
// RestartRequired key was controlled by the pushed InheritedSettings, and
// would false-fail a correct plugin whose own conf legitimately sets one of
// them (own-conf wins over the pushed value, per the whole design's
// precedence rule). This plugin's own conf sets scratch-path explicitly, to
// the exact value seed.ScratchPath already has, so scratch-path never
// becomes pendingRestart no matter what RunReload pushes - the old
// assertions ("pendingRestart must contain every RestartRequired key",
// keyed off "server-user" specifically for the cache-clobber precondition)
// would have failed here for a plugin that is behaving correctly.
func TestRunReload_SettingsReloadablePlugin_OwnConfOwnsRestartRequiredKey(t *testing.T) {
	seed := api.InheritedSettings{
		ServerUser:                          "rstudio-server",
		EnableDebugLogging:                  false,
		ScratchPath:                         "/var/lib/rstudio-launcher/local",
		LoggingDir:                          "/var/log/rstudio/launcher",
		HeartbeatIntervalSeconds:            5,
		JobExpiryHours:                      24,
		PluginMetricsIntervalSeconds:        60,
		IncludePluginMetricsIntervalSeconds: true,
	}
	ownConf := settings.StaticOwnConfSource{"scratch-path": seed.ScratchPath}
	p := &settingsReloadTestPlugin{
		testPlugin: *newTestPlugin(t),
		reloader:   settings.NewReloader(settings.Registry, ownConf, seed, seed.JobExpiryHours, nil),
	}
	conformance.RunReload(t, p, conformance.ReloadOpts{StartupInherited: seed})
}
