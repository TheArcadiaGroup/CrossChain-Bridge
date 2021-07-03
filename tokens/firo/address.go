package firo

import (
	"fmt"

	firoutil "github.com/TheArcadiaGroup/firoutil"
)

// DecodeAddress decode address
func (b *Bridge) DecodeAddress(addr string) (address firoutil.Address, err error) {
	chainConfig := b.GetChainParams()
	address, err = firoutil.DecodeAddress(addr, chainConfig)
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
func (b *Bridge) NewAddressPubKeyHash(pkData []byte) (*firoutil.AddressPubKeyHash, error) {
	return firoutil.NewAddressPubKeyHash(firoutil.Hash160(pkData), b.GetChainParams())
}

// NewAddressScriptHash encap
func (b *Bridge) NewAddressScriptHash(redeemScript []byte) (*firoutil.AddressScriptHash, error) {
	return firoutil.NewAddressScriptHash(redeemScript, b.GetChainParams())
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
	_, ok := address.(*firoutil.AddressPubKeyHash)
	return ok
}

// IsP2shAddress check p2sh addrss
func (b *Bridge) IsP2shAddress(addr string) bool {
	address, err := b.DecodeAddress(addr)
	if err != nil {
		return false
	}
	_, ok := address.(*firoutil.AddressScriptHash)
	return ok
}

// DecodeWIF decode wif
func DecodeWIF(wif string) (*firoutil.WIF, error) {
	return firoutil.DecodeWIF(wif)
}
