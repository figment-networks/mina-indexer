package server

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/figment-networks/mina-indexer/client/graph"
	"github.com/figment-networks/mina-indexer/config"
	"github.com/figment-networks/mina-indexer/model"
	"github.com/figment-networks/mina-indexer/store"
)

// Server handles HTTP requests
type Server struct {
	*gin.Engine

	graphClient *graph.Client
	db          *store.Store
}

// New returns a new server instance
func New(db *store.Store, cfg *config.Config) *Server {
	s := &Server{
		Engine: gin.New(),

		db:          db,
		graphClient: graph.NewDefaultClient(cfg.MinaEndpoint),
	}

	s.initMiddleware(cfg)
	s.initRoutes()

	return s
}

func (s *Server) initRoutes() {
	s.GET("/health", s.GetHealth)
	s.GET("/status", s.GetStatus)
	s.GET("/height", s.GetCurrentHeight)
	s.GET("/block", s.GetCurrentBlock)
	s.GET("/blocks", s.GetBlocks)
	s.GET("/blocks/:id", s.GetBlock)
	s.GET("/blocks/:id/transactions", s.GetBlockTransactions)
	s.GET("/block_times", s.GetBlockTimes)
	s.GET("/block_stats", timeBucketMiddleware(), s.GetBlockStats)
	s.GET("/validators", s.GetValidators)
	s.GET("/validators/:id", s.GetValidator)
	s.GET("/snarkers/", s.GetSnarkers)
	s.GET("/transactions", s.GetTransactions)
	s.GET("/transactions/:id", s.GetTransaction)
	s.GET("/accounts/:id", s.GetAccount)
}

func (s *Server) initMiddleware(cfg *config.Config) {
	s.Use(gin.Recovery())
	s.Use(requestLoggerMiddleware(logrus.StandardLogger()))

	if cfg.IsDevelopment() {
		s.Use(corsMiddleware())
	}

	if cfg.RollbarToken != "" {
		s.Use(rollbarMiddleware())
	}
}

// GetHealth renders the server health status
func (s Server) GetHealth(c *gin.Context) {
	if err := s.db.Test(); err != nil {
		jsonError(c, 500, "unhealthy")
		return
	}
	jsonOk(c, gin.H{"healthy": true})
}

// GetStatus returns the status of the service
func (s Server) GetStatus(c *gin.Context) {
	data := gin.H{
		"app_name":    config.AppName,
		"app_version": config.AppVersion,
		"git_commit":  config.GitCommit,
		"go_version":  config.GoVersion,
		"sync_status": "stale",
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second*5))
	defer cancel()

	daemonStatus, err := s.graphClient.GetDaemonStatus(ctx)
	if err == nil {
		data["node_version"] = daemonStatus.CommitID
		data["node_status"] = daemonStatus.SyncStatus
	} else {
		logrus.WithError(err).Error("node status fetch failed")
		data["node_status_error"] = true
	}

	if block, err := s.db.Blocks.Recent(); err == nil {
		data["last_block_time"] = block.Time
		data["last_block_height"] = block.Height

		if time.Since(block.Time).Minutes() <= 30 {
			data["sync_status"] = "current"
		}
	} else {
		logrus.WithError(err).Error("recent block fetch failed")
	}

	jsonOk(c, data)
}

// GetCurrentHeight returns the current blockchain height
func (s *Server) GetCurrentHeight(c *gin.Context) {
	block, err := s.db.Blocks.Recent()
	if shouldReturn(c, err) {
		return
	}

	jsonOk(c, gin.H{
		"height": block.Height,
		"time":   block.Time,
	})
}

// GetCurrentBlock returns the current blockchain height
func (s *Server) GetCurrentBlock(c *gin.Context) {
	block, err := s.db.Blocks.Recent()
	if shouldReturn(c, err) {
		return
	}
	jsonOk(c, block)
}

// GetBlock returns a single block
func (s *Server) GetBlock(c *gin.Context) {
	var block *model.Block
	var err error

	id := resourceID(c, "id")
	if id.IsNumeric() {
		if id.UInt64() == 0 {
			badRequest(c, errors.New("height must be greater than 0"))
			return
		}
		block, err = s.db.Blocks.FindByHeight(id.UInt64())
	} else {
		block, err = s.db.Blocks.FindByHash(id.String())
	}
	if shouldReturn(c, err) {
		return
	}

	creator, err := s.db.Accounts.FindByPublicKey(block.Creator)
	if err == store.ErrNotFound {
		creator = nil
		err = nil
	}
	if shouldReturn(c, err) {
		return
	}

	transactions, err := s.db.Transactions.ByHeight(block.Height, uint(block.TransactionsCount))
	if shouldReturn(c, err) {
		return
	}

	jobs, err := s.db.Jobs.ByHeight(block.Height)
	if shouldReturn(c, err) {
		return
	}

	jsonOk(c, gin.H{
		"block":        block,
		"creator":      creator,
		"transactions": transactions,
		"snark_jobs":   jobs,
	})
}

