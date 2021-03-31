package router

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"sync/atomic"
	"time"

	"github.com/anyswap/CrossChain-Bridge/common"
	"github.com/anyswap/CrossChain-Bridge/common/hexutil"
	"github.com/anyswap/CrossChain-Bridge/log"
	"github.com/anyswap/CrossChain-Bridge/params"
	ethereum "github.com/fsn-dev/fsn-go-sdk/efsn"
	ethcommon "github.com/fsn-dev/fsn-go-sdk/efsn/common"
	ethtypes "github.com/fsn-dev/fsn-go-sdk/efsn/core/types"
	"github.com/fsn-dev/fsn-go-sdk/efsn/ethclient"
)

var (
	routerConfigContract ethcommon.Address
	routerConfigClients  []*ethclient.Client
	routerConfigCtx      = context.Background()

	channels   = make([]chan ethtypes.Log, 0, 3)
	subscribes = make([]ethereum.Subscription, 0, 3)

	// topic of event 'UpdateConfig()'
	updateConfigTopic = ethcommon.HexToHash("0x22590461e7ba17e1fe7580cb0ea47f283d3b2248f04873dfbe926d08fe4c5ab9")

	latestUpdateIDBlock uint64
)

// InitRouterConfigClients init router config clients
func InitRouterConfigClients() {
	onchainCfg := params.GetRouterConfig().Onchain
	InitRouterConfigClientsWithArgs(onchainCfg.Contract, onchainCfg.APIAddress)
}

// InitRouterConfigClientsWithArgs init standalone
func InitRouterConfigClientsWithArgs(configContract string, gateways []string) {
	var err error
	routerConfigContract = ethcommon.HexToAddress(configContract)
	routerConfigClients = make([]*ethclient.Client, len(gateways))
	for i, gateway := range gateways {
		routerConfigClients[i], err = ethclient.Dial(gateway)
		if err != nil {
			log.Fatal("init router config clients failed", "gateway", gateway, "err", err)
		}
	}
}

// CallOnchainContract call onchain contract
func CallOnchainContract(data hexutil.Bytes, blockNumber string) (result []byte, err error) {
	msg := ethereum.CallMsg{
		To:   &routerConfigContract,
		Data: data,
	}
	for _, cli := range routerConfigClients {
		result, err = cli.CallContract(routerConfigCtx, msg, nil)
		if err == nil {
			return result, nil
		}
	}
	log.Debug("call onchain contract error", "contract", routerConfigContract.String(), "data", data, "err", err)
	return nil, err
}

// SubscribeUpdateID subscribe update ID and reload configs
func SubscribeUpdateID() {
	SubscribeRouterConfig([]ethcommon.Hash{updateConfigTopic})
	for _, ch := range channels {
		go processUpdateID(ch)
	}
}

func processUpdateID(ch <-chan ethtypes.Log) {
	for {
		rlog := <-ch

		// sleep random in a second to mess steps
		rNum, _ := rand.Int(rand.Reader, big.NewInt(1000))
		time.Sleep(time.Duration(rNum.Uint64()) * time.Millisecond)

		blockNumber := rlog.BlockNumber
		oldBlock := atomic.LoadUint64(&latestUpdateIDBlock)
		if blockNumber > oldBlock {
			atomic.StoreUint64(&latestUpdateIDBlock, blockNumber)
			ReloadRouterConfig()
		}
	}
}

// SubscribeRouterConfig subscribe router config
func SubscribeRouterConfig(topics []ethcommon.Hash) {
	fq := ethereum.FilterQuery{
		Addresses: []ethcommon.Address{routerConfigContract},
		Topics:    [][]ethcommon.Hash{topics},
	}
	for i, cli := range routerConfigClients {
		ch := make(chan ethtypes.Log)
		sub, err := cli.SubscribeFilterLogs(routerConfigCtx, fq, ch)
		if err != nil {
			log.Error("subscribe 'UpdateConfig' event failed", "index", i, "err", err)
			continue
		}
		channels = append(channels, ch)
		subscribes = append(subscribes, sub)
	}
	log.Info("subscribe 'UpdateConfig' event finished", "subscribes", len(subscribes))
}

