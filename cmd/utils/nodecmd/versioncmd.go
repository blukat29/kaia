// Modifications Copyright 2024 The Kaia Authors
// Modifications Copyright 2018 The klaytn Authors
// Copyright 2016 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.
//
// This file is derived from cmd/geth/misccmd.go (2018/06/04).
// Modified and improved for the klaytn development.
// Modified and improved for the Kaia development.

package nodecmd

import (
	"bytes"
	"fmt"
	"log"

	"github.com/kaiachain/kaia/blockchain/types"
	"github.com/kaiachain/kaia/cmd/utils"
	"github.com/kaiachain/kaia/params"
	"github.com/kaiachain/kaia/rlp"
	"github.com/kaiachain/kaia/storage/database"
	"github.com/urfave/cli/v2"
)

var (
	// Git SHA1 commit hash of the release (set via linker flags)
	gitCommit = ""

	// Git tag (set via linker flags if exists)
	gitTag = ""
)

var VersionCommand = &cli.Command{
	Action:    version,
	Name:      "version",
	Usage:     "Show version number",
	ArgsUsage: " ",
	Category:  "MISCELLANEOUS COMMANDS",
}

func version(ctx *cli.Context) error {
	fmt.Print("Kaia ")
	if gitTag != "" {
		// stable version
		fmt.Println(params.Version)
	} else {
		// unstable version
		fmt.Println(params.VersionWithCommit(gitCommit))
	}
	return nil
}

// GetGitCommit returns gitCommit set by linker flags.
func GetGitCommit() string {
	return gitCommit
}

var ScanCommand = &cli.Command{
	Action:    scan,
	Name:      "scan",
	Usage:     "Scan the node",
	ArgsUsage: " ",
	Flags: []cli.Flag{
		utils.DbTypeFlag,
		utils.SingleDBFlag,
		utils.NumStateTrieShardsFlag,
		utils.DataDirFlag,
		utils.ChainDataDirFlag,
	},
	Category: "MISCELLANEOUS COMMANDS",
}

func scan(ctx *cli.Context) error {
	var (
		stack              = MakeFullNode(ctx)
		parallelDBWrite    = !ctx.Bool(utils.NoParallelDBWriteFlag.Name)
		singleDB           = ctx.Bool(utils.SingleDBFlag.Name)
		numStateTrieShards = ctx.Uint(utils.NumStateTrieShardsFlag.Name)
		datadir            = ctx.String(utils.DataDirFlag.Name)
	)
	dbtype := database.DBType(ctx.String(utils.DbTypeFlag.Name)).ToValid()
	if len(dbtype) == 0 {
		logger.Crit("invalid dbtype", "dbtype", ctx.String(utils.DbTypeFlag.Name))
	}
	dbc := &database.DBConfig{
		Dir: datadir, DBType: dbtype, ParallelDBWrite: parallelDBWrite,
		SingleDB: singleDB, NumStateTrieShards: numStateTrieShards,
		LevelDBCacheSize: 0, PebbleDBCacheSize: 0, OpenFilesLimit: 0,
	}
	dbm := stack.OpenDatabase(dbc)
	defer dbm.Close()

	for i := uint64(0); i < 1000000; i++ {
		printBlock(dbm, i)
	}
	return nil
}

func printBlock(dbm database.DBManager, num uint64) {
	h := dbm.ReadCanonicalHash(num)
	data := dbm.ReadBodyRLP(h, num)
	body := new(types.Body)
	if err := rlp.Decode(bytes.NewReader(data), body); err != nil {
		log.Fatal("decode error", "err", err)
	}
	for _, tx := range body.Transactions {
		fmt.Printf("%x\n", tx.Hash())
	}
}