func (s *Server) GetBlockTransactions(c *gin.Context) {
	var block *model.Block
	var err error

	id := resourceID(c, "id")
	if id.IsNumeric() {
		if id.UInt64() == 0 {
			badRequest(c, errors.New("height must be greater than 0"))
			return
		}
		block, err = s.db.Blocks.FindByHeight(id.UInt64())
	} else {
		block, err = s.db.Blocks.FindByHash(id.String())
	}
	if shouldReturn(c, err) {
		return
	}

	transactions, err := s.db.Transactions.ByHeight(block.Height, uint(block.TransactionsCount))
	if shouldReturn(c, err) {
		return
	}

	jsonOk(c, transactions)
}

// GetBlocks returns a list of available blocks matching the filter
func (s *Server) GetBlocks(c *gin.Context) {
	search := &store.BlockSearch{}

	if err := c.BindQuery(search); err != nil {
		badRequest(c, err)
		return
	}

	if err := search.Validate(); err != nil {
		badRequest(c, err)
		return
	}

	blocks, err := s.db.Blocks.Search(search)
	if shouldReturn(c, err) {
		return
	}

	jsonOk(c, blocks)
}

// GetBlockTimes returns avg block times info
func (s *Server) GetBlockTimes(c *gin.Context) {
	params := blockTimesParams{}

	if err := c.BindQuery(&params); err != nil {
		badRequest(c, err)
		return
	}
	params.setDefaults()

	result, err := s.db.Blocks.AvgTimes(params.Limit)
	if err != nil {
		badRequest(c, err)
		return
	}

	jsonOk(c, result)
}

// GetBlockStats returns block stats for an interval
func (s *Server) GetBlockStats(c *gin.Context) {
	tb := c.MustGet("timebucket").(timeBucket)
	result, err := s.db.Blocks.Stats(tb.Period, tb.Interval)
	if shouldReturn(c, err) {
		return
	}
	jsonOk(c, result)
}

// GetTransaction returns a single transaction details
func (s *Server) GetTransaction(c *gin.Context) {
	var tran *model.Transaction
	var err error

	id := resourceID(c, "id")
	if id.IsNumeric() {
		tran, err = s.db.Transactions.FindByID(id.Int64())
	} else {
		tran, err = s.db.Transactions.FindByHash(id.String())
	}
	if shouldReturn(c, err) {
		return
	}

	jsonOk(c, tran)
}

// GetValidators rendes all existing validators
func (s *Server) GetValidators(c *gin.Context) {
	validators, err := s.db.Validators.Index()
	if shouldReturn(c, err) {
		return
	}
	jsonOk(c, validators)
}

// GetValidator renders the validator details
func (s *Server) GetValidator(c *gin.Context) {
	validator, err := s.db.Validators.FindByPublicKey(c.Param("id"))
	if shouldReturn(c, err) {
		return
	}

	account, err := s.db.Accounts.FindByPublicKey(c.Param("id"))
	if shouldReturn(c, err) {
		return
	}

	delegations, err := s.db.Accounts.AllByDelegator(validator.PublicKey)
	if shouldReturn(c, err) {
		return
	}

	stats, err := s.db.Stats.ValidatorStats(validator, 90, store.BucketDay)
	if shouldReturn(c, err) {
		return
	}

	jsonOk(c, gin.H{
		"validator":   validator,
		"account":     account,
		"delegations": delegations,
		"stats":       stats,
	})
}

// GetSnarkers renders all existing snarkers
func (s *Server) GetSnarkers(c *gin.Context) {
	snarkers, err := s.db.Snarkers.All()
	if shouldReturn(c, err) {
		return
	}
	jsonOk(c, snarkers)
}

// GetTransactions returns transactions by height
func (s *Server) GetTransactions(c *gin.Context) {
	search := store.TransactionSearch{}
	if err := c.BindQuery(&search); err != nil {
		badRequest(c, err)
		return
	}

	if err := search.Validate(); err != nil {
		badRequest(c, err)
		return
	}

	transactions, err := s.db.Transactions.Search(search)
	if shouldReturn(c, err) {
		return
	}

	jsonOk(c, transactions)
}

// GetAccount returns account for by hash or ID
func (s *Server) GetAccount(c *gin.Context) {
	var (
		acc *model.Account
		err error
	)

	id := resourceID(c, "id")
	if id.IsNumeric() {
		acc, err = s.db.Accounts.FindByID(id.Int64())
	} else {
		acc, err = s.db.Accounts.FindByPublicKey(id.String())
	}
	if shouldReturn(c, err) {
		return
	}

	jsonOk(c, acc)
}
