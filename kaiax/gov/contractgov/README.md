# kaiax/gov/contractgov
This module is responsible for providing the governance parameter set from **contract governance** at a given block number.

## Concepts

Please read [gov module](../gov/README.md) and [header governance](../headergov/README.md) first.

### Key Concepts

- *GovParam contract*: a contract that stores the governance parameters.
- *effective parameter set at blockNum*: the governance parameter set that are effective when mining the given block.

### Contract governance

Contract governance is the process of changing the governance parameters among members of the GC via on-chain voting.
This module reads GovParam contract.
GovParam is updated by on-chain voting specified by [KIP-81](https://kips.kaia.io/KIPs/kip-81).

GovParam address can be retrieved from headergov module.
This module reads GovParam from the latest state, which is done via a view function `getAllParamsAt(uint256 blockNumber)` that returns all the effective parameters at the block `blockNumber`.
GovParam contract stores all values as bytes.

Note that not all values in GovParam contract may be recognized as valid by this module.
Some parameters may be invalid (e.g., invalid parameter name or non-canonical value), which will be ignored.
Therefore, users should use this module or API to check the contract parameters, instead of calling GovParam contract directly.

## Persistent Schema

This module does not have any persistent data.

## In-memory Structures

## Module lifecycle
### Init

- Dependencies:
  - headergov: To fetch the GovParam address.

### Start and stop
This module does not have any background threads.

## Block processing

### Consensus
This module does not have any consensus-related block processing logic.

### Execution
This module does not have any execution-related block processing logic.

### Rewind
This module does not have any rewind-related block processing logic.

## APIs

### governance_getContractParam


## Getters

- `EffectiveParamSet(num)`: Returns the effective parameter set at the block `num`.
  ```
  EffectiveParamSet(num) -> ParamSet
  ```

- `EffectiveParamsPartial(num)`: Returns only the parameters effective by GovParam contract at the block `num`. It is used for assembling parameters in a gov module.
  ```
  EffectiveParamsPartial(num) -> map[ParamName]any
  ```
