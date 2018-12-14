package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"github.com/tendermint/tendermint/libs/pubsub/query"
	"github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tendermint/types"
)

var (
	url                     string
	validatorNetworkAddress string
	ticker                  *time.Ticker

	// https://prometheus.io/docs/concepts/metric_types/
	latestVotedBlock = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gaia_validator_latest_block_vote",
		Help: "Height of the latest block that was voted on by the validator",
	})
)

func startSubscription() (chan interface{}, context.CancelFunc) {
	httpClient := client.NewHTTP(url, "/websocket") // TODO Make second parameter configurable with a default value
	if err := httpClient.OnStart(); err != nil {
		panic(err)
	}

	query := query.MustParse("tm.event = 'NewBlock'")

	blocks := make(chan interface{}, 10)

	ctx, cancel := context.WithCancel(context.Background())
	if err := httpClient.Subscribe(ctx, "", query, blocks); err != nil {
		fmt.Println("Unable to start subscription", err)
		panic(err)
	}

	return blocks, cancel
}

func processBlocks(blocks chan interface{}) {
	for e := range blocks {
		switch e.(type) {
		case types.EventDataNewBlock:
			block := e.(types.EventDataNewBlock).Block
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
}

func readConfig() {
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("fatal error config file: %s", err))
	}

	validatorNetworkAddress = viper.GetString("validatorNetworkAddress")
	url = viper.GetString("url")
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
	http.ListenAndServe("localhost:8080", nil)
}
