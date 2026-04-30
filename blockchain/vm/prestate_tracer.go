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
// Pre/post reconstruction is moved to CaptureTxEnd because Kaia fires
// CaptureStart earlier in vm.Call/Create than upstream geth: at that hook
// the recipient credit, top-level value transfer, and (for CREATE) the
// caller nonce++ have not yet been applied. Reading the post-tx StateDB and
// reversing gasUsed*gasPrice + value yields correct pre-state without
// having to track which mutations Kaia has or hasn't done at hook time.

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
	env *EVM

	// Captured at CaptureStart — actual pre/post reconstruction is deferred to
	// CaptureTxEnd, where the StateDB has its final post-tx state.
	from   common.Address
	to     common.Address
	value  *big.Int // may be nil
	create bool

	// Synthetic balance the caller (e.g. debug_traceCall) credited to a sender
	// to bypass the EVM's balance check. The tracer subtracts this from the
	// reconstructed pre-balance so prestate reflects the real pre-tx state, not
	// the simulator's top-up.
	synthAddr   common.Address
	synthAmount *big.Int

	pre       prestateState
	post      prestateState
	gasLimit  uint64 // tx.gasLimit, captured in CaptureTxStart
	config    PrestateTracerConfig
	interrupt atomic.Bool
	reason    error
	created   map[common.Address]bool
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
	}, nil
}

func (t *PrestateTracer) CaptureTxStart(gasLimit uint64) {
	t.gasLimit = gasLimit
}

// SetSyntheticBalance records a synthetic balance the caller added to an
// account before tracing (e.g. debug_traceCall topping up an underfunded
// sender). At CaptureTxEnd the tracer will subtract this amount from the
// reconstructed pre-balance so prestate output reflects the real pre-tx
// state rather than the simulator's top-up.
func (t *PrestateTracer) SetSyntheticBalance(addr common.Address, amount *big.Int) {
	t.synthAddr = addr
	if amount != nil {
		t.synthAmount = new(big.Int).Set(amount)
	}
}

