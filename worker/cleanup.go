package worker

import (
	"github.com/figment-networks/mina-indexer/config"
	"github.com/figment-networks/mina-indexer/store"
)

func RunCleanup(cfg *config.Config, db *store.Store) error {
	// Nothing to cleanup right now!
	return nil
}
