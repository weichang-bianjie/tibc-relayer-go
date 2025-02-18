package initialization

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"

	log "github.com/sirupsen/logrus"

	"github.com/bianjieai/tibc-relayer-go/internal/app/relayer/repostitory"
	"github.com/bianjieai/tibc-relayer-go/internal/app/relayer/services/channels"
	"github.com/bianjieai/tibc-relayer-go/internal/pkg/configs"
	"github.com/bianjieai/tibc-relayer-go/internal/pkg/types/cache"
	metricsmodel "github.com/bianjieai/tibc-relayer-go/internal/pkg/types/mertics"
	"github.com/bianjieai/tibc-relayer-go/tools"
)

const TendermintAndTendermint = "tendermint_and_tendermint"
const TendermintAndETH = "tendermint_and_eth"

const TypSource = "source"
const TypDest = "dest"

func ChannelMap(cfg *configs.Config, logger *log.Logger) map[string]channels.IChannel {
	if len(cfg.App.ChannelTypes) != 1 {
		logger.Fatal("channel_types should be equal 1")
	}
	for _, channelType := range cfg.App.ChannelTypes {
		switch channelType {
		case TendermintAndTendermint:
			sourceChain := tendermintChain(&cfg.Chain.Source, logger)
			destChain := tendermintChain(&cfg.Chain.Dest, logger)
			return channelMap(cfg, sourceChain, destChain, logger)
		case TendermintAndETH:
			sourceChain := tendermintChain(&cfg.Chain.Source, logger)
			destChain := ethChain(&cfg.Chain.Dest, logger)
			return channelMap(cfg, sourceChain, destChain, logger)
		default:
			logger.WithFields(log.Fields{
				"channel_type": channelType,
			}).Fatal("channel type does not exist")
		}
	}
	return nil
}

func channelMap(cfg *configs.Config, sourceChain, destChain repostitory.IChain, logger *log.Logger) map[string]channels.IChannel {
	// init source chain channel
	sourceChannel := channel(cfg, sourceChain, destChain, TypSource, logger)

	// add error_handler mw
	sourceChannel = channels.NewWriterMW(
		sourceChannel, sourceChain.ChainName(), logger,
		tools.DefaultHomePath, tools.DefaultCacheDirName, cfg.Chain.Source.Cache.Filename,
	)

	// add metric mw
	metricsModel := metricsmodel.NewMetric(sourceChain.ChainName())
	sourceChannel = channels.NewMetricMW(sourceChannel, metricsModel)

	// init dest chain channel
	destChannel := channel(cfg, destChain, sourceChain, TypDest, logger)

	// add error_handler mw
	destChannel = channels.NewWriterMW(
		destChannel, destChain.ChainName(), logger,
		tools.DefaultHomePath, tools.DefaultCacheDirName, cfg.Chain.Dest.Cache.Filename,
	)

	// add metric mw

	destChannel = channels.NewMetricMW(destChannel, metricsModel)
	channelMap := map[string]channels.IChannel{}
	channelMap[sourceChain.ChainName()] = sourceChannel
	//if cfg.Chain.Dest.Eth.ChainName == "" {
	//	// todo
	//	// The process of eth -> tendermint has not been implemented yet
	//	channelMap[destChain.ChainName()] = destChannel
	//}
	channelMap[destChain.ChainName()] = destChannel

	return channelMap
}

func channel(cfg *configs.Config, sourceChain, destChain repostitory.IChain, typ string, logger *log.Logger) channels.IChannel {

	var channel channels.IChannel
	var filename string
	switch typ {
	case TypSource:
		filename = path.Join(tools.DefaultHomePath, tools.DefaultCacheDirName, cfg.Chain.Source.Cache.Filename)
	case TypDest:
		filename = path.Join(tools.DefaultHomePath, tools.DefaultCacheDirName, cfg.Chain.Dest.Cache.Filename)
	}

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		// If the file does not exist, the initial height is the startHeight in the configuration
		switch typ {
		case TypSource:
			channel = channels.NewChannel(sourceChain, destChain, cfg.Chain.Source.Cache.StartHeight, logger)
		case TypDest:
			channel = channels.NewChannel(sourceChain, destChain, cfg.Chain.Dest.Cache.StartHeight, logger)
		}

	} else {
		// If the file exists, the initial height is the latest_height in the file
		file, err := os.Open(filename)
		if err != nil {
			logger.Fatal("read cache file err: ", err)
		}
		defer file.Close()

		content, err := ioutil.ReadAll(file)
		if err != nil {
			logger.Fatal("read cache file err: ", err)
		}

		cacheData := &cache.Data{}
		err = json.Unmarshal(content, cacheData)
		if err != nil {
			logger.Fatal("read cache file unmarshal err: ", err)
		}
		channel = channels.NewChannel(sourceChain, destChain, cacheData.LatestHeight, logger)
	}

	return channel
}
