package repostitory

import (
	"context"
	"strconv"

	"github.com/bianjieai/tibc-sdk-go/packet"

	tibc "github.com/bianjieai/tibc-sdk-go"
	tibcclient "github.com/bianjieai/tibc-sdk-go/client"
	"github.com/bianjieai/tibc-sdk-go/tendermint"
	tibctypes "github.com/bianjieai/tibc-sdk-go/types"
	sdk "github.com/irisnet/core-sdk-go"
	"github.com/irisnet/core-sdk-go/types"
	coretypes "github.com/irisnet/core-sdk-go/types"
	"github.com/tendermint/tendermint/libs/log"
	tenderminttypes "github.com/tendermint/tendermint/proto/tendermint/types"
	tmttypes "github.com/tendermint/tendermint/types"
)

var _ IChain = new(Tendermint)

const EventTypeSendPacket = "send_packet"
const EventTypeWriteAck = "write_acknowledgement"
const EventTypeSendCleanPacket = "send_clean_packet"

type Tendermint struct {
	logger log.Logger

	coreSdk    sdk.Client
	tibcClient tibc.Client
	baseTx     types.BaseTx
	address    string

	chainName             string
	chainType             string
	updateClientFrequency uint64
}

func NewTendermintClient(chainType, chaiName string, updateClientFrequency uint64, config *TerndermintConfig) (*Tendermint, error) {
	cfg, err := coretypes.NewClientConfig(config.RPCAddr, config.GrpcAddr, config.ChainID, config.Options...)
	if err != nil {
		return nil, err
	}
	coreClient := sdk.NewClient(cfg)
	tibcClient := tibc.NewClient(coreClient)

	// import key to core-sdk
	address, err := coreClient.Key.Import(config.Name, config.Password, config.PrivKeyArmor)
	if err != nil {
		return nil, err
	}

	return &Tendermint{
		chainType:             chainType,
		chainName:             chaiName,
		updateClientFrequency: updateClientFrequency,
		logger:                coreClient.BaseClient.Logger(),
		coreSdk:               coreClient,
		tibcClient:            tibcClient,
		baseTx:                config.BaseTx,
		address:               address,
	}, err
}

func (c *Tendermint) GetPackets(height uint64) (*Packets, error) {
	var bizPackets []packet.Packet
	var ackPackets []packet.Packet
	var cleanPackets []packet.Packet

	curHeight := int64(height)
	block, err := c.coreSdk.Block(context.Background(), &curHeight)
	if err != nil {
		return nil, err
	}

	packets := newPackets()

	for _, tx := range block.Block.Txs {
		resultTx, err := c.coreSdk.QueryTx(string(tx.Hash()))
		if err != nil {
			return nil, err
		}
		tmpPacket, err := c.getPacket(EventTypeSendPacket, resultTx)
		if err != nil {
			return nil, err
		}
		bizPackets = append(bizPackets, *tmpPacket)

		// get ack packet
		tmpAckPacket, err := c.getPacket(EventTypeWriteAck, resultTx)
		if err != nil {
			return nil, err
		}
		ackPackets = append(ackPackets, *tmpAckPacket)

		tmpCleanPacket, err := c.getPacket(EventTypeSendCleanPacket, resultTx)
		if err != nil {
			return nil, err
		}
		cleanPackets = append(cleanPackets, *tmpCleanPacket)

	}

	packets.BizPackets = bizPackets
	packets.AckPackets = ackPackets
	packets.CleanPackets = cleanPackets
	return packets, nil
}

func (c *Tendermint) GetProof(chainName string, sequence uint64, height uint64) ([]byte, error) {
	// todo
	key := packet.PacketCommitmentKey(c.chainName, chainName, sequence)
	_, proofBz, _, err := tendermint.QueryTendermintProof(c.coreSdk, int64(height), key)
	if err != nil {
		return nil, err
	}
	return proofBz, nil
}

func (c *Tendermint) RecvPackets(msgs types.Msgs) (types.ResultTx, types.Error) {
	return c.tibcClient.RecvPackets(msgs, c.baseTx)
}

func (c *Tendermint) GetBlockHeader(req *GetBlockHeaderReq) (tibctypes.Header, error) {
	block, err := c.coreSdk.QueryBlock(int64(req.LatestHeight))
	if err != nil {
		return nil, err
	}
	rescommit, err := c.coreSdk.Commit(context.Background(), &block.BlockResult.Height)
	if err != nil {
		return nil, err
	}
	commit := rescommit.Commit
	signedHeader := &tenderminttypes.SignedHeader{
		Header: block.Block.Header.ToProto(),
		Commit: commit.ToProto(),
	}
	validatorSet, err := c.getValidator(int64(req.LatestHeight))
	if err != nil {
		return nil, err

	}
	trustedValidators, err := c.getValidator(int64(req.TrustedHeight))
	if err != nil {
		return nil, err
	}
	// The trusted fields may be nil. They may be filled before relaying messages to a client.
	// The relayer is responsible for querying client and injecting appropriate trusted fields.
	return &tendermint.Header{
		SignedHeader: signedHeader,
		ValidatorSet: validatorSet,
		TrustedHeight: tibcclient.Height{
			RevisionHeight: req.TrustedHeight,
		},
		TrustedValidators: trustedValidators,
	}, nil
}

