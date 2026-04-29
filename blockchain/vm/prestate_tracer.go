// Modifications Copyright 2026 The Kaia Authors
// Copyright 2022 The go-ethereum Authors
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
// This file is derived from eth/tracers/native/prestate.go (go-ethereum 5d52a35).

package vm

import (
	"bytes"
	"encoding/json"
	"math/big"
	"sync/atomic"

	"github.com/kaiachain/kaia/common"
	"github.com/kaiachain/kaia/common/hexutil"
	"github.com/kaiachain/kaia/crypto"
)

var _ Tracer = (*PrestateTracer)(nil)

// PrestateAccount captures account state for the prestate tracer.
//
//go:generate gencodec -type PrestateAccount -field-override prestateAccountMarshaling -out gen_prestate_account_json.go
type PrestateAccount struct {
	Balance *big.Int                    `json:"balance,omitempty"`
	Nonce   uint64                      `json:"nonce,omitempty"`
	Code    []byte                      `json:"code,omitempty"`
	Storage map[common.Hash]common.Hash `json:"storage,omitempty"`
}

type prestateAccountMarshaling struct {
	Balance *hexutil.Big
	Code    hexutil.Bytes
}

type prestateState = map[common.Address]*PrestateAccount

// PrestateTracerConfig is parsed from the JSON tracer config.
type PrestateTracerConfig struct {
	DiffMode bool `json:"diffMode"`
}

// PrestateTracer records the prestate (and optionally the post-tx diff) of all
// accounts and storage slots touched during a transaction.
type PrestateTracer struct {
	env       *EVM
	pre       prestateState
	post      prestateState
	create    bool
	to        common.Address
	gasLimit  uint64 // tx.gasLimit, captured in CaptureTxStart
	config    PrestateTracerConfig
	interrupt atomic.Bool
	reason    error
	created   map[common.Address]bool
	deleted   map[common.Address]bool
}

// NewPrestateTracer constructs a PrestateTracer. cfg may be nil.
func NewPrestateTracer(cfg json.RawMessage) (*PrestateTracer, error) {
	var config PrestateTracerConfig
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &config); err != nil {
			return nil, err
		}
	}
	return &PrestateTracer{
		pre:     prestateState{},
		post:    prestateState{},
		config:  config,
		created: make(map[common.Address]bool),
		deleted: make(map[common.Address]bool),
	}, nil
}

func (t *PrestateTracer) CaptureTxStart(gasLimit uint64) {
	t.gasLimit = gasLimit
}

func (t *PrestateTracer) CaptureTxEnd(restGas uint64) {
	if !t.config.DiffMode {
		return
	}

	for addr, state := range t.pre {
		// Deleted accounts are pruned from the diff entirely.
		if _, ok := t.deleted[addr]; ok {
			continue
		}
		modified := false
		postAccount := &PrestateAccount{Storage: make(map[common.Hash]common.Hash)}
		newBalance := t.env.StateDB.GetBalance(addr)
		newNonce := t.env.StateDB.GetNonce(addr)
		newCode := t.env.StateDB.GetCode(addr)

		if newBalance.Cmp(t.pre[addr].Balance) != 0 {
			modified = true
			postAccount.Balance = newBalance
		}
		if newNonce != t.pre[addr].Nonce {
			modified = true
			postAccount.Nonce = newNonce
		}
		if !bytes.Equal(newCode, t.pre[addr].Code) {
			modified = true
			postAccount.Code = newCode
		}

		for key, val := range state.Storage {
			// Drop empty pre-slots so they don't appear in the pre output unnecessarily.
			if val == (common.Hash{}) {
				delete(t.pre[addr].Storage, key)
			}
			newVal := t.env.StateDB.GetState(addr, key)
			if val != newVal {
				modified = true
				if newVal != (common.Hash{}) {
					postAccount.Storage[key] = newVal
				}
			}
		}

		if modified {
			t.post[addr] = postAccount
		} else {
			// State unchanged: drop from pre as well so the diff shows only changes.
			delete(t.pre, addr)
		}
	}
	// Newly created contracts had no prestate worth capturing.
	for a := range t.created {
		if s, ok := t.pre[a]; ok && s.Balance.Sign() == 0 && len(s.Storage) == 0 && len(s.Code) == 0 {
			delete(t.pre, a)
		}
	}
}

