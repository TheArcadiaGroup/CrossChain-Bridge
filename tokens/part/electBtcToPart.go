package part

import (
	belectrs "github.com/anyswap/CrossChain-Bridge/tokens/btc/electrs"
)

// ToPARTVout convert address in ElectTx to PART format
func (b *Bridge) ToPARTVout(vout *belectrs.ElectTxOut) *belectrs.ElectTxOut {
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

// ToPARTTx convert address in ElectTx to PART format
func (b *Bridge) ToPARTTx(tx *belectrs.ElectTx) *belectrs.ElectTx {
	// Vin Prevout ToPARTVout
	for _, vin := range tx.Vin {
		if vin.Prevout != nil {
			*vin.Prevout = *b.ToPARTVout(vin.Prevout)
		}
	}
	// Vout ToPARTVout
	for _, vout := range tx.Vout {
		*vout = *b.ToPARTVout(vout)
	}
	return tx
}
