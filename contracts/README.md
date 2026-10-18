# Ferryman contracts

Solidity 0.8.28 contracts for two flows: **Ferry Out** (leave Elysium, land HYPE on HyperEVM) and **Ferry In** (buy an Ascend token on Elysium with HYPE from HyperEVM). Immutable, no owner, no proxy, no OpenZeppelin. Built with Foundry.

## Layout

```
src/
  libraries/ExitParams.sol     ExitParams struct, salt and CREATE2 receiver helpers
  libraries/InfinityTypes.sol  PoolKey, BalanceDelta and pool id helpers (plain addresses, no infinity-core)
  libraries/TokenTransfers.sol tolerant ERC20 transfer helpers
  interfaces/IArbSys.sol       ArbSys.withdrawEth (0x64, Elysium)
  interfaces/IOutbox.sol       Arbitrum Outbox.executeTransaction (selector 0x08635a95)
  interfaces/IERC20.sol        minimal ERC20 surface
  interfaces/IWHYPE.sol        deposit / withdraw
  interfaces/infinity/         IVault, ICLPoolManager, ILockCallback (subset of PancakeSwap Infinity)
  elysium/ExitDesk.sol         exitHYPE: starts an exit, sends HYPE to the precomputed receiver
  elysium/FeeJar.sol           collects FerryIn fees; permissionless release to the beneficiary
  elysium/InfinitySwapAdapter.sol  exact-input swaps through Infinity CL pools (one lock per call)
  elysium/FerryIn.sol          buy(token, recipient, minOut): fee, swap, refund on any failure
  elysium/WHYPE9.sol           WETH9-style wrapper, only if Elysium's WHYPE lacks deposit()
  hyperevm/ExitFactory.sol     claimAndSweep / sweep: executes the Outbox claim and pays out
  hyperevm/Receiver.sol        per-exit holding contract deployed via CREATE2
test/                          Foundry tests; mocks in test/mocks/ (ArbSys, Outbox, Vault, CLPoolManager, ERC20)
script/                        DeployHyperEVM.s.sol, DeployElysium.s.sol
lib/forge-std/                 vendored forge-std (no nested git repo)
```

Addresses and interfaces: `docs/INTERFACES.md` section 2.

`foundry.toml` grants read-only filesystem access to `../deployments`, so the Elysium deploy script can read the HYPE route file.

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
- **InfinitySwapAdapter** (Elysium). `swapExactIn(path, tokenIn, amountIn, minOut, recipient)` runs each hop inside one Vault lock. Intermediate currencies net to zero, so only the first input is paid and only the last output is taken. Native HYPE into a WHYPE pool is wrapped first. `quoteExactIn` returns the same output through a revert, so it is safe for `eth_call`. Every hop's pool manager must be registered with the Vault, a hop that spends less than its input reverts (`PartialFill`), and the adapter has no owner.
- **FerryIn** (Elysium). `buy(token, recipient, minOut)` takes 0.3% (`FEE_BPS = 30`) to FeeJar, then swaps the rest through the fixed HYPE route for the token's quote currency and then the token's Ascend pool. Any failure refunds the full `msg.value` to the recipient and emits `Refunded` with the revert reason. It never reverts after receiving value. Refund order: native HYPE, then WHYPE, then `owed[recipient]` (claimable with `claimOwed()`). `registerToken(key)` is permissionless. It requires `key.hooks == LAUNCHPAD`, a pool on the constructor's pool manager, one supported quote currency and the token on the other side, and an initialized pool. The first registration for a token wins. `quote(token, hypeIn)` is for `eth_call`.
- **WHYPE9** (Elysium). Only deployed if Elysium's WHYPE has no `deposit()`. Set `DEPLOY_WHYPE9=true`.

### Ferry In function signatures

```solidity
// InfinitySwapAdapter(address vault, address whype)
function swapExactIn(PoolKey[] calldata path, address tokenIn, uint256 amountIn, uint256 minOut, address recipient) external payable returns (uint256 amountOut);
function quoteExactIn(PoolKey[] calldata path, address tokenIn, uint256 amountIn) external returns (uint256 amountOut);

// FerryIn(address adapter, address feeJar, address launchpad, address whype, PoolKey[] hypeRoutes)
function registerToken(PoolKey calldata key) external;
function buy(address token, address recipient, uint256 minOut) external payable;
function quote(address token, uint256 hypeIn) external returns (uint256 amountOut);
function claimOwed() external;
function poolKeyOf(address token) external view returns (PoolKey memory);
function isQuoteCurrency(address currency) external view returns (bool);
function FEE_BPS() external view returns (uint256); // 30
```

`PoolKey` is `(address currency0, address currency1, address hooks, address poolManager, uint24 fee, bytes32 parameters)`, which is the same ABI tuple as infinity-core's `PoolKey`.

## Security properties

- **No redirection.** The receiver is a pure function of `ExitParams`, and the Outbox payout must target that receiver. The recipient is fixed by the signed Elysium exit, so neither the keeper nor anyone else can change where funds go.
- **Tip cannot be stolen.** The claim and the payout happen in one call, so the tip is paid to whoever runs that call.
- **Single sweep per exit.** A salt is marked swept before payout and cannot be swept twice.
- **Reentrancy guard.** `claimAndSweep` and `sweep` share a lock.
- **Atomic.** If any payout fails, the whole claim reverts, including the Outbox execution.
- **Immutable.** No owner, no upgrade path, no admin keys.

