// Modifications Copyright 2024 The Kaia Authors
// Modifications Copyright 2018 The klaytn Authors
// Copyright 2015 The go-ethereum Authors
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
// This file is derived from cmd/geth/chaincmd.go (2018/06/04).
// Modified and improved for the klaytn development.
// Modified and improved for the Kaia development.

package nodecmd

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kaiachain/kaia/blockchain"
	"github.com/kaiachain/kaia/blockchain/types"
	"github.com/kaiachain/kaia/blockchain/types/accountkey"
	"github.com/kaiachain/kaia/cmd/utils"
	"github.com/kaiachain/kaia/common"
	"github.com/kaiachain/kaia/crypto"
	headergov_impl "github.com/kaiachain/kaia/kaiax/gov/headergov/impl"
	"github.com/kaiachain/kaia/log"
	"github.com/kaiachain/kaia/params"
	"github.com/kaiachain/kaia/rlp"
	"github.com/kaiachain/kaia/storage/database"
	"github.com/urfave/cli/v2"
)

var logger = log.NewModuleLogger(log.CMDUtilsNodeCMD)

var (
	InitCommand = &cli.Command{
		Action:    initGenesis,
		Name:      "init",
		Usage:     "Bootstrap and initialize a new genesis block",
		ArgsUsage: "<genesisPath>",
		Flags: []cli.Flag{
			utils.DbTypeFlag,
			utils.SingleDBFlag,
			utils.NumStateTrieShardsFlag,
			utils.DynamoDBTableNameFlag,
			utils.DynamoDBRegionFlag,
			utils.DynamoDBIsProvisionedFlag,
			utils.DynamoDBReadCapacityFlag,
			utils.DynamoDBWriteCapacityFlag,
			utils.DynamoDBReadOnlyFlag,
			utils.LevelDBCompressionTypeFlag,
			utils.DataDirFlag,
			utils.ChainDataDirFlag,
			utils.RocksDBSecondaryFlag,
			utils.RocksDBCacheSizeFlag,
			utils.RocksDBDumpMallocStatFlag,
			utils.RocksDBFilterPolicyFlag,
			utils.RocksDBCompressionTypeFlag,
			utils.RocksDBBottommostCompressionTypeFlag,
			utils.RocksDBDisableMetricsFlag,
			utils.RocksDBMaxOpenFilesFlag,
			utils.RocksDBCacheIndexAndFilterFlag,
			utils.OverwriteGenesisFlag,
			utils.LivePruningFlag,
		},
		Category: "BLOCKCHAIN COMMANDS",
		Description: `
The init command initializes a new genesis block and definition for the network.
This is a destructive action and changes the network in which you will be
participating.

It expects the genesis file as argument.`,
	}

	DumpGenesisCommand = &cli.Command{
		Action:    dumpGenesis,
		Name:      "dumpgenesis",
		Usage:     "Dumps genesis block JSON configuration to stdout",
		ArgsUsage: "",
		Flags: []cli.Flag{
			utils.MainnetFlag,
			utils.KairosFlag,
		},
		Category: "BLOCKCHAIN COMMANDS",
		Description: `
The dumpgenesis command dumps the genesis block configuration in JSON format to stdout.`,
	}

	DbGetCommand = &cli.Command{
		Action:    dbGet,
		Name:      "dbget",
		Usage:     "Read a key from the underlying key-value database",
		ArgsUsage: "",
		Flags: []cli.Flag{
			utils.MainnetFlag,
			utils.KairosFlag,
		},
		Category: "BLOCKCHAIN COMMANDS",
	}
)

