// Package reloaddispatch implements the config-reload dispatch decision
// shared by the launcher package's wire-protocol handler and the
// conformance package's plugin-author reload conformance area
// (conformance.RunReload). It exists purely to avoid duplicating (and
// risking a silent drift between) that dispatch logic in two places,
// without expanding launcher's public API surface to share it: this
// package is internal, so only code within this module can import it, and
// the conformance package (also within this module) is exactly the other
// consumer that needs it.
//
// This package intentionally does not import package launcher, even
// though its Handle function's dispatch decision is documented on
// launcher.SettingsReloadablePlugin and launcher.ConfigReloadablePlugin:
// launcher imports this package (to share its own createHandler's
// dispatch logic), so the reverse import would be a cycle. Instead, the
// two interfaces below mirror those public interfaces' method shapes
// structurally - Go interface satisfaction is duck-typed, so any plugin
// value satisfying launcher.SettingsReloadablePlugin or
// launcher.ConfigReloadablePlugin also satisfies the identically-shaped
// interface here, with no import needed.
package reloaddispatch

import (
	"context"
	"errors"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/settings"
)

// ConfigReloadError represents a configuration reload failure with a
// classified error type. This is the real definition backing the public
// launcher.ConfigReloadError, which is a type alias to this type - see
// that alias's doc comment in launcher/launcher.go for the plugin-author-
// facing contract. It lives here, rather than in launcher, only so Handle
// can classify a returned error without launcher needing to import this
// package's Handle in one direction while this package imports
// launcher.ConfigReloadError in the other.
type ConfigReloadError struct {
	Type    api.ConfigReloadErrorType
	Message string
}

// Error implements the error interface.
func (e *ConfigReloadError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "config reload failed: " + e.Type.String()
}

// SettingsReloadablePlugin mirrors launcher.SettingsReloadablePlugin's
// method shape (see that type for the documented, plugin-author-facing
// contract). A plugin implementing the public interface automatically
// satisfies this one too, structurally, with no dependency on package
// launcher.
type SettingsReloadablePlugin interface {
	// SettingsReloader returns the plugin's *settings.Reloader.
	SettingsReloader() *settings.Reloader
}

// ConfigReloadablePlugin mirrors launcher.ConfigReloadablePlugin's method
// shape - see SettingsReloadablePlugin's doc comment for why no import of
// package launcher is needed here.
type ConfigReloadablePlugin interface {
	// ReloadConfig is called when the Launcher requests a configuration
	// reload.
	ReloadConfig(ctx context.Context) error
}

// Handle implements the config-reload dispatch decision documented on
// launcher.SettingsReloadablePlugin and launcher.ConfigReloadablePlugin:
// SettingsReloadablePlugin takes precedence when p implements it; a
// plugin implementing neither is reported as
// api.ReloadErrorRequestNotSupported. generation is echoed back verbatim
// as echoedGeneration, so every caller of Handle gets that guarantee for
// free rather than re-threading the value itself.
//
// p is typed any (rather than a shared plugin interface) because this
// package deliberately has nothing resembling launcher.Plugin to name -
// see the package doc for why importing one would cycle.
func Handle(ctx context.Context, p any, inherited *api.InheritedSettings, generation uint) (errorType api.ConfigReloadErrorType, errorMessage string, applied, pendingRestart []string, echoedGeneration uint) {
	echoedGeneration = generation

	if srPlugin, ok := p.(SettingsReloadablePlugin); ok {
		reloader := srPlugin.SettingsReloader()
		if reloader == nil {
			// The plugin claims to support settings reload by implementing
			// SettingsReloadablePlugin, but supplied no Reloader - a
			// plugin-author bug (e.g. forgot to construct one, or an
			// initialization-order mistake), not something the Launcher
			// asked for. Report it as an unclassified reload failure rather
			// than ReloadErrorRequestNotSupported: RequestNotSupported would
			// misrepresent this as "this plugin type doesn't do settings
			// reload", when the plugin itself claims otherwise. Above all,
			// never panic the whole plugin process on every reload request.
			errorType = api.ReloadErrorUnknown
			errorMessage = "plugin implements SettingsReloadablePlugin but SettingsReloader() returned nil"
			return
		}
		report, err := reloader.Reload(ctx, inherited, generation)
		if err != nil {
			var verr *settings.ValidationError
			errorType = api.ReloadErrorUnknown
			if errors.As(err, &verr) {
				errorType = api.ReloadErrorValidate
			}
			errorMessage = err.Error()
			return
		}
		applied = report.Applied
		pendingRestart = report.PendingRestart
		return
	}

	crPlugin, ok := p.(ConfigReloadablePlugin)
	if !ok {
		errorType = api.ReloadErrorRequestNotSupported
		errorMessage = "this plugin does not support configuration reload"
		return
	}
	if err := crPlugin.ReloadConfig(ctx); err != nil {
		var crErr *ConfigReloadError
		if errors.As(err, &crErr) {
			errorType = crErr.Type
			errorMessage = crErr.Message
		} else {
			errorType = api.ReloadErrorUnknown
			errorMessage = err.Error()
		}
		return
	}
	return
}
