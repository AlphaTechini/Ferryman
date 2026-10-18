# Ferryman contracts

Solidity 0.8.28 contracts for the Ferry Out flow (leave Elysium, land HYPE on HyperEVM). Immutable, no owner, no proxy. Built with Foundry.

## Layout

```
src/
  libraries/ExitParams.sol     ExitParams struct, salt and CREATE2 receiver helpers
  interfaces/IArbSys.sol       ArbSys.withdrawEth (0x64, Elysium)
  interfaces/IOutbox.sol       Arbitrum Outbox.executeTransaction (selector 0x08635a95)
  elysium/ExitDesk.sol         exitHYPE: starts an exit, sends HYPE to the precomputed receiver
  elysium/FeeJar.sol           collects FerryIn fees; permissionless release to the beneficiary
  hyperevm/ExitFactory.sol     claimAndSweep / sweep: executes the Outbox claim and pays out
  hyperevm/Receiver.sol        per-exit holding contract deployed via CREATE2
test/                          Foundry tests, mocks (ArbSys, Outbox) in test/mocks/
script/                        deployment scripts (none yet)
lib/forge-std/                 vendored forge-std (no nested git repo)
```

Addresses and interfaces: `docs/INTERFACES.md` section 2.

## Build and test

Standard Foundry:

```sh
forge build
forge test -vv
forge fmt
```

`foundry.toml` pins `solc_version = "0.8.28"`, so a normal environment downloads the compiler automatically.

In the restricted build container, the native solc download host is blocked. There, pass the local solc wrapper on the command line for every forge call:

```sh
forge build --offline --use /usr/local/bin/solc
forge test --offline --use /usr/local/bin/solc -vv
```

## Contracts

- **ExitDesk** (Elysium). `exitHYPE(recipient, claimTip, toCore)` builds `ExitParams` from `(chainId, desk, sender, nonce, amount, claimTip, recipient, toCore)`, derives the receiver with CREATE2, and calls `ArbSys.withdrawEth{value}(receiver)`. `previewReceiver` returns the same params and receiver without state changes.
- **ExitFactory** (HyperEVM). `claimAndSweep(outboxCalldata, p)` checks the selector and that the Outbox payout targets `receiverFor(p)`, executes the claim (or accepts a claim already executed by someone else, if the receiver holds the funds), deploys the Receiver, then pays `amount - claimTip` to `p.recipient` and `claimTip` to the caller. `sweep(p)` forwards late or stray funds to `p.recipient`.
- **Receiver** (HyperEVM). Holds funds for one exit. Accepts HYPE at any time, including before deployment. Only the factory can call `pull`. No selfdestruct.
- **FeeJar** (Elysium). Receives FerryIn fees. `release()` sends the full balance to the immutable beneficiary.

## Security properties

- **No redirection.** The receiver is a pure function of `ExitParams`, and the Outbox payout must target that receiver. The recipient is fixed by the signed Elysium exit, so neither the keeper nor anyone else can change where funds go.
- **Tip cannot be stolen.** The claim and the payout happen in one call, so the tip is paid to whoever runs that call.
- **Single sweep per exit.** A salt is marked swept before payout and cannot be swept twice.
- **Reentrancy guard.** `claimAndSweep` and `sweep` share a lock.
- **Atomic.** If any payout fails, the whole claim reverts, including the Outbox execution.
- **Immutable.** No owner, no upgrade path, no admin keys.
