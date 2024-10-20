// Modifications Copyright 2024 The Kaia Authors
// Modifications Copyright 2018 The klaytn Authors
// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.
//
// This file is derived from accounts/keystore/keystore_plain.go (2018/06/04).
// Modified and improved for the klaytn development.
// Modified and improved for the Kaia development.

package keystore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kaiachain/kaia/common"
)

type keyStorePlain struct {
	keysDirPath string
}

func (ks keyStorePlain) GetKey(addr common.Address, filename, auth string) (Key, error) {
	fd, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	var key Key
	keyv4 := new(KeyV4)
	if err := json.NewDecoder(fd).Decode(keyv4); err != nil {
		keyv3 := new(KeyV3)
		if err := json.NewDecoder(fd).Decode(keyv3); err != nil {
			return nil, err
		}
		key = keyv3
	} else {
		key = keyv4
	}
	if key.GetAddress() != addr {
		return nil, fmt.Errorf("key content mismatch: have address %x, want %x", key.GetAddress(), addr)
	}
	return key, nil
}

func (ks keyStorePlain) StoreKey(filename string, key Key, auth string) error {
	content, err := json.Marshal(key)
	if err != nil {
		return err
	}
	return writeKeyFile(filename, content)
}

func (ks keyStorePlain) JoinPath(filename string) string {
	if filepath.IsAbs(filename) {
		return filename
	}
	return filepath.Join(ks.keysDirPath, filename)
}
