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
