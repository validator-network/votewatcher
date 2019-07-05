package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/types"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
)

var (
	tendermintURL           string
	validatorNetworkAddress string
	ticker                  *time.Ticker
	prometheusURL           string

	// https://prometheus.io/docs/concepts/metric_types/
	latestVotedBlock = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gaia_validator_latest_block_vote",
		Help: "Height of the latest block that was voted on by the validator",
	})
)

func startSubscription() (<-chan ctypes.ResultEvent, context.CancelFunc) {
	fmt.Println("Contacting Gaia at", tendermintURL)
	httpClient := client.NewHTTP(tendermintURL, "/websocket") // TODO Make second parameter configurable with a default value
	if err := httpClient.OnStart(); err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	eventChannel, err := httpClient.WSEvents.Subscribe(ctx, "", "tm.event = 'NewBlock'")

	if err != nil {
		fmt.Println("Unable to start subscription", err)
		panic(err)
	}

	return eventChannel, cancel
}

func processBlocks(events <-chan ctypes.ResultEvent) {
	for e := range events {
		switch e.Data.(type) {
		case types.EventDataNewBlock:
			block := e.Data.(types.EventDataNewBlock).Block
			checkForVote(block)
		default:
			fmt.Printf("Unknown message received %T\n%v\n", e, e)
		}
	}
}

func checkForVote(block *types.Block) {
	for _, vote := range block.LastCommit.Precommits {
		if vote == nil {
			continue
		}

		if vote.ValidatorAddress.String() == validatorNetworkAddress {
			fmt.Println("Validator has voted at height", vote.Height)
			latestVotedBlock.Set(float64(vote.Height))
			return
		}
	}

	fmt.Println("Unable to find validator vote at height", block.Height)
}

func readConfig() {
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	viper.SetConfigType("yaml")

	viper.SetDefault("prometheusURL", "[::]:26661")

	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("fatal error config file: %s", err))
	}

	validatorNetworkAddress = viper.GetString("validatorNetworkAddress")
	tendermintURL = viper.GetString("tendermintURL")
	prometheusURL = viper.GetString("prometheusURL")
}

func init() {
	prometheus.MustRegister(latestVotedBlock)
}

func main() {
	readConfig()

	blocks, cancel := startSubscription()
	defer cancel()
	go processBlocks(blocks)

	http.Handle("/metrics", promhttp.Handler())
	fmt.Println("Serving Prometheus requests at", prometheusURL)
	err := http.ListenAndServe(prometheusURL, nil)
	if err != nil {
		panic(err)
	}
}