## Deploy (testnet)

Order matters. ExitFactory is on HyperEVM, and its address goes into the Elysium step.

1. **HyperEVM**, ExitFactory (outbox defaults to `0x87391D602aaffB3Fe8051EcCDb75d6EEe8C31060`):

   ```sh
   forge script script/DeployHyperEVM.s.sol \
     --rpc-url https://rpc.hyperliquid-testnet.xyz/evm --account <name> --broadcast
   ```

   Note the printed `ExitFactory` address and `receiverInitHash`.

2. **Elysium**, FeeJar, ExitDesk, InfinitySwapAdapter, FerryIn (and WHYPE9 if needed). The HYPE route file is read with `HYPE_ROUTES_JSON`:

   ```sh
   EXIT_FACTORY=<exitFactory from step 1> \
   FEE_BENEFICIARY=<fee router> \
   HYPE_ROUTES_JSON=$PWD/../deployments/hype-routes.json \
   forge script script/DeployElysium.s.sol \
     --rpc-url https://testnet-rpc.elysium.kinetiq.xyz --account <name> --broadcast
   ```

   Optional: `VAULT`, `LAUNCHPAD`, `WHYPE` (defaults are in the script), `RECEIVER_INIT_HASH` (checked against the local computation), `DEPLOY_WHYPE9=true`.

   Copy `deployments/hype-routes.example.json` to `deployments/hype-routes.json` and fill in the real pool keys first. The script reverts if a route pool is missing or uninitialized, if its pool manager is not the same across routes, or if the quote currency repeats.

3. Write the addresses into `deployments/testnet.json`, which the keeper and the web app read.

### HYPE route file

`deployments/hype-routes.example.json` shows the shape. Each entry is one pool from HYPE to a quote currency, and the list holds one entry per supported quote currency:

```json
{ "routes": [ { "currency0": "0x0000000000000000000000000000000000000000", "currency1": "0x...quote", "hooks": "0x...", "poolManager": "0x...", "fee": 3000, "parameters": "0x...32 bytes" } ] }
```

Use `currency0 = address(0)` for native HYPE, or the WHYPE address. Keys must be sorted (`currency0 < currency1`). The `_comment` field in the example is ignored.

## Kill tests (run on real networks before relying on the design)

From `details.md` section 11. Each one can change the design if it fails.

1. **Exit contract calls `ArbSys.withdrawEth` to a precomputed address.** Wait for maturity, fetch claim calldata, execute it on the Outbox, deploy `Receiver`, sweep. If it fails, the user's EOA calls `withdrawEth(receiver)` directly.
2. **Ascend `launch()` and `claim` on testnet.** Read `LaunchParams` from the verified ABI. If launch access is not available, use a labelled stand-in token.
3. **EOA calls `Inbox.createRetryableTicket` to an Elysium receiver.** This is the Ferry In entry point. If it fails, use `depositEth` plus a keeper-executed signed buy.
4. **WHYPE `deposit()` (selector `0xd0e30db0`) and one swap through `InfinitySwapAdapter` in a real Ascend pool.** If WHYPE has no `deposit()`, deploy `WHYPE9` and a labelled seeded HYPE/mock-USDC pool. If the adapter fails, Ferry In is cut.
5. **HyperCore landing.** Send HYPE from an EOA to `0x2222222222222222222222222222222222222222` on HyperEVM testnet and check `spotClearinghouseState`.
6. **Oxley's testnet bridge.** Check it, and drop any "no other exit tool exists" claim if it is wrong.

## Unverified assumptions

Everything below comes from the Infinity source at HEAD and from search snippets, not from Elysium itself. The tests use mocks that follow these assumptions.

- **Vault and CLPoolManager.** The Vault is at `0x2B738A9A69b7a45C9dA6B519f6bE6efBfb912aec`. The Elysium Vault has `lock`, `sync`, `settle`, `take`, `isAppRegistered`, and `currencyDelta` with the same semantics as the HEAD version. The adapter requires `isAppRegistered(poolManager) == true` for every hop. If the deployed Vault lacks this, every swap refunds.
- **Ascend hookData is empty.** The adapter passes `""`. The Ascend hook may return a delta or decode hookData, so check that a real swap succeeds and that the realized output is what we expect.
- **WHYPE.** Elysium's WHYPE at `0xCD57F65c2B0e5881CFC2E609F7CD53b746E1F234` has `deposit()` and `withdraw(uint256)`. If it does not, set `DEPLOY_WHYPE9=true`. The refund fallback uses `withdraw`.
- **HYPE/quote pools.** Routes exist on Elysium and are initialized. The constructor checks this, so a missing pool fails the deploy, not a buy.
- **Ascend pool shape.** Pool keys, `Initialize` events, and the launchpad ABI are unknown. `registerToken` only accepts `hooks == LAUNCHPAD` on the route pool manager, and it trusts that the first registration is honest. A griefer who creates a manipulated Ascend pool for a token first can get that pool registered, so the token's buys would use it. `minOut` and refunds limit the damage but do not remove it.
- **Retryable behavior.** A reverting L2 call leaves `l2CallValue` in the ticket escrow until the ticket expires. That is why FerryIn refunds instead of reverting. Auto-redeem gas and fee estimates are unverified.
- **Partial fills.** An exact-input hop can stop early when the pool runs out of liquidity before the price limit. The adapter reverts with `PartialFill` in that case, so FerryIn refunds the buyer instead of keeping leftover input.

