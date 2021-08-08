package part

import (
	"fmt"

	"github.com/giangnamnabka/partutil"
)

// DecodeAddress decode address
func (b *Bridge) DecodeAddress(addr string) (address partutil.Address, err error) {
	chainConfig := b.GetChainParams()
	address, err = partutil.DecodeAddress(addr, chainConfig)
	if err != nil {
		return
	}
	if !address.IsForNet(chainConfig) {
		err = fmt.Errorf("invalid address for net")
		return
	}
	return
}

// NewAddressPubKeyHash encap
func (b *Bridge) NewAddressPubKeyHash(pkData []byte) (*partutil.AddressPubKeyHash, error) {
	return partutil.NewAddressPubKeyHash(partutil.Hash160(pkData), b.GetChainParams())
}

// NewAddressScriptHash encap
func (b *Bridge) NewAddressScriptHash(redeemScript []byte) (*partutil.AddressScriptHash, error) {
	return partutil.NewAddressScriptHash(redeemScript, b.GetChainParams())
}

// IsValidAddress check address
func (b *Bridge) IsValidAddress(addr string) bool {
	_, err := b.DecodeAddress(addr)
	return err == nil
}

// IsP2pkhAddress check p2pkh addrss
func (b *Bridge) IsP2pkhAddress(addr string) bool {
	address, err := b.DecodeAddress(addr)
	if err != nil {
		return false
	}
	_, ok := address.(*partutil.AddressPubKeyHash)
	return ok
}

// IsP2shAddress check p2sh addrss
func (b *Bridge) IsP2shAddress(addr string) bool {
	address, err := b.DecodeAddress(addr)
	if err != nil {
		return false
	}
	_, ok := address.(*partutil.AddressScriptHash)
	return ok
}

// DecodeWIF decode wif
func DecodeWIF(wif string) (*partutil.WIF, error) {
	return partutil.DecodeWIF(wif)
}
