package worker

import (
	"container/ring"
	"fmt"
	"math/big"
	"sync"

	"github.com/fsn-dev/crossChain-Bridge/common"
	"github.com/fsn-dev/crossChain-Bridge/log"
	"github.com/fsn-dev/crossChain-Bridge/mongodb"
	"github.com/fsn-dev/crossChain-Bridge/tokens"
)

var (
	swapinSwapStarter  sync.Once
	swapoutSwapStarter sync.Once

	swapRing        *ring.Ring
	swapRingLock    sync.RWMutex
	swapRingMaxSize = 1000
)

func StartSwapJob() error {
	go startSwapinSwapJob()
	go startSwapoutSwapJob()
	return nil
}

func startSwapinSwapJob() error {
	swapinSwapStarter.Do(func() {
		logWorker("swap", "start swapin swap job")
		for {
			res, err := findSwapinsToSwap()
			if err != nil {
				logWorkerError("swapin", "find swapins error", err)
			}
			if len(res) > 0 {
				logWorker("swapin", "find swapins to swap", "count", len(res))
			}
			for _, swap := range res {
				err = processSwapinSwap(swap)
				if err != nil {
					logWorkerError("swapin", "process swapin swap error", err, "txid", swap.TxId)
				}
			}
			restInJob(restIntervalInDoSwapJob)
		}
	})
	return nil
}

func startSwapoutSwapJob() error {
	swapoutSwapStarter.Do(func() {
		logWorker("swapout", "start swapout swap job")
		for {
			res, err := findSwapoutsToSwap()
			if err != nil {
				logWorkerError("swapout", "find swapouts error", err)
			}
			if len(res) > 0 {
				logWorker("swapout", "find swapouts to swap", "count", len(res))
			}
			for _, swap := range res {
				err = processSwapoutSwap(swap)
				if err != nil {
					logWorkerError("swapout", "process swapout swap error", err)
				}
			}
			restInJob(restIntervalInDoSwapJob)
		}
	})
	return nil
}

func findSwapinsToSwap() ([]*mongodb.MgoSwap, error) {
	status := mongodb.TxNotSwapped
	septime := getSepTimeInFind(maxDoSwapLifetime)
	return mongodb.FindSwapinsWithStatus(status, septime)
}

func findSwapoutsToSwap() ([]*mongodb.MgoSwap, error) {
	status := mongodb.TxNotSwapped
	septime := getSepTimeInFind(maxDoSwapLifetime)
	return mongodb.FindSwapoutsWithStatus(status, septime)
}

func processSwapinSwap(swap *mongodb.MgoSwap) (err error) {
	txid := swap.TxId
	bridge := tokens.DstBridge
	log.Debug("start processSwapinSwap", "txid", txid, "status", swap.Status)
	res, err := mongodb.FindSwapinResult(txid)
	if err != nil {
		return err
	}
	if res.SwapTx != "" {
		if res.Status == mongodb.TxNotSwapped {
			mongodb.UpdateSwapinStatus(txid, mongodb.TxProcessed, now(), "")
		}
		return fmt.Errorf("%v already swapped to %v", txid, res.SwapTx)
	}

	history := getSwapHistory(txid, true)
	if history != nil {
		if _, err := bridge.GetTransaction(history.matchTx); err == nil {
			matchTx := &MatchTx{
				SwapTx:   history.matchTx,
				SwapType: tokens.Swap_Swapin,
			}
			updateSwapinResult(txid, matchTx)
			logWorker("swapin", "ignore swapped swapin", "txid", txid, "matchTx", history.matchTx)
			return fmt.Errorf("found swapped in history, txid=%v, matchTx=%v", txid, history.matchTx)
		}
	}

	value, err := common.GetBigIntFromStr(res.Value)
	if err != nil {
		return fmt.Errorf("wrong value %v", res.Value)
	}

	args := &tokens.BuildTxArgs{
		SwapInfo: tokens.SwapInfo{
			SwapID:   res.TxId,
			SwapType: tokens.Swap_Swapin,
		},
		To:    res.Bind,
		Value: value,
	}
	rawTx, err := bridge.BuildRawTransaction(args)
	if err != nil {
		return err
	}
	if rawTx == nil {
		return fmt.Errorf("build raw tx is empty, txid=%v", txid)
	}

	signedTx, txHash, err := bridge.DcrmSignTransaction(rawTx, args.GetExtraArgs())
	if err != nil {
		return err
	}
	if signedTx == nil {
		return fmt.Errorf("signed tx is empty, txid=%v", txid)
	}

	// update database before sending transaction
	addSwapHistory(txid, value, txHash, true)
	err = mongodb.UpdateSwapinStatus(txid, mongodb.TxProcessed, now(), "")
	if err != nil {
		return err
	}
	matchTx := &MatchTx{
		SwapTx:    txHash,
		SwapValue: tokens.CalcSwappedValue(value, bridge.IsSrcEndpoint()).String(),
		SwapType:  tokens.Swap_Swapin,
	}
	err = updateSwapinResult(txid, matchTx)
	if err != nil {
		return err
	}

	_, err = bridge.SendTransaction(signedTx)
	if err != nil {
		logWorkerError("swapin", "update swapin status to TxSwapFailed", err, "txid", txid)
		mongodb.UpdateSwapinStatus(txid, mongodb.TxSwapFailed, now(), "")
		mongodb.UpdateSwapinResultStatus(txid, mongodb.TxSwapFailed, now(), "")
		return err
	}
	return nil
}