func (t *PrestateTracer) CaptureStart(env *EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
	t.env = env
	t.create = create
	t.to = to

	t.lookupAccount(from)
	t.lookupAccount(to)
	t.lookupAccount(env.Context.Coinbase)

	// At this point the StateDB has already deducted (value + gasLimit*gasPrice)
	// from sender and credited value to the recipient. Reverse those to recover
	// the true pre-state balances.
	if value != nil {
		toBal := new(big.Int).Sub(t.pre[to].Balance, value)
		t.pre[to].Balance = toBal

		fromBal := new(big.Int).Set(t.pre[from].Balance)
		gasPrice := env.TxContext.GasPrice
		consumedGas := new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(t.gasLimit))
		fromBal.Add(fromBal, new(big.Int).Add(value, consumedGas))
		t.pre[from].Balance = fromBal
	} else {
		fromBal := new(big.Int).Set(t.pre[from].Balance)
		gasPrice := env.TxContext.GasPrice
		consumedGas := new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(t.gasLimit))
		fromBal.Add(fromBal, consumedGas)
		t.pre[from].Balance = fromBal
	}
	t.pre[from].Nonce--

	if create && t.config.DiffMode {
		t.created[to] = true
	}
}

func (t *PrestateTracer) CaptureEnd(output []byte, gasUsed uint64, err error) {
	if t.create && !t.config.DiffMode {
		// Match upstream: in non-diff mode, the freshly-created contract has no
		// useful prestate, so drop it.
		delete(t.pre, t.to)
	}
}

func (t *PrestateTracer) CaptureEnter(typ OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
}

func (t *PrestateTracer) CaptureExit(output []byte, gasUsed uint64, err error) {
}

func (t *PrestateTracer) CaptureState(env *EVM, pc uint64, op OpCode, gas, cost, ccLeft, ccOpcode uint64, scope *ScopeContext, depth int, err error) {
	if t.interrupt.Load() {
		return
	}
	stack := scope.Stack
	stackData := stack.Data()
	stackLen := len(stackData)
	caller := scope.Contract.Address()
	switch {
	case stackLen >= 1 && (op == SLOAD || op == SSTORE):
		slot := common.Hash(stackData[stackLen-1].Bytes32())
		t.lookupStorage(caller, slot)
	case stackLen >= 1 && (op == EXTCODECOPY || op == EXTCODEHASH || op == EXTCODESIZE || op == BALANCE || op == SELFDESTRUCT):
		addr := common.Address(stackData[stackLen-1].Bytes20())
		t.lookupAccount(addr)
		if op == SELFDESTRUCT {
			t.deleted[caller] = true
		}
	case stackLen >= 5 && (op == DELEGATECALL || op == CALL || op == STATICCALL || op == CALLCODE):
		addr := common.Address(stackData[stackLen-2].Bytes20())
		t.lookupAccount(addr)
	case op == CREATE:
		nonce := t.env.StateDB.GetNonce(caller)
		addr := crypto.CreateAddress(caller, nonce)
		t.lookupAccount(addr)
		t.created[addr] = true
	case stackLen >= 4 && op == CREATE2:
		offset := stackData[stackLen-2]
		size := stackData[stackLen-3]
		init := scope.Memory.GetCopy(int64(offset.Uint64()), int64(size.Uint64()))
		inithash := crypto.Keccak256(init)
		salt := stackData[stackLen-4]
		addr := crypto.CreateAddress2(caller, salt.Bytes32(), inithash)
		t.lookupAccount(addr)
		t.created[addr] = true
	}
}

func (t *PrestateTracer) CaptureFault(env *EVM, pc uint64, op OpCode, gas, cost, ccLeft, ccOpcode uint64, scope *ScopeContext, depth int, err error) {
}

// GetResult returns the prestate, or in diffMode the {pre, post} pair.
func (t *PrestateTracer) GetResult() (json.RawMessage, error) {
	var (
		res []byte
		err error
	)
	if t.config.DiffMode {
		res, err = json.Marshal(struct {
			Pre  prestateState `json:"pre"`
			Post prestateState `json:"post"`
		}{t.pre, t.post})
	} else {
		res, err = json.Marshal(t.pre)
	}
	if err != nil {
		return nil, err
	}
	return res, t.reason
}

// Stop terminates the tracer at the next opportunity.
func (t *PrestateTracer) Stop(err error) {
	t.reason = err
	t.interrupt.Store(true)
}

func (t *PrestateTracer) lookupAccount(addr common.Address) {
	if _, ok := t.pre[addr]; ok {
		return
	}
	t.pre[addr] = &PrestateAccount{
		Balance: t.env.StateDB.GetBalance(addr),
		Nonce:   t.env.StateDB.GetNonce(addr),
		Code:    t.env.StateDB.GetCode(addr),
		Storage: make(map[common.Hash]common.Hash),
	}
}

// lookupStorage assumes lookupAccount has already populated addr's account.
func (t *PrestateTracer) lookupStorage(addr common.Address, key common.Hash) {
	if _, ok := t.pre[addr].Storage[key]; ok {
		return
	}
	t.pre[addr].Storage[key] = t.env.StateDB.GetState(addr, key)
}