func parseChainConfig(data []byte) (config *ChainConfig, err error) {
	offset, overflow := common.GetUint64(data, 0, 32)
	if overflow {
		return nil, errParseDataError
	}
	if uint64(len(data)) < offset+11*32 {
		return nil, errParseDataError
	}
	data = data[32:]
	config = &ChainConfig{}
	config.BlockChain, err = ParseStringInData(data, 0)
	if err != nil {
		return nil, errParseDataError
	}
	config.RouterContract = common.BytesToAddress(common.GetData(data, 32, 32)).String()
	config.Confirmations = common.GetBigInt(data, 64, 32).Uint64()
	config.InitialHeight = common.GetBigInt(data, 96, 32).Uint64()
	config.WaitTimeToReplace = common.GetBigInt(data, 128, 32).Int64()
	config.MaxReplaceCount = int(common.GetBigInt(data, 160, 32).Int64())
	config.SwapDeadlineOffset = common.GetBigInt(data, 192, 32).Int64()
	config.PlusGasPricePercentage = common.GetBigInt(data, 224, 32).Uint64()
	config.MaxGasPriceFluctPercent = common.GetBigInt(data, 256, 32).Uint64()
	config.DefaultGasLimit = common.GetBigInt(data, 288, 32).Uint64()
	return config, nil
}

// GetChainConfig abi
func GetChainConfig(chainID *big.Int) (*ChainConfig, error) {
	if chainID == nil || chainID.Sign() == 0 {
		return nil, errors.New("chainID is zero")
	}
	funcHash := common.FromHex("0x19ed16dc")
	data := PackDataWithFuncHash(funcHash, chainID)
	res, err := CallOnchainContract(data, "latest")
	if err != nil {
		return nil, err
	}
	config, err := parseChainConfig(res)
	if err != nil {
		return nil, err
	}
	config.ChainID = chainID.String()
	return config, nil
}

func parseTokenConfig(data []byte) (config *TokenConfig, err error) {
	if uint64(len(data)) < 9*32 {
		return nil, errParseDataError
	}
	decimals := uint8(common.GetBigInt(data, 0, 32).Uint64())
	contractAddress := common.BytesToAddress(common.GetData(data, 32, 32)).String()
	contractVersion := common.GetBigInt(data, 64, 32).Uint64()
	maximumSwap := common.GetBigInt(data, 96, 32)
	minimumSwap := common.GetBigInt(data, 128, 32)
	bigValueThreshold := common.GetBigInt(data, 160, 32)
	swapFeeRatePerMillion := common.GetBigInt(data, 192, 32).Uint64()
	maximumSwapFee := common.GetBigInt(data, 224, 32)
	minimumSwapFee := common.GetBigInt(data, 256, 32)
	config = &TokenConfig{
		Decimals:              decimals,
		ContractAddress:       contractAddress,
		ContractVersion:       contractVersion,
		MaximumSwap:           maximumSwap,
		MinimumSwap:           minimumSwap,
		BigValueThreshold:     bigValueThreshold,
		SwapFeeRatePerMillion: swapFeeRatePerMillion,
		MaximumSwapFee:        maximumSwapFee,
		MinimumSwapFee:        minimumSwapFee,
	}
	return config, err
}

func getTokenConfig(funcHash []byte, chainID *big.Int, token string) (*TokenConfig, error) {
	data := PackDataWithFuncHash(funcHash, token, chainID)
	res, err := CallOnchainContract(data, "latest")
	if err != nil {
		return nil, err
	}
	config, err := parseTokenConfig(res)
	if err != nil {
		return nil, err
	}
	config.TokenID = token
	return config, nil
}

// GetTokenConfig abi
func GetTokenConfig(chainID *big.Int, token string) (tokenCfg *TokenConfig, err error) {
	funcHash := common.FromHex("0x459511d1")
	return getTokenConfig(funcHash, chainID, token)
}

// GetUserTokenConfig abi
func GetUserTokenConfig(chainID *big.Int, token string) (tokenCfg *TokenConfig, err error) {
	funcHash := common.FromHex("0x2879196f")
	return getTokenConfig(funcHash, chainID, token)
}