func (c *Tendermint) GetLightClientState(chainName string) (tibctypes.ClientState, error) {
	return c.tibcClient.GetClientState(chainName)

}

func (c *Tendermint) GetLightClientConsensusState(chainName string, height uint64) (tibctypes.ConsensusState, error) {
	return c.tibcClient.GetConsensusState(chainName, height)

}

func (c *Tendermint) GetStatus() (interface{}, error) {
	return c.coreSdk.Status(context.Background())
}

func (c *Tendermint) GetLatestHeight() (uint64, error) {
	block, err := c.coreSdk.Block(context.Background(), nil)
	if err != nil {
		return 0, err
	}
	var height = block.Block.Height
	return uint64(height), err
}

func (c *Tendermint) GetLightClientDelayHeight(chainName string) (uint64, error) {
	res, err := c.GetLightClientState(chainName)
	if err != nil {
		return 0, err
	}
	return res.GetDelayBlock(), nil
}

func (c *Tendermint) GetLightClientDelayTime(chainName string) (uint64, error) {
	res, err := c.GetLightClientState(chainName)
	if err != nil {
		return 0, err
	}
	return res.GetDelayTime(), nil

}

func (c *Tendermint) UpdateClient(header tibctypes.Header, chainName string) error {
	request := tibctypes.UpdateClientRequest{
		ChainName: chainName,
		Header:    header,
	}
	_, err := c.tibcClient.UpdateClient(request, c.baseTx)
	if err != nil {
		return err
	}
	return nil
}

func (c *Tendermint) GetCommitmentsPacket(chainName string, sequence uint64) (*packet.QueryPacketCommitmentResponse, error) {
	return c.tibcClient.PacketCommitment(chainName, c.chainName, sequence)
}

func (c *Tendermint) UnreceivedCommitmentsPackets(chainName string, sequences []uint64) (*packet.QueryUnreceivedAcksResponse, error) {
	return c.tibcClient.UnreceivedAcks(chainName, c.chainName, sequences)
}

func (c *Tendermint) GetAckPacket(chainName string, sequence uint64) (*packet.QueryPacketAcknowledgementResponse, error) {
	return c.tibcClient.PacketAcknowledgement(chainName, c.chainName, sequence)
}

func (c *Tendermint) GetReceiptPacket(chainName string, sequence uint64) (*packet.QueryPacketReceiptResponse, error) {
	return c.tibcClient.PacketReceipt(chainName, c.chainName, sequence)
}

func (c *Tendermint) ChainName() string {

	return c.chainName
}

func (c *Tendermint) ChainType() string {
	return c.chainType
}

func (c *Tendermint) UpdateClientFrequency() uint64 {
	return c.updateClientFrequency
}

func (c *Tendermint) getValidator(height int64) (*tenderminttypes.ValidatorSet, error) {
	validators, err := c.coreSdk.Validators(context.Background(), &height, nil, nil)
	if err != nil {
		return nil, err
	}
	validatorSet, err := tmttypes.NewValidatorSet(validators.Validators).ToProto()
	if err != nil {
		return nil, err
	}

	return validatorSet, nil
}

func (c *Tendermint) getPacket(typ string, tx types.ResultQueryTx) (*packet.Packet, error) {
	sequenceStr, err := tx.Result.Events.GetValue(typ, "packet_sequence")
	if err != nil {
		return nil, err
	}

	srcChain, err := tx.Result.Events.GetValue(typ, "packet_src_chain")
	if err != nil {
		return nil, err
	}

	dstPort, err := tx.Result.Events.GetValue(typ, "packet_dst_port")
	if err != nil {
		return nil, err
	}

	port, err := tx.Result.Events.GetValue(typ, "packet_port")
	if err != nil {
		return nil, err
	}

	rlyChan, err := tx.Result.Events.GetValue(typ, "packet_relay_channel")
	if err != nil {
		return nil, err
	}

	data, err := tx.Result.Events.GetValue(typ, "packet_data")
	if err != nil {
		return nil, err
	}

	sequence, err := strconv.Atoi(sequenceStr)
	if err != nil {
		return nil, err
	}
	return &packet.Packet{
		Sequence:         uint64(sequence),
		SourceChain:      srcChain,
		DestinationChain: dstPort,
		Port:             port,
		RelayChain:       rlyChan,
		Data:             []byte(data),
	}, nil
}

type TerndermintConfig struct {
	Options      []coretypes.Option
	BaseTx       types.BaseTx
	PrivKeyArmor string
	Name         string
	Password     string

	RPCAddr  string
	GrpcAddr string
	ChainID  string
}

func NewTerndermintConfig() *TerndermintConfig {
	return &TerndermintConfig{}
}
