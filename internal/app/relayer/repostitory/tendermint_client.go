package repostitory

import (
	"context"

	sdk "github.com/irisnet/core-sdk-go"
	"github.com/irisnet/core-sdk-go/types"
)

var _ IChain = new(TendermintClient)

type TendermintClient struct {
	sdk.Client

	delay     uint64
	chainName string
}
type Config struct {
	Options []types.Option

	Delay    uint64
	NodeURI  string
	GrpcAddr string
	ChainID  string
}

func NewTendermintClient(chaiName string, config Config) (*TendermintClient, error) {
	cfg, err := types.NewClientConfig(config.NodeURI, config.GrpcAddr, config.ChainID, config.Options...)
	if err != nil {
		return nil, err
	}
	return &TendermintClient{
		delay:     config.Delay,
		chainName: chaiName,
		Client:    sdk.NewClient(cfg),
	}, err
}

func (c *TendermintClient) GetBlockAndPackets(height uint64) (interface{}, error) {
	a := int64(height)
	return c.Client.Block(context.Background(), &a)
}

func (c *TendermintClient) GetBlockHeader(height uint64) (interface{}, error) {
	tmp := int64(height)
	block, err := c.Client.Block(context.Background(), &tmp)
	header := block.Block.Header
	return header, err
}

func (c *TendermintClient) GetLightClientState(chainName string) (interface{}, error) {
	//status(context.Background(),chainName)
	return c.Client.Status(context.Background())
}

func (c *TendermintClient) GetLightClientConsensusState(chainName string, height uint64) (interface{}, error) {
	//status(context.Background(),chainName)
	var tmp = int64(height)
	return c.Client.ConsensusParams(context.Background(), &tmp)
}

func (c *TendermintClient) GetStatus() (interface{}, error) {
	return c.Client.Status(context.Background())
}

func (c *TendermintClient) GetLatestHeight() (uint64, error) {
	block, err := c.Client.Block(context.Background(), nil)
	var height = block.Block.Height
	return uint64(height), err
}

func (c *TendermintClient) Delay() uint64 {
	//c.Client.Block()
	return c.delay
}

func (c *TendermintClient) ChainName() string {
	//c.Client.Block()
	return c.chainName
}
