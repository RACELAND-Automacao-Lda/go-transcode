package transcode

import (
	"os"
	"os/signal"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/m1k1o/go-transcode/internal/api"
	"github.com/m1k1o/go-transcode/internal/config"
	"github.com/m1k1o/go-transcode/internal/server"
)

var Service *Main

func init() {
	Service = &Main{
		ServerConfig: &config.Server{},
	}
}

type Main struct {
	ServerConfig *config.Server

	logger        zerolog.Logger
	apiManager    *api.ApiManagerCtx
	serverManager *server.ServerManagerCtx
}

func (main *Main) Preflight() {
	main.logger = log.With().Str("service", "main").Logger()
}

func (main *Main) Start() {
	config := main.ServerConfig

	main.apiManager = api.New(config)
	main.apiManager.Start()

	main.serverManager = server.New(&config.Server)
	main.serverManager.Mount(main.apiManager.Mount)
	main.serverManager.Start()

	main.logger.Info().Msgf("serving streams from basedir %s: %s", config.BaseDir, config.Streams)
}

func (main *Main) Shutdown() {
	var err error

	err = main.serverManager.Shutdown()
	main.logger.Err(err).Msg("http manager shutdown")

	err = main.apiManager.Shutdown()
	main.logger.Err(err).Msg("api manager shutdown")
}

func (main *Main) ServeCommand(cmd *cobra.Command, args []string) {
	main.logger.Info().Msg("starting main server")
	main.Start()
	main.logger.Info().Msg("main ready")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	sig := <-quit

	main.logger.Warn().Msgf("received %s, attempting graceful shutdown", sig)
	main.Shutdown()
	main.logger.Info().Msg("shutdown complete")
}