func processSwapoutSwap(swap *mongodb.MgoSwap) (err error) {
	txid := swap.TxId
	bridge := tokens.SrcBridge
	log.Debug("start processSwapoutSwap", "txid", txid, "status", swap.Status)
	res, err := mongodb.FindSwapoutResult(txid)
	if err != nil {
		return err
	}
	if res.SwapTx != "" {
		if res.Status == mongodb.TxNotSwapped {
			mongodb.UpdateSwapoutStatus(txid, mongodb.TxProcessed, now(), "")
		}
		return fmt.Errorf("%v already swapped to %v", txid, res.SwapTx)
	}

	history := getSwapHistory(txid, false)
	if history != nil {
		if _, err := bridge.GetTransaction(history.matchTx); err == nil {
			matchTx := &MatchTx{
				SwapTx:   history.matchTx,
				SwapType: tokens.Swap_Swapout,
			}
			updateSwapoutResult(txid, matchTx)
			logWorker("swapout", "ignore swapped swapout", "txid", txid, "matchTx", history.matchTx)
			return fmt.Errorf("found swapped out history, txid=%v, matchTx=%v", txid, history.matchTx)
		}
	}

	value, err := common.GetBigIntFromStr(res.Value)
	if err != nil {
		return fmt.Errorf("wrong value %v", res.Value)
	}

	args := &tokens.BuildTxArgs{
		SwapInfo: tokens.SwapInfo{
			SwapID:   res.TxId,
			SwapType: tokens.Swap_Swapout,
		},
		To:    res.Bind,
		Value: value,
		Memo:  fmt.Sprintf("%s%s", tokens.UnlockMemoPrefix, res.TxId),
	}
	rawTx, err := bridge.BuildRawTransaction(args)
	if err != nil {
		return err
	}
	if rawTx == nil {
		return fmt.Errorf("build raw tx is empty, txid=%v", txid)
	}

	signedTx, txHash, err := bridge.DcrmSignTransaction(rawTx, args.GetExtraArgs())
	if err != nil {
		return err
	}
	if signedTx == nil {
		return fmt.Errorf("signed tx is empty, txid=%v", txid)
	}

	// update database before sending transaction
	addSwapHistory(txid, value, txHash, false)
	err = mongodb.UpdateSwapoutStatus(txid, mongodb.TxProcessed, now(), "")
	if err != nil {
		return err
	}
	matchTx := &MatchTx{
		SwapTx:    txHash,
		SwapValue: tokens.CalcSwappedValue(value, bridge.IsSrcEndpoint()).String(),
		SwapType:  tokens.Swap_Swapout,
	}
	err = updateSwapoutResult(txid, matchTx)
	if err != nil {
		return err
	}

	_, err = bridge.SendTransaction(signedTx)
	if err != nil {
		logWorkerError("swapout", "update swapout status to TxSwapFailed", err, "txid", txid)
		mongodb.UpdateSwapoutStatus(txid, mongodb.TxSwapFailed, now(), "")
		mongodb.UpdateSwapoutResultStatus(txid, mongodb.TxSwapFailed, now(), "")
	}
	return err

}

type swapInfo struct {
	txid     string
	value    *big.Int
	matchTx  string
	isSwapin bool
}

func addSwapHistory(txid string, value *big.Int, matchTx string, isSwapin bool) {
	// Create the new item as its own ring
	item := ring.New(1)
	item.Value = &swapInfo{
		txid:     txid,
		value:    value,
		matchTx:  matchTx,
		isSwapin: isSwapin,
	}

	swapRingLock.Lock()
	defer swapRingLock.Unlock()

	if swapRing == nil {
		swapRing = item
	} else {
		if swapRing.Len() == swapRingMaxSize {
			swapRing = swapRing.Move(-1)
			swapRing.Unlink(1)
			swapRing = swapRing.Move(1)
		}
		swapRing.Move(-1).Link(item)
	}
}

func getSwapHistory(txid string, isSwapin bool) *swapInfo {
	swapRingLock.RLock()
	defer swapRingLock.RUnlock()

	if swapRing == nil {
		return nil
	}

	r := swapRing
	for i := 0; i < r.Len(); i++ {
		item := r.Value.(*swapInfo)
		if item.txid == txid && item.isSwapin == isSwapin {
			return item
		}
		r = r.Prev()
	}

	return nil
}