// GetCustomConfig abi
func GetCustomConfig(chainID *big.Int, key string) (string, error) {
	funcHash := common.FromHex("0x61387d61")
	data := PackDataWithFuncHash(funcHash, chainID, key)
	res, err := CallOnchainContract(data, "latest")
	if err != nil {
		return "", err
	}
	return ParseStringInData(res, 0)
}

// GetMPCPubkey abi
func GetMPCPubkey(mpcAddress string) (pubkey string, err error) {
	funcHash := common.FromHex("0x58bb97fb")
	data := PackDataWithFuncHash(funcHash, common.HexToAddress(mpcAddress))
	res, err := CallOnchainContract(data, "latest")
	if err != nil {
		return "", err
	}
	return ParseStringInData(res, 0)
}

// IsChainIDExist abi
func IsChainIDExist(chainID *big.Int) (exist bool, err error) {
	funcHash := common.FromHex("0xfd15ea70")
	data := PackDataWithFuncHash(funcHash, chainID)
	res, err := CallOnchainContract(data, "latest")
	if err != nil {
		return false, err
	}
	return common.GetBigInt(res, 0, 32).Sign() != 0, nil
}

// IsTokenIDExist abi
func IsTokenIDExist(tokenID string) (exist bool, err error) {
	funcHash := common.FromHex("0xaf611ca0")
	data := PackDataWithFuncHash(funcHash, tokenID)
	res, err := CallOnchainContract(data, "latest")
	if err != nil {
		return false, err
	}
	return common.GetBigInt(res, 0, 32).Sign() != 0, nil
}

// GetAllChainIDs abi
func GetAllChainIDs() (chainIDs []*big.Int, err error) {
	funcHash := common.FromHex("0xe27112d5")
	res, err := CallOnchainContract(funcHash, "latest")
	if err != nil {
		return nil, err
	}
	return ParseNumberSliceAsBigIntsInData(res, 0)
}

// GetAllTokenIDs abi
func GetAllTokenIDs() (tokenIDs []string, err error) {
	funcHash := common.FromHex("0x684a10b3")
	res, err := CallOnchainContract(funcHash, "latest")
	if err != nil {
		return nil, err
	}
	return ParseStringSliceInData(res, 0)
}

// GetMultichainToken abi
func GetMultichainToken(tokenID string, chainID *big.Int) (tokenAddr string, err error) {
	funcHash := common.FromHex("0xb735ab5a")
	data := PackDataWithFuncHash(funcHash, tokenID, chainID)
	res, err := CallOnchainContract(data, "latest")
	if err != nil {
		return "", err
	}
	return common.BigToAddress(common.GetBigInt(res, 0, 32)).String(), nil
}

// MultichainToken struct
type MultichainToken struct {
	ChainID      *big.Int
	TokenAddress string
}

func parseMultichainTokens(data []byte) (mcTokens []MultichainToken, err error) {
	offset, overflow := common.GetUint64(data, 0, 32)
	if overflow {
		return nil, errParseDataError
	}
	length, overflow := common.GetUint64(data, offset, 32)
	if overflow {
		return nil, errParseDataError
	}
	if uint64(len(data)) < offset+32+length*64 {
		return nil, errParseDataError
	}
	mcTokens = make([]MultichainToken, length)
	data = data[offset+32:]
	for i := uint64(0); i < length; i++ {
		mcTokens[i].ChainID = common.GetBigInt(data, i*64, 32)
		mcTokens[i].TokenAddress = common.BytesToAddress(common.GetData(data, i*64+32, 32)).String()
	}
	return mcTokens, nil
}

// GetAllMultichainTokens abi
func GetAllMultichainTokens(tokenID string) ([]MultichainToken, error) {
	funcHash := common.FromHex("0x8fcb62a3")
	data := PackDataWithFuncHash(funcHash, tokenID)
	res, err := CallOnchainContract(data, "latest")
	if err != nil {
		return nil, err
	}
	return parseMultichainTokens(res)
}