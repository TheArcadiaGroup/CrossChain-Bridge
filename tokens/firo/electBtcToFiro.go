package firo

import (
	belectrs "github.com/anyswap/CrossChain-Bridge/tokens/btc/electrs"
)

// ToFIROVout convert address in ElectTx to FIRO format
func (b *Bridge) ToFIROVout(vout *belectrs.ElectTxOut) *belectrs.ElectTxOut {
	// ScriptpubkeyAddress
	if vout.ScriptpubkeyAddress == nil {
		return vout
	}
	addr, err := b.ConvertBTCAddress(*vout.ScriptpubkeyAddress, "Main")
	if err != nil {
		return vout
	}
	*vout.ScriptpubkeyAddress = addr.String()
	return vout
}

// ToFIROTx convert address in ElectTx to FIRO format
func (b *Bridge) ToFIROTx(tx *belectrs.ElectTx) *belectrs.ElectTx {
	// Vin Prevout ToFIROVout
	for _, vin := range tx.Vin {
		if vin.Prevout != nil {
			*vin.Prevout = *b.ToFIROVout(vin.Prevout)
		}
	}
	// Vout ToFIROVout
	for _, vout := range tx.Vout {
		*vout = *b.ToFIROVout(vout)
	}
	return tx
}