// CaptureTxEnd fires after gas refund and the fee transfer to coinbase/rewardbase,
// so the StateDB is in its final post-tx state. We reconstruct the sender's, the
// recipient's, and the fee recipient's pre-tx balances by reversing the deltas.
func (t *PrestateTracer) CaptureTxEnd(restGas uint64) {
	if t.env == nil {
		return // CaptureStart was never called (e.g. malformed tx)
	}
	gasUsed := new(big.Int).SetUint64(t.gasLimit - restGas)
	gasCost := new(big.Int).Mul(gasUsed, t.env.TxContext.GasPrice)
	value := t.value
	if value == nil {
		value = new(big.Int)
	}

	// Sender: read the post-tx StateDB directly (NOT via lazy lookupAccount,
	// which may have stored a mid-execution snapshot if an opcode such as
	// BALANCE(from) touched the sender during execution). Reverse the
	// value + gasCost debit to recover the pre-tx balance, then back out any
	// synthetic credit added by debug_traceCall. Nonce was incremented once
	// for this tx (in TxInternalDataLegacy.Execute for CALL; in vm.Create for
	// top-level CREATE), so subtract one.
	if _, ok := t.pre[t.from]; !ok {
		t.pre[t.from] = &PrestateAccount{Storage: make(map[common.Hash]common.Hash)}
	}
	fromAcc := t.pre[t.from]
	postFromBal := t.env.StateDB.GetBalance(t.from)
	postFromNonce := t.env.StateDB.GetNonce(t.from)
	fromAcc.Balance = new(big.Int).Add(postFromBal, new(big.Int).Add(value, gasCost))
	if t.synthAmount != nil && t.from == t.synthAddr {
		fromAcc.Balance.Sub(fromAcc.Balance, t.synthAmount)
	}
	if postFromNonce > 0 {
		fromAcc.Nonce = postFromNonce - 1
	}
	fromAcc.Code = t.env.StateDB.GetCode(t.from)

	// Recipient: only meaningful for CALL (no pre-state for a contract created
	// in this tx). Force a post-tx read for the same reason as the sender.
	if !t.create {
		if _, ok := t.pre[t.to]; !ok {
			t.pre[t.to] = &PrestateAccount{Storage: make(map[common.Hash]common.Hash)}
		}
		toAcc := t.pre[t.to]
		postToBal := t.env.StateDB.GetBalance(t.to)
		toAcc.Balance = new(big.Int).Sub(postToBal, value)
		toAcc.Nonce = t.env.StateDB.GetNonce(t.to)
		toAcc.Code = t.env.StateDB.GetCode(t.to)
	}

	// Note: we deliberately do not snapshot the fee recipient (Coinbase /
	// Rewardbase) here. By the time CaptureTxEnd runs, the StateDB has already
	// credited gasUsed*gasPrice to that account, and we have no way to read the
	// pre-tx balance retroactively. The JS prestate tracer has the same gap;
	// users who need the proposer's prestate can query eth_getBalance at the
	// parent block. In diffMode the fee recipient still appears in `post` if
	// the contract execution touched it.

	if t.create && t.config.DiffMode {
		t.created[t.to] = true
		// Replace any entry the lazy lookup may have stored for the new
		// contract's address. If the constructor touched its own account
		// (BALANCE, SSTORE, SLOAD, etc.), CaptureState already populated
		// t.pre[t.to] from a *post-create* StateDB read — that reflects
		// nonce=1, the post-Transfer balance, and any storage slot already
		// written, none of which are the real prestate.
		//
		// Reconstruct from scratch: Kaia (post-Shanghai) allows CREATE to
		// proceed over an address that already has balance as long as nonce,
		// code, and storage are empty, and the StateDB carries that prefund
		// into the new contract. So the only field that could have nonzero
		// prestate is balance = (post-tx balance - value); nonce/code/storage
		// were empty pre-tx by definition or the creation would have
		// collided. When the prefund is zero the diff pruning pass at the
		// end of CaptureTxEnd drops this empty entry.
		prefund := new(big.Int).Sub(t.env.StateDB.GetBalance(t.to), value)
		t.pre[t.to] = &PrestateAccount{
			Balance: prefund,
			Storage: make(map[common.Hash]common.Hash),
		}
	}

	if !t.config.DiffMode {
		if t.create {
			// In non-diff mode, the freshly-created contract has no useful prestate.
			delete(t.pre, t.to)
		}
		return
	}

	// diffMode: walk pre and build post by comparing against the post-tx StateDB.
	for addr, preAcc := range t.pre {
		// If the account was destroyed by SELFDESTRUCT (and the destruction was
		// not rolled back), drop it from the diff entirely. Use HasSelfDestructed
		// rather than !Exist: StateDB.Exist returns true for selfdestructed
		// accounts because the state object lingers until commit, and the
		// journal-aware HasSelfDestructed correctly reports `false` for
		// readOnly aborts, outer reverts, and EIP-6780 noops on pre-existing
		// accounts.
		if t.env.StateDB.HasSelfDestructed(addr) {
			delete(t.pre, addr)
			continue
		}

		modified := false
		postAcc := &PrestateAccount{Storage: make(map[common.Hash]common.Hash)}
		newBalance := t.env.StateDB.GetBalance(addr)
		newNonce := t.env.StateDB.GetNonce(addr)
		newCode := t.env.StateDB.GetCode(addr)

		if newBalance.Cmp(preAcc.Balance) != 0 {
			modified = true
			postAcc.Balance = newBalance
		}
		if newNonce != preAcc.Nonce {
			modified = true
			postAcc.Nonce = newNonce
		}
		if !bytes.Equal(newCode, preAcc.Code) {
			modified = true
			postAcc.Code = newCode
		}

		for key, val := range preAcc.Storage {
			// Drop empty pre-slots so they don't appear in the pre output unnecessarily.
			if val == (common.Hash{}) {
				delete(preAcc.Storage, key)
			}
			newVal := t.env.StateDB.GetState(addr, key)
			if val != newVal {
				modified = true
				if newVal != (common.Hash{}) {
					postAcc.Storage[key] = newVal
				}
			}
		}

		if modified {
			t.post[addr] = postAcc
		} else {
			// Nothing actually changed for this account — drop it from pre too,
			// so the diff only contains real changes.
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
	t.from = from
	t.to = to
	t.create = create
	if value != nil {
		t.value = new(big.Int).Set(value)
	}
	// Don't lookup balances/nonces here. Kaia fires CaptureStart before the
	// recipient credit, value transfer, and (for CREATE) caller nonce++ are
	// applied — reversing those from this hook would over- or under-correct.
	// We do the snapshot in CaptureTxEnd against the post-tx StateDB instead.
}

func (t *PrestateTracer) CaptureEnd(output []byte, gasUsed uint64, err error) {
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
		// Ensure the contract account is captured before reading a slot — for
		// nested frames the contract's prestate may not have been seen yet.
		t.lookupAccount(caller)
		slot := common.Hash(stackData[stackLen-1].Bytes32())
		t.lookupStorage(caller, slot)
	case stackLen >= 1 && (op == EXTCODECOPY || op == EXTCODEHASH || op == EXTCODESIZE || op == BALANCE || op == SELFDESTRUCT):
		// SELFDESTRUCT: don't mark `caller` as deleted here — the opcode body
		// hasn't run yet. The diffMode loop in CaptureTxEnd inspects
		// StateDB.Exist on the post-tx state, which correctly handles
		// readOnly aborts, outer reverts, and EIP-6780 (where non-same-tx
		// accounts survive).
		addr := common.Address(stackData[stackLen-1].Bytes20())
		t.lookupAccount(addr)
	case stackLen >= 5 && (op == DELEGATECALL || op == CALL || op == STATICCALL || op == CALLCODE):
		addr := common.Address(stackData[stackLen-2].Bytes20())
		t.lookupAccount(addr)
	case op == CREATE:
		t.lookupAccount(caller)
		nonce := t.env.StateDB.GetNonce(caller)
		addr := crypto.CreateAddress(caller, nonce)
		t.lookupAccount(addr)
		t.created[addr] = true
	case stackLen >= 4 && op == CREATE2:
		t.lookupAccount(caller)
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
