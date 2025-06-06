// Copyright 2025 The klaytn Authors
// This file is part of the klaytn library.
//
// The klaytn library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The klaytn library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the klaytn library. If not, see <http://www.gnu.org/licenses/>.
// Modified and improved for the Kaia development.

package state

import (
	"math/big"
	"testing"

	"github.com/kaiachain/kaia/common"
	"github.com/kaiachain/kaia/storage/database"
	"github.com/kaiachain/kaia/storage/statedb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRollback(t *testing.T) {
	dbm := database.NewMemoryDBManager()
	dbm.WritePruningEnabled()

	var (
		sdb  = NewDatabase(dbm)
		acc1 = common.HexToAddress("0x0000000000000000000000000000000000000aaa")
		acc2 = common.HexToAddress("0x0000000000000000000000000000000000000bbb")
		acc3 = common.HexToAddress("0x0000000000000000000000000000000000000ccc")

		root1 common.Hash
		root2 common.Hash
	)

	{ // Store a block before bundle execution.
		t.Log("begin block 1")
		opts := &statedb.TrieOpts{PruningBlockNumber: 1}
		state, err := New(common.Hash{}, sdb, nil, opts)
		assert.NoError(t, err)

		state.AddBalance(acc1, big.NewInt(10))
		state.AddBalance(acc2, big.NewInt(20))
		state.AddBalance(acc3, big.NewInt(30))
		root1, err = state.Commit(true)
		assert.NoError(t, err)
		t.Logf("end block 1, root %s", root1.Hex())
	}

	{ // Build a bundle-containing block.
		t.Log("begin block 2")
		opts := &statedb.TrieOpts{PruningBlockNumber: 2}
		state, err := New(root1, sdb, nil, opts)
		assert.NoError(t, err)

		// Run regular transactions
		state.AddBalance(acc2, big.NewInt(200))

		{ // Run bundle transactions.
			// Save state before bundle execution.
			snapshot := state.Copy()
			state.PauseLivePruning()

			// Execute bundle transactions.
			state.AddBalance(acc1, big.NewInt(100))

			// Restore state due to bundle transaction revert.
			state.Set(snapshot)
			state.ResumeLivePruning()
		}

		// Finalize the block.
		root2, err = state.Commit(true)
		assert.NoError(t, err)
		assert.Equal(t, uint64(10), state.GetBalance(acc1).Uint64())
		assert.Equal(t, uint64(220), state.GetBalance(acc2).Uint64())
		assert.Equal(t, uint64(30), state.GetBalance(acc3).Uint64())
		t.Logf("end block 2, root %s", root2.Hex())
	}

	{ // Simulate the passage of time, in that

		// - db.pruningMarks are eventually written to the diskDB.
		sdb.TrieDB().Cap(0)

		// - in-memory trie cache is evicted (governed by --state.cache-size).
		sdb = NewDatabase(dbm)

		// - bc.pruneTrieNodeLoop() deleted (after retention) as dictated by the pruning marks.
		marks := dbm.ReadPruningMarks(0, 99)
		for _, mark := range marks {
			t.Logf("delete trie node (%s, %d)", mark.Hash.Hex(), mark.Number)
			dbm.DeleteTrieNode(mark.Hash)
		}
	}

	{ // After that, some trie nodes that represent the latest state, must be intact.
		// i.e. the states must not be pruned.
		t.Log("query block 1")
		state, err := New(root1, sdb, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, uint64(10), state.GetBalance(acc1).Uint64())
		assert.Equal(t, uint64(20), state.GetBalance(acc2).Uint64())
		assert.Equal(t, uint64(30), state.GetBalance(acc3).Uint64())

		t.Log("query block 2")
		state, err = New(root2, sdb, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, uint64(10), state.GetBalance(acc1).Uint64())
		assert.Equal(t, uint64(220), state.GetBalance(acc2).Uint64())
		assert.Equal(t, uint64(30), state.GetBalance(acc3).Uint64())

	}
}
