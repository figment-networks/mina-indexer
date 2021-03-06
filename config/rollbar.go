package config

import (
	"log"

	"github.com/rollbar/rollbar-go"
)

var rollbarEnabled = false

// TrackRecovery logs all panics to rollbar
func TrackRecovery() {
	if rollbarEnabled {
		if err := recover(); err != nil {
			TrackPanic(err)
		}
	}
}

// TrackPanic records the panic error to rollbar
func TrackPanic(err interface{}) {
	if rollbarEnabled {
		log.Printf("recovering from error: %+v\n", err)
		rollbar.LogPanic(err, true)
	}
}

// TrackError records an error
func TrackError(err error) {
	if rollbarEnabled {
		rollbar.Log("error", err)
	}
}

// InitRollbar configures the rollbar tracker
func InitRollbar(cfg *Config) {
	if cfg == nil {
		return
	}
	if cfg.RollbarToken == "" {
		if cfg.AppEnv == modeProduction {
			log.Println("rollbar token is not provided, skipping error tracking")
		}
		return
	}

	rollbarEnabled = true

	rollbar.SetEnabled(true)
	rollbar.SetEnvironment(cfg.AppEnv)
	rollbar.SetCodeVersion(AppVersion)
	rollbar.SetToken(cfg.RollbarToken)
	rollbar.SetServerRoot(cfg.RollbarNamespace)
}