// initGenesis will initialise the given JSON format genesis file and writes it as
// the zero'd block (i.e. genesis) or will fail hard if it can't succeed.
func initGenesis(ctx *cli.Context) error {
	// Make sure we have a valid genesis JSON
	genesisPath := ctx.Args().First()
	if len(genesisPath) == 0 {
		logger.Crit("Must supply path to genesis JSON file")
	}
	file, err := os.Open(genesisPath)
	if err != nil {
		logger.Crit("Failed to read genesis file", "err", err)
	}
	defer file.Close()

	genesis := new(blockchain.Genesis)
	if err := json.NewDecoder(file).Decode(genesis); err != nil {
		logger.Crit("Invalid genesis file", "err", err)
		return err
	}
	if genesis.Config == nil {
		logger.Crit("Genesis config is not set")
	}

	// Update undefined config with default values
	genesis.Config.SetDefaultsForGenesis()

	// Validate config values
	if err := ValidateGenesisConfig(genesis); err != nil {
		logger.Crit("Invalid genesis", "err", err)
	}

	// Set genesis.Governance and reward intervals
	genesis.Governance = headergov_impl.GetGenesisGovBytes(genesis.Config)
	params.SetStakingUpdateInterval(genesis.Config.Governance.Reward.StakingUpdateInterval)
	params.SetProposerUpdateInterval(genesis.Config.Governance.Reward.ProposerUpdateInterval)

	// Open an initialise both full and light databases
	stack := MakeFullNode(ctx)
	parallelDBWrite := !ctx.Bool(utils.NoParallelDBWriteFlag.Name)
	singleDB := ctx.Bool(utils.SingleDBFlag.Name)
	numStateTrieShards := ctx.Uint(utils.NumStateTrieShardsFlag.Name)
	overwriteGenesis := ctx.Bool(utils.OverwriteGenesisFlag.Name)
	livePruning := ctx.Bool(utils.LivePruningFlag.Name)

	dbtype := database.DBType(ctx.String(utils.DbTypeFlag.Name)).ToValid()
	if len(dbtype) == 0 {
		logger.Crit("invalid dbtype", "dbtype", ctx.String(utils.DbTypeFlag.Name))
	}

	var dynamoDBConfig *database.DynamoDBConfig
	if dbtype == database.DynamoDB {
		dynamoDBConfig = &database.DynamoDBConfig{
			TableName:          ctx.String(utils.DynamoDBTableNameFlag.Name),
			Region:             ctx.String(utils.DynamoDBRegionFlag.Name),
			IsProvisioned:      ctx.Bool(utils.DynamoDBIsProvisionedFlag.Name),
			ReadCapacityUnits:  ctx.Int64(utils.DynamoDBReadCapacityFlag.Name),
			WriteCapacityUnits: ctx.Int64(utils.DynamoDBWriteCapacityFlag.Name),
			ReadOnly:           ctx.Bool(utils.DynamoDBReadOnlyFlag.Name),
		}
	}
	rocksDBConfig := database.GetDefaultRocksDBConfig()
	if dbtype == database.RocksDB {
		rocksDBConfig = &database.RocksDBConfig{
			Secondary:                 ctx.Bool(utils.RocksDBSecondaryFlag.Name),
			DumpMallocStat:            ctx.Bool(utils.RocksDBDumpMallocStatFlag.Name),
			DisableMetrics:            ctx.Bool(utils.RocksDBDisableMetricsFlag.Name),
			CacheSize:                 ctx.Uint64(utils.RocksDBCacheSizeFlag.Name),
			CompressionType:           ctx.String(utils.RocksDBCompressionTypeFlag.Name),
			BottommostCompressionType: ctx.String(utils.RocksDBBottommostCompressionTypeFlag.Name),
			FilterPolicy:              ctx.String(utils.RocksDBFilterPolicyFlag.Name),
			MaxOpenFiles:              ctx.Int(utils.RocksDBMaxOpenFilesFlag.Name),
			CacheIndexAndFilter:       ctx.Bool(utils.RocksDBCacheIndexAndFilterFlag.Name),
		}
	}

	for _, name := range []string{"chaindata"} { // Removed "lightchaindata" since Kaia doesn't use it
		dbc := &database.DBConfig{
			Dir: name, DBType: dbtype, ParallelDBWrite: parallelDBWrite,
			SingleDB: singleDB, NumStateTrieShards: numStateTrieShards,
			LevelDBCacheSize: 0, PebbleDBCacheSize: 0, OpenFilesLimit: 0,
			DynamoDBConfig: dynamoDBConfig, RocksDBConfig: rocksDBConfig,
		}
		chainDB := stack.OpenDatabase(dbc)

		// Initialize DeriveSha implementation
		blockchain.InitDeriveSha(genesis.Config)

		_, hash, err := blockchain.SetupGenesisBlock(chainDB, genesis, params.UnusedNetworkId, false, overwriteGenesis)
		if err != nil {
			logger.Crit("Failed to write genesis block", "err", err)
		}

		// Write the live pruning flag to database
		if livePruning {
			logger.Info("Writing live pruning flag to database")
			chainDB.WritePruningEnabled()
		}

		logger.Info("Successfully wrote genesis state", "database", name, "hash", hash.String())
		chainDB.Close()
	}
	return nil
}

func dumpGenesis(ctx *cli.Context) error {
	genesis := MakeGenesis(ctx)
	if genesis == nil {
		genesis = blockchain.DefaultGenesisBlock()
	}
	if err := json.NewEncoder(os.Stdout).Encode(genesis); err != nil {
		logger.Crit("could not encode genesis")
	}
	return nil
}

func MakeGenesis(ctx *cli.Context) *blockchain.Genesis {
	var genesis *blockchain.Genesis
	switch {
	case ctx.Bool(utils.MainnetFlag.Name):
		genesis = blockchain.DefaultGenesisBlock()
	case ctx.Bool(utils.KairosFlag.Name):
		genesis = blockchain.DefaultKairosGenesisBlock()
	}
	return genesis
}

