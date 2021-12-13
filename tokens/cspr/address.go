package eth

import (
	"time"
)

// IsValidAddress check address
func (b *Bridge) IsValidAddress(address string) bool {
	return true
}

// IsContractAddress is contract address
func (b *Bridge) IsContractAddress(address string) (bool, error) {
	var code []byte
	var err error
	for i := 0; i < retryRPCCount; i++ {
		code, err = b.GetCode(address)
		if err == nil {
			return len(code) > 1, nil // unexpect RSK getCode return 0x00
		}
		time.Sleep(retryRPCInterval)
	}
	return false, err
}