func ValidateGenesisConfig(g *blockchain.Genesis) error {
	if g.Config.ChainID == nil {
		return errors.New("chainID is not specified")
	}

	if g.Config.Clique == nil && g.Config.Istanbul == nil {
		return errors.New("consensus engine should be configured")
	}

	if g.Config.Clique != nil && g.Config.Istanbul != nil {
		return errors.New("only one consensus engine can be configured")
	}

	if g.Config.Governance == nil || g.Config.Governance.Reward == nil {
		return errors.New("governance and reward policies should be configured")
	}

	if g.Config.Governance.Reward.ProposerUpdateInterval == 0 || g.Config.Governance.Reward.
		StakingUpdateInterval == 0 {
		return errors.New("proposerUpdateInterval and stakingUpdateInterval cannot be zero")
	}

	if g.Config.Istanbul != nil {
		// TODO-Kaia: Add validation logic for other GovernanceModes
		// Check if governingNode is properly set
		if strings.ToLower(g.Config.Governance.GovernanceMode) == "single" {
			var found bool

			istanbulExtra, err := types.ExtractIstanbulExtra(&types.Header{Extra: g.ExtraData})
			if err != nil {
				return err
			}

			for _, v := range istanbulExtra.Validators {
				if v == g.Config.Governance.GoverningNode {
					found = true
					break
				}
			}
			if !found {
				return errors.New("governingNode is not in the validator list")
			}
		}
	}
	return nil
}

func dbGet(ctx *cli.Context) error {
	startNum, _ := strconv.Atoi(ctx.Args().First())
	endNum, _ := strconv.Atoi(ctx.Args().Get(1))
	fmt.Printf("searching between %d and %d\n", startNum, endNum)

	stack := MakeFullNode(ctx)
	parallelDBWrite := !ctx.Bool(utils.NoParallelDBWriteFlag.Name)
	singleDB := ctx.Bool(utils.SingleDBFlag.Name)
	numStateTrieShards := ctx.Uint(utils.NumStateTrieShardsFlag.Name)

	dbtype := database.DBType(ctx.String(utils.DbTypeFlag.Name)).ToValid()
	if len(dbtype) == 0 {
		logger.Crit("invalid dbtype", "dbtype", ctx.String(utils.DbTypeFlag.Name))
	}
	dbc := &database.DBConfig{
		Dir: "chaindata", DBType: dbtype, ParallelDBWrite: parallelDBWrite,
		SingleDB: singleDB, NumStateTrieShards: numStateTrieShards,
		LevelDBCacheSize: 0, PebbleDBCacheSize: 0, OpenFilesLimit: 0,
	}
	chainDB := stack.OpenDatabase(dbc)
	defer chainDB.Close()

	fmt.Printf("%x\n", chainDB.ReadHeadHeaderHash())
	fmt.Printf("%x\n", chainDB.ReadCanonicalHash(1))

	for i := uint64(startNum); i <= uint64(endNum); i++ {
		printBlock(chainDB, i)
	}

	return nil
}

var targetAddrs = []common.Address{
	common.HexToAddress("0x7bdd84820064d632c6bdbdf608f0bccacdc37966"),
	common.HexToAddress("0x60F6e30602Af53781AA5137CFDF96C3d18b16FEe"),
	common.HexToAddress("0x7A795b405Ab719258ADA4cc8a8d52E47CE77db74"),
	common.HexToAddress("0x5ad32CA5ABF4EA798ba2d8F8CfFF7724dC4A3f81"),
	common.HexToAddress("0xf3d2bca68c23b94cefbff553ebad9322138c8697"),
	common.HexToAddress("0xb3b12c667ab36daa70c05f3a5e17721506409186"),
	common.HexToAddress("0x7dde841f9a2bf323a8c00be344383e86644718ed"),
}

func printBlock(dbm database.DBManager, num uint64) {
	h := dbm.ReadCanonicalHash(num)
	r := dbm.ReadBodyRLP(h, num)
	b := new(types.Body)
	if err := rlp.Decode(bytes.NewReader(r), b); err != nil {
		logger.Crit("decode error", "err", err)
	}
	for _, tx := range b.Transactions {
		if tx.Type().IsAccountUpdate() && tx.AccountKey() != nil {
			ak := tx.AccountKey()
			fmt.Printf("AU %d %x %s\n", num, tx.Hash(), ak.String())
			printKey("AU", ak)
		}
	}
}

func printKey(prefix string, accountKey accountkey.AccountKey) {
	switch ak := accountKey.(type) {
	case *accountkey.AccountKeyWeightedMultiSig:
		for _, key := range ak.Keys {
			fmt.Printf("%s MS %s\n", prefix, stringifyPub(key.Key))
		}
	case *accountkey.AccountKeyRoleBased:
		for _, key := range *ak {
			printKey(prefix+" RB", key)
		}
	case *accountkey.AccountKeyPublic:
		fmt.Printf("%s PK %s\n", prefix, stringifyPub(ak.PublicKeySerializable))
	}
}

func stringifyPub(pub *accountkey.PublicKeySerializable) string {
	addr := crypto.PubkeyToAddress(ecdsa.PublicKey(*pub))
	for _, a := range targetAddrs {
		if a == addr {
			fmt.Println("###### found", a.Hex())
		}
	}
	return fmt.Sprintf("%s %x", pub.String(), addr)
}
