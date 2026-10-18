# Ferryman research notes

Tags: **VERIFIED(src)** = read from source code cloned into `/tmp/claude-0/research/<repo>` (path given). **VERIFIED(calc)** = computed with viem `keccak`. **SEARCH** = from a web-search snippet only. **ASSUMED** = not confirmed; test first.

**Reachability:** the sandbox could NOT reach `testnet-api.elysium.kinetiq.xyz`, `elysium.kinetiq.xyz`, the Elysium RPC, the HyperEVM RPC, or any docs site (WebFetch gives ENOTFOUND, curl gives proxy 403). Only `git clone`, npm and WebSearch snippets worked. So everything about the live Ascend launchpad, Lodestar, WHYPE, live pools and live Blockscout responses is **UNVERIFIED** and section 2 gives a discovery procedure instead of an ABI. Sources used: `pancakeswap/infinity-core` (HEAD at clone time), `pancakeswap/infinity-periphery`, `OffchainLabs/nitro-contracts` @faad883 plus npm `@arbitrum/nitro-contracts@1.3.0` (for precompile interfaces), `blockscout/blockscout` master.

---

## 1. PancakeSwap Infinity (CL) swap from a custom contract

Repo paths are relative to `/tmp/claude-0/research/infinity-core/src` (core) and `.../infinity-periphery/src`. All VERIFIED(src) unless noted. Caveat: this is current master; the Elysium Vault/CLPoolManager deployments may be an older revision (the `AppDeficit`/`clear` bits are recent). The calls used below (lock, lockAcquired, sync, settle, take, swap) have been stable.

### 1.1 Types
```solidity
// types/Currency.sol
type Currency is address;                       // native = Currency.wrap(address(0))  (CurrencyLibrary.NATIVE)

// types/PoolKey.sol  (field ORDER matters for abi.encode / tuple decoding)
struct PoolKey {
    Currency currency0;      // lower address; sorted numerically
    Currency currency1;      // higher address
    IHooks hooks;            // address
    IPoolManager poolManager;// address (the CLPoolManager)
    uint24 fee;              // LP fee, cap 1_000_000; dynamic fee = 0x800000
    bytes32 parameters;      // bits [0,16) hooks registration bitmap, bits [16,40) tickSpacing (int24 stored as uint24)
}

// types/BalanceDelta.sol
type BalanceDelta is int256;   // amount0 = UPPER 128 bits (sar(128,x)); amount1 = LOWER 128 bits (signextend(15,x)); both int128
// library: delta.amount0(), delta.amount1(); toBalanceDelta(int128,int128)

// pool-cl/interfaces/ICLPoolManager.sol
struct SwapParams { bool zeroForOne; int256 amountSpecified; uint160 sqrtPriceLimitX96; }   // exactly this order
function swap(PoolKey memory key, ICLPoolManager.SwapParams memory params, bytes calldata hookData)
    external returns (BalanceDelta delta);
```
JSON (for Go/TS): `poolKey = {currency0, currency1, hooks, poolManager, fee (uint24), parameters (bytes32)}`; tickSpacing = `(parameters >> 16) & 0xFFFFFF`.

### 1.2 Sign convention and price limits
- Exact input: `amountSpecified = -int256(amountIn)` (negative). Exact output: positive. (CLPool.sol: `exactInput = params.amountSpecified < 0`; periphery `CLRouterBase._swapExactInputSingle` passes `-int256(uint256(amountIn))`.) `amountSpecified == 0` reverts `SwapAmountCannotBeZero`.
- `zeroForOne = true` sells currency0 for currency1.
- Limit validity (CLPool.sol l.212): zeroForOne requires `MIN_SQRT_RATIO < limit < currentSqrtPrice`; oneForZero requires `currentSqrtPrice < limit < MAX_SQRT_RATIO`, else `InvalidSqrtPriceLimit`.
- `TickMath.MIN_SQRT_RATIO = 4295128739`; `TickMath.MAX_SQRT_RATIO = 1461446703485210103287273052203988822378723970342`; MIN_TICK -887272 / MAX_TICK 887272. Router uses `zeroForOne ? MIN_SQRT_RATIO + 1 : MAX_SQRT_RATIO - 1` (`infinity-periphery/src/pool-cl/CLRouterBase.sol` l.135). A tighter limit makes the swap partially fill (exact input then consumes less than specified; leftover input is NOT charged, delta reflects what was used) so always check realized amounts.
- Result delta signs: the currency you pay has negative delta, the currency you receive positive. Output for zeroForOne = `delta.amount1()`, else `delta.amount0()`. Input consumed = `-(zeroForOne ? amount0 : amount1)`.

### 1.3 Vault interface (`interfaces/IVault.sol`, `interfaces/ILockCallback.sol`)
```solidity
interface ILockCallback { function lockAcquired(bytes calldata data) external returns (bytes memory); }   // selector 0xab6291fe (calc)
// IVault
function lock(bytes calldata data) external returns (bytes memory);        // 0x81548319; Vault calls ILockCallback(msg.sender).lockAcquired(data); reverts CurrencyNotSettled if any delta != 0 after
function sync(Currency currency) external;                                   // 0xa5841194
function settle() external payable returns (uint256 paid);                   // 0x11da60b4
function settleFor(address recipient) external payable returns (uint256 paid);
function take(Currency currency, address to, uint256 amount) external;       // 0x0b0d9c09 (selector computed with address types)
function currencyDelta(address settler, Currency currency) external view returns (int256);
function getLocker() external view returns (address);
function clear(Currency currency, uint256 amount) external;
function mint(address to, Currency currency, uint256 amount) external;
function burn(address from, Currency currency, uint256 amount) external;
```
Accounting: `CLPoolManager.swap` -> `vault.accountAppDeltaWithHookDelta(...)` credits/debits the **locker** (your contract, `vault.getLocker()`), not `msg.sender` of the pool manager call chain beyond that. Positive delta = Vault owes you (call `take`); negative = you owe (call `settle`). Hook deltas are accounted to the hook address, not you.

Settlement rules (`Vault.sol` `_settle`, l.238):
- ERC20: `vault.sync(currency)`; `IERC20.transfer(vault, amount)`; `vault.settle()`. Paid = vault balance now - synced balance. `settle()` with `msg.value > 0` for ERC20 reverts `SettleNonNativeCurrencyWithValue`. Only ONE currency can be synced at a time; sync again before each ERC20 settle.
- Native: `vault.settle{value: amount}()` (sync not needed; periphery calls `sync(currency)` first for native too, which just zeroes the reserve; harmless and recommended).
- Receiving: `vault.take(currency, to, amount)` (native sent via call; `to` must accept ETH).
- WHYPE is an ERC20 so Ferryman can either hold WHYPE and settle as ERC20, or settle native HYPE directly if the pool's currency is `address(0)`. Which one the Ascend pools use is UNKNOWN (read the PoolKey).
- Reentrancy: only one locker at a time (`LockerAlreadySet`). `lockAcquired` must be callable ONLY by the Vault (`require(msg.sender == address(vault))`).

### 1.4 Minimal single-hop exact-input swap (modeled on `CLRouterBase` + `DeltaResolver`)
```solidity
import {IVault} from "infinity-core/src/interfaces/IVault.sol";
import {ILockCallback} from "infinity-core/src/interfaces/ILockCallback.sol";
import {ICLPoolManager} from "infinity-core/src/pool-cl/interfaces/ICLPoolManager.sol";
import {PoolKey} from "infinity-core/src/types/PoolKey.sol";
import {Currency, CurrencyLibrary} from "infinity-core/src/types/Currency.sol";
import {BalanceDelta} from "infinity-core/src/types/BalanceDelta.sol";
import {TickMath} from "infinity-core/src/pool-cl/libraries/TickMath.sol";

contract InfinitySwapAdapter is ILockCallback {
    IVault public immutable vault;
    constructor(IVault v) { vault = v; }

    struct Job { PoolKey key; bool zeroForOne; uint128 amountIn; uint128 minOut; address recipient; bytes hookData; }

    // Caller must already have transferred `amountIn` of the input token to this contract (or send native value).
    function swapExactIn(Job calldata j) external payable returns (uint256 out) {
        out = abi.decode(vault.lock(abi.encode(j)), (uint256));
    }

    function lockAcquired(bytes calldata data) external returns (bytes memory) {
        require(msg.sender == address(vault), "not vault");
        Job memory j = abi.decode(data, (Job));
        Currency cin  = j.zeroForOne ? j.key.currency0 : j.key.currency1;
        Currency cout = j.zeroForOne ? j.key.currency1 : j.key.currency0;

        BalanceDelta d = ICLPoolManager(address(j.key.poolManager)).swap(
            j.key,
            ICLPoolManager.SwapParams({
                zeroForOne: j.zeroForOne,
                amountSpecified: -int256(uint256(j.amountIn)),                       // exact input
                sqrtPriceLimitX96: j.zeroForOne ? TickMath.MIN_SQRT_RATIO + 1 : TickMath.MAX_SQRT_RATIO - 1
            }),
            j.hookData);

        int128 aIn  = j.zeroForOne ? d.amount0() : d.amount1();   // <= 0
        int128 aOut = j.zeroForOne ? d.amount1() : d.amount0();   // >= 0
        uint256 spent = uint256(uint128(-aIn));
        uint256 got = uint256(uint128(aOut));
        require(got >= j.minOut, "slippage");

        // pay what we owe
        vault.sync(cin);
        if (Currency.unwrap(cin) == address(0)) {
            vault.settle{value: spent}();
        } else {
            IERC20(Currency.unwrap(cin)).transfer(address(vault), spent);
            vault.settle();
        }
        // collect what we are owed
        vault.take(cout, j.recipient, got);
        return abi.encode(got);
    }
    receive() external payable {}
}
```
Notes: (a) After `lock` returns, any unspent input (partial fill) stays in the adapter; refund it. (b) Safer than trusting the returned delta with hook pools: assert `vault.currencyDelta(address(this), cout)` equals what you take, and settle `-vault.currencyDelta(address(this), cin)`. (c) For HYPE->quote->token, run two swaps in ONE lock callback (second input = first output; do not `take` the intermediate, deltas net out) or two separate locks. (d) Wrap the whole `vault.lock` in try/catch in FerryIn; the lock reverts atomically so a failed swap leaves no state.

### 1.5 Addresses and the Ascend hook
- Vault on Elysium: `0x2B738A9A69b7a45C9dA6B519f6bE6efBfb912aec` (from details.md; ASSUMED correct, unverified).
- **CLPoolManager address on Elysium: NOT FOUND** (RPC/explorer unreachable). Read it from `poolKey.poolManager` in any Ascend launch event/tx, or from the launchpad getter (see 2.3). `Vault.registerApp` set of apps would also show it (`AppRegistered(address)` event on the Vault: scan logs of the Vault for topic0 `keccak("AppRegistered(address)")`).
- **hookData for the Ascend hook: UNKNOWN.** details.md says the hook does not restrict swappers. ASSUMED: pass `""` (empty). Verify by an `eth_call` of the adapter against a real pool; if it reverts, fetch `AscendLaunchpad` verified source (hook callbacks `beforeSwap/afterSwap`) and look for `hookData` decoding. The hook may have BEFORE/AFTER_SWAP_RETURNS_DELTA (fee taking); then trust the returned `delta` for the swapper, and always enforce `minOut` on the realized amount.

---

## 2. AscendLaunchpad, WHYPE, Lodestar, HYPE/USDC pool (all UNVERIFIED: host unreachable)

### 2.1 What could not be fetched
`GET https://testnet-api.elysium.kinetiq.xyz/api/v2/smart-contracts/0x6d01f6644bbf1b5074472D3e85aa1465B4d2bF3d` returned ENOTFOUND/403 from here. Therefore the launch event name, topic0, `LaunchParams`, creator functions, WHYPE ABI, Lodestar ABI and any HYPE/USDC pool are **not known**. Do NOT hardcode guesses; build discovery into the keeper/indexer.

### 2.2 Discovery procedure (run from a machine with network)
```bash
A=0x6d01f6644bbf1b5074472D3e85aa1465B4d2bF3d
B=https://testnet-api.elysium.kinetiq.xyz/api/v2
curl -s $B/smart-contracts/$A | jq '{name,implementations,proxy_type,abi:(.abi|map(select(.type=="event" or .type=="function")|{type,name,inputs:(.inputs|map({name,type,indexed,components}))}))}' > launchpad.abi.json
# if it is a proxy, read .implementations[0].address and fetch /smart-contracts/<impl>
curl -s "$B/addresses/$A/logs" | jq '.items[0:5]'            # first events: find the launch event topic0
curl -s $B/smart-contracts/0xCD57F65c2B0e5881CFC2E609F7CD53b746E1F234 | jq '.abi[]|select(.name=="deposit" or .name=="withdraw")'
curl -s $B/smart-contracts/0xdF26E0b534474F3b623bceE7d210c134d9677377 | jq '.abi|map(select(.type=="function")|.name)'
```
Or with `cast` against the Elysium RPC: `cast call WHYPE "name()(string)"`; test WHYPE.deposit by `cast call --value 1 WHYPE 0xd0e30db0` (selector of `deposit()` = **0xd0e30db0**, VERIFIED(calc)) and also try a plain value transfer to the address (WETH9 fallback = deposit).

### 2.3 Expected shapes (ASSUMED; confirm against the ABI file above)
- Launch event: the indexer must discover it by decoding the ABI event whose args contain a `PoolKey`-like tuple or `(token, creator)` pair. Compute `topic0 = keccak256(signature)` from the ABI (`viem.toEventSelector`). Pool key is likely either in the event as a tuple `(address,address,address,address,uint24,bytes32)` or reconstructable: `currency0/1 = sort(token, quote)`, `hooks = launchpad`, `poolManager = CLPoolManager`, `fee = 10000` (1%), `parameters = hooksBitmap | tickSpacing<<16`. If not in the event, fetch the launch tx and decode the `Initialize` event of the CLPoolManager: `event Initialize(PoolId indexed id, Currency indexed currency0, Currency indexed currency1, IHooks hooks, uint24 fee, bytes32 parameters, uint160 sqrtPriceX96, int24 tick)` (VERIFIED(src) `pool-cl/interfaces/ICLPoolManager.sol`-ish; check exact in repo before coding) which gives the whole PoolKey (poolManager = emitter address).
- Quote currency of a pool = whichever of currency0/currency1 is not the launched token.
- Creator transfer: `transferCreator` / `acceptCreator` / `collect` / `claim` names come from details.md's description of Lodestar; arguments unknown until the ABI is read.
- Treat `AscendLaunchpad` address as config (details.md notes it was redeployed twice).

### 2.4 HYPE -> quote-currency routing
Unknown whether any HYPE/USDC pool exists on Elysium. Plan: indexer checks CLPoolManager `Initialize` events for any pool whose currencies are {WHYPE or native, an Ascend mock USDC / canonical USDC}. If none, deploy a labelled seed pool (details.md Kill test 4). The two mock USDCs mean each token's route must start from its own `quoteCurrency`, so a HYPE->USDC_A and HYPE->USDC_B hop may both be needed.

---

## 3. Arbitrum Nitro

### 3.1 Inbox (ETH-native; `nitro-contracts/src/bridge/IInbox.sol`, `Inbox.sol`, `AbsInbox.sol`) VERIFIED(src)
```solidity
function createRetryableTicket(
    address to, uint256 l2CallValue, uint256 maxSubmissionCost,
    address excessFeeRefundAddress, address callValueRefundAddress,
    uint256 gasLimit, uint256 maxFeePerGas, bytes calldata data
) external payable returns (uint256);                      // selector 0x679b6ded (calc)
function unsafeCreateRetryableTicket(<same args>) external payable returns (uint256); // 0x6e6e8a6a
function calculateRetryableSubmissionFee(uint256 dataLength, uint256 baseFee) external view returns (uint256); // 0xa66b327d
//   = (1400 + 6 * dataLength) * (baseFee == 0 ? block.basefee : baseFee)   (Inbox.sol l.314)
function depositEth() external payable returns (uint256);                              // 0x439370b1
event InboxMessageDelivered(uint256 indexed messageNum, bytes data);   // topic0 0xff64905f73a67fb594e0f940a8075a860db489ad991e032f48c81123eb52d60b
event MessageDelivered(uint256 indexed messageIndex, bytes32 indexed beforeInboxAcc, address inbox, uint8 kind, address sender, bytes32 messageDataHash, uint256 baseFeeL1, uint64 timestamp); // 0x5e3c1311...
```
(Note: `InboxMessageDelivered`'s first param is indexed in the real interface; the hash above was computed on the plain signature, which is what topic0 uses regardless of `indexed`.)
Rules:
- `msg.value` must be `>= maxSubmissionCost + l2CallValue + gasLimit * maxFeePerGas` else `InsufficientValue`. `maxSubmissionCost >= calculateRetryableSubmissionFee(data.length, 0)` evaluated with the **HyperEVM** block.basefee at call time, else `InsufficientSubmissionCost`. Use a 3x-5x margin on submission cost (ArbOS recomputes with the parent basefee at delivery; excess is refunded).
- `gasLimit == 1` or `maxFeePerGas == 1` triggers revert `RetryableData(...)` (a way to surface args via eth_call); never use 1.
- `onlyAllowed`: `if (allowListEnabled && !isAllowed[tx.origin]) revert NotAllowedOrigin`. Applies to depositEth too.
- Refund addresses that are contracts get aliased on the L2 side automatically (`createRetryableTicket`); EOAs are not touched. Use `unsafeCreateRetryableTicket` only if you need no up-front submission-cost check.
- Message number returned = `messageNum`; the retryable ticket id on L2 is `keccak256(abi.encode(chainId, bytes32(messageNum) as uint, ...))` (derive via `ArbRetryableTx.TicketCreated` / from Blockscout `to-rollup` message instead of computing).
- **ERC20Inbox (custom gas token) has the same function names but different semantics (`amount` param, no msg.value).** Elysium uses HYPE as native gas and HYPE is the native coin on HyperEVM, so the plain ETH `Inbox` (payable) is expected. Confirm: `cast call INBOX "bridge()"` then bridge `nativeToken()` reverts or returns 0x0 for ETH-based rollups (ASSUMED ETH-based; Chainstack/web search says "HYPE is gas token, 1:1 bridge", SEARCH).

### 3.2 Address aliasing and execution (VERIFIED(src) `libraries/AddressAliasHelper.sol`)
`alias(addr) = addr + 0x1111000000000000000000000000000000001111 (mod 2^160)`. Retryable auto-redeem runs on L2 as a normal tx: `tx.origin = msg.sender = L1 sender` (aliased if the L1 caller is a contract; ASSUMED an EOA caller is NOT aliased per Arbitrum docs), `msg.value = l2CallValue`, `to = target`, calldata `data`, gas = `gasLimit`. FerryIn does no sender auth, so aliasing is irrelevant (`details.md`).
ArbOS flow (docs, ASSUMED from Arbitrum docs, not in this repo): (1) full `msg.value` is minted to the sender (From) on L2; (2) submission fee deducted and refunded excess goes to `excessFeeRefundAddress`; (3) `l2CallValue` moved into the ticket escrow; (4) if `From` balance covers `gasLimit * maxFeePerGas` and maxFeePerGas >= L2 basefee, ArbOS auto-schedules the redeem right away; unused gas cost refunded to `excessFeeRefundAddress`; (5) the retry tx moves `l2CallValue` from escrow to `to` and calls it.
**If the L2 call reverts:** the `l2CallValue` returns to the ticket escrow and the ticket stays redeemable (by anyone via `ArbRetryableTx.redeem(ticketId)`, paying L2 gas) for the ticket lifetime (7 days, `getLifetime()`; `keepalive` extends). After expiry/cancel the escrow goes to `callValueRefundAddress` (the beneficiary). If auto-redeem is not scheduled (gas deposit insufficient or maxFeePerGas < basefee) the same state applies. This is why FerryIn must catch internally and refund rather than revert; and why `callValueRefundAddress` must be the user.

### 3.3 Gas estimation (`node-interface/NodeInterface.sol`, precompile address `0x00000000000000000000000000000000000000C8` on the child chain) VERIFIED(src) for signature, ASSUMED for usage
```solidity
function estimateRetryableTicket(address sender, uint256 deposit, address to, uint256 l2CallValue,
    address excessFeeRefundAddress, address callValueRefundAddress, bytes calldata data) external;   // 0xc3dc5879
function gasEstimateComponents(address to, bool contractCreation, bytes calldata data) external payable
    returns (uint64 gasEstimate, uint64 gasEstimateForL1, uint256 baseFee, uint256 l1BaseFeeEstimate); // 0xc94e6eeb
```
Use: `eth_estimateGas` on the **Elysium RPC** with `to=0xC8`, `data=estimateRetryableTicket(...)`; the returned gas number is the retryable's redeem gas (it simulates the whole submit+redeem; `deposit` should be large, e.g. 1 HYPE). Add 30% buffer. `maxFeePerGas` = 2x `eth_gasPrice` on Elysium (retryable gas is paid at the L2 basefee, excess refunded). Total `msg.value` = `l2CallValue + maxSubmissionCost + gasLimit*maxFeePerGas`. Simpler fallback: simulate `FerryIn.buy` with `eth_estimateGas` on Elysium from the user's address with value, then 1.5x.

### 3.4 Outbox (`bridge/IOutbox.sol`) VERIFIED(src)
```solidity
function executeTransaction(bytes32[] calldata proof, uint256 index, address l2Sender, address to,
    uint256 l2Block, uint256 l1Block, uint256 l2Timestamp, uint256 value, bytes calldata data) external;
// canonical string: executeTransaction(bytes32[],uint256,address,address,uint256,uint256,uint256,uint256,bytes)
// selector = 0x08635a95   <-- CONFIRMED by keccak with viem
event OutBoxTransactionExecuted(address indexed to, address indexed l2Sender, uint256 indexed zero, uint256 transactionIndex); // 0x20af7f3b...
```
`to` is arg index 3 (0-based) of the ABI args; `value` arg 7. Anyone may call. The Outbox forwards `value` from the bridge to `to` via a call with `data` (empty for withdrawEth). Decode `to` from calldata with `decodeFunctionData`, not byte offsets (proof is dynamic).

### 3.5 ArbSys (0x64) and ArbRetryableTx (0x6E) (npm `@arbitrum/nitro-contracts@1.3.0/src/precompiles/*`) VERIFIED(src)
```solidity
function withdrawEth(address destination) external payable returns (uint256);        // 0x25e16063; msg.value is withdrawn
function sendTxToL1(address destination, bytes calldata data) external payable returns (uint256); // 0x928c169a
event L2ToL1Tx(address caller, address indexed destination, uint256 indexed hash, uint256 indexed position,
                uint256 arbBlockNum, uint256 ethBlockNum, uint256 timestamp, uint256 callvalue, bytes data);
// topic0 = 0x3e7aafa77dbf186b7fd488006beff893744caa3c4f6f299e8a709fa2087374fc  (calc, signature L2ToL1Tx(address,address,uint256,uint256,uint256,uint256,uint256,uint256,bytes))
// ArbRetryableTx
function redeem(bytes32 ticketId) external returns (bytes32);   // 0xeda1122c
function getTimeout(bytes32) view returns (uint256); function getLifetime() view returns (uint256);
function keepalive(bytes32) returns (uint256); function getBeneficiary(bytes32) view returns (address); function cancel(bytes32) ;
event TicketCreated(bytes32 indexed ticketId);                                  // 0x7c793cce...
event RedeemScheduled(bytes32 indexed ticketId, bytes32 indexed retryTxHash, uint64 indexed sequenceNum, uint64 donatedGas, address gasDonor, uint256 maxRefund, uint256 submissionFeeRefund); // 0x5ccd0095...
event Redeemed(bytes32 indexed userTxHash);                                     // 0x27fc6cca... (deprecated)
```
`withdrawEth` from a contract: `caller` in L2ToL1Tx = the calling contract (ExitDesk), NOT aliased. `position` = leaf index (this is the message `id`). `destination` is the HyperEVM address (our Receiver) and Blockscout's `origination_address_hash` for the message is the `caller`, so Rescue must look at the Outbox `to`/destination not `origination_address_hash` for ownership: for `withdrawEth` the destination is the owner, for Rescue use `/arbitrum/messages/withdrawals/{txhash}` (has `destination_address_hash`, `callvalue`).

---

## 4. Blockscout Arbitrum endpoints (derived from source; live shapes UNVERIFIED)

Source: `blockscout/apps/block_scout_web/lib/block_scout_web/{routers/api_router.ex, controllers/api/v2/arbitrum_controller.ex, views/api/v2/arbitrum_view.ex, schemas/api/v2/arbitrum/*.ex}`, `apps/explorer/lib/explorer/arbitrum/claim_rollup_message.ex`. The deployed instance may be an older version; parse defensively (accept both `outbox_address_hash` and `outbox_address`).

Routes (all GET under `/api/v2/arbitrum`): `/messages/{from-rollup|to-rollup}`, `/messages/{dir}/count`, `/messages/claim/{message_id}`, `/messages/withdrawals/{tx_hash}`, `/batches`, `/batches/count`, `/batches/{n}`, plus `/api/v2/main-page/arbitrum/messages/to-rollup`, `/main-page/arbitrum/batches/committed`, `/main-page/arbitrum/batches/latest-number`.

### 4.1 messages list
Query params: `id` (paging cursor; returns messages with `id < cursor`), no status/address filters exist. Order: `id` descending. Page size 50. Next page: `next_page_params` is `{"id": <last id on the page>}` (null when exhausted); pass it as `?id=<n>`.
```json
{
  "items": [
    {
      "id": 573261,
      "origination_address_hash": "0x…",          // for from-rollup: the L2ToL1Tx `caller`
      "origination_transaction_hash": "0x…",
      "origination_timestamp": "2026-10-17T12:34:56.000000Z",   // ISO-8601 (ASSUMED format), nullable
      "origination_transaction_block_number": 1234567,           // nullable, origin-chain block
      "completion_transaction_hash": null,                       // set once relayed (executed on HyperEVM)
      "status": "confirmed"                                      // initiated | sent | confirmed | relayed
    }
  ],
  "next_page_params": { "id": 573212 }
}
```
Status meaning: `initiated` (L2ToL1Tx emitted) -> `sent` (batch posted to parent) -> `confirmed` (assertion confirmed in Outbox; claimable) -> `relayed` (executed). For to-rollup only `initiated` and `relayed` apply. Rescue selector: `status == "confirmed" && completion_transaction_hash == null && age >= 24h`. Statuses `initiated/sent` in the DB may lag; `claim/{id}` re-checks via RPC.
`/messages/{dir}/count` returns a bare integer.

### 4.2 claim
`GET /api/v2/arbitrum/messages/claim/573261`
```json
{ "calldata": "0x08635a95…", "outbox_address_hash": "0x87391D602aaffB3Fe8051EcCDb75d6EEe8C31060" }
```
(INTERFACES.md says `outbox_address`; current source says `outbox_address_hash`. Accept both.) Errors: 400 `{"message":"withdrawal is unconfirmed yet"}` (status sent/initiated), 400 `{"message":"withdrawal was executed already"}` (relayed), 404 `{"message":"cannot find requested withdrawal"}`, 404 `{"message":"internal error occurred"}`. The calldata is a complete `Outbox.executeTransaction(...)` call (selector 0x08635a95), proof included, so no proof generation needed. Decode `to` (index 3) to check it equals the receiver.

### 4.3 withdrawals by tx
`GET /api/v2/arbitrum/messages/withdrawals/{l2TxHash}` -> `{"items":[{ "id":int, "status":"…", "caller_address_hash":"0x…", "destination_address_hash":"0x…", "arb_block_number":int, "eth_block_number":int, "l2_timestamp":int(unix?), "callvalue":"<wei string>", "data":"0x…", "token":null|{…}, "completion_transaction_hash":null|"0x…" }]}`. Use this to map an ExitIntent tx to its message `id`.

### 4.4 batches
`GET /api/v2/arbitrum/batches` params: `number` (cursor), `batch_numbers` (array filter). Next: `{"number": <n>}`.
```json
{ "items":[ { "number":123, "transactions_count":10, "blocks_count":50,
   "batch_data_container":"in_blob4844|in_calldata|in_anytrust|…"|null,
   "commitment_transaction":{ "hash":"0x…","block_number":123,"timestamp":"…","status":"unfinalized|finalized" } } ],
  "next_page_params":{"number":122} }
```
`GET /batches/{n}` returns `{number, transactions_count, start_block_number, end_block_number, before_acc_hash, after_acc_hash, commitment_transaction:{hash,block_number,timestamp,status}, (+DA fields)}`. Field names VERIFIED(src); enum value strings ASSUMED.

### 4.5 address logs (v2)
`GET /api/v2/addresses/{addr}/logs?topic=0x…(optional first-topic filter)&block_number=..&index=..` (cursor). Response:
```json
{ "items":[ { "transaction_hash":"0x…", "address":{"hash":"0x…","name":null,…}, "topics":["0x<topic0>","0x…",null,null],
   "data":"0x…", "index":12, "decoded":{ "method_call":"Event(…)","method_id":"…","parameters":[{"name":"x","type":"uint256","indexed":false,"value":"…"}] } | null,
   "smart_contract":{…}|null, "block_number":123, "block_hash":"0x…", "block_timestamp":"…" } ],
  "next_page_params":{"block_number":123,"index":12,"items_count":50} | null }
```
VERIFIED(src) `views/api/v2/transaction_view.ex#prepare_log` and address controller `logs` (supports `topic` query param). For indexing use JSON-RPC `eth_getLogs` on the Elysium RPC as primary (reliable, block ranges) and Blockscout only as fallback.

---

## 5. HyperEVM testnet (mostly SEARCH/ASSUMED; no live access)
- RPC `https://rpc.hyperliquid-testnet.xyz/evm`, chainId 998, HYPE 18 decimals: SEARCH (Chainlink/Alchemy snippets). Explorer `https://testnet.purrsec.com` (SEARCH, Chainlink doc).
- System address `0x2222222222222222222222222222222222222222`: send HYPE as native `value` (EOA or contract); its payable `receive` emits `Received` with the sender, HyperCore credits the **sender's spot HYPE balance**. SEARCH (Hyperliquid docs snippet). Gas cost of the transfer is small; sending from a contract that passes the 2300 gas stipend might fail (use `call{value:}`). The system-address transfer needs the sender account to exist on HyperCore (a new address is auto-created; ASSUMED).
- HyperCore balance check (ASSUMED URL, standard body; SEARCH confirms the body shape):
```bash
curl -s https://api.hyperliquid-testnet.xyz/info -H 'content-type: application/json' \
  -d '{"type":"spotClearinghouseState","user":"0xUSER"}'
# -> {"balances":[{"coin":"HYPE","token":1105,"total":"1.5","hold":"0.0","entryNtl":"0.0"}, …]}   (token index on testnet may differ; match by coin name)
```
- Blocks (SEARCH, third-party): small blocks ~2-3M gas about every second; big blocks 30M gas about every minute for deploys. A txn is routed by the account flag set with exchange action `{"type":"evmUserModify","usingBigBlocks":true}` (ASSUMED name). Deploy ExitFactory/Receiver (small contracts) in small blocks; any deploy over ~2-3M gas needs big blocks. The Inbox call is cheap so no issue. HyperEVM basefee can be very low and spike; use `eth_gasPrice` x1.5.
- Foundry: pass `--legacy` if EIP-1559 fee estimation misbehaves; set `--gas-estimate-multiplier 150`. ASSUMED.

---

## 6. Elysium
- Chain id 99801, Arbitrum Orbit (Nitro/ArbOS), settles to HyperEVM testnet 998, HYPE gas, bridged 1:1: SEARCH (Chainstack snippets).
- Block time: sources disagree; design target 100-200 ms, observed 3-4 blocks/s (SEARCH). Do not rely on it; poll with `eth_getTransactionReceipt`.
- ETH-style vs ERC20 inbox: native HYPE on both chains implies the standard payable `Inbox` (ASSUMED); verify as in 3.1.
- Retryable docs from Kinetiq: not retrievable. `confirmPeriodBlocks`: not published/found (details.md measured ~23-25 min to claimable on testnet). Mainnet not live as of Oct 2026 (SEARCH).

---

## Implications for implementation

1. **InfinitySwapAdapter**: implement `ILockCallback.lockAcquired(bytes)`; guard with `msg.sender == vault`; exact input is `amountSpecified = -int256(amountIn)`; limit `zeroForOne ? TickMath.MIN_SQRT_RATIO + 1 : TickMath.MAX_SQRT_RATIO - 1`; derive `zeroForOne = (tokenIn == key.currency0)`; pay with `sync -> transfer -> settle` for ERC20, `settle{value}` for native; collect with `take(cout, recipient, got)`; require `got >= minOut`; refund unspent input after the lock. Use only `vault.lock` wrapped in try/catch from FerryIn.
2. Decode `BalanceDelta` as amount0 = upper 128 bits (`sar(128,x)`), amount1 = lower 128 (`signextend(15,x)`); in Go: `a0 = int128(new(big.Int).Rsh(x,128))`, `a1 = sign-extend low 128`.
3. Never hardcode CLPoolManager: take `poolKey.poolManager` from the indexed launch data; keep PoolKey tuple order `(currency0,currency1,hooks,poolManager,fee,parameters)` in Go/TS ABI. Native currency is `address(0)`; check whether quote/HYPE legs are native or WHYPE before wrapping.
4. First action for a coder with network access: fetch the Ascend ABI (2.2), WHYPE `deposit()` (`0xd0e30db0`) and Initialize logs; then fill `deployments/testnet.json` plus a `ascend.json` of `{launchEventTopic0, poolKeyLocation}`. Until then code the indexer against `eth_getLogs` on the launchpad address with the ABI loaded from a JSON file.
5. **FerryIn gas**: `maxFeePerGas = 2 * eth_gasPrice(Elysium)`; `gasLimit = 1.3 * eth_estimateGas(NodeInterface 0xC8 estimateRetryableTicket)` (or 1.5x of a direct estimate, min 1.5M for a two-hop swap); `maxSubmissionCost = 4 * calculateRetryableSubmissionFee(data.length, 0)` read on HyperEVM; `value = l2CallValue + maxSubmissionCost + gasLimit*maxFeePerGas`; `excessFeeRefundAddress = callValueRefundAddress = user`; `to = FerryIn`; calldata `buy(token, recipient, minOut)`. `l2CallValue` = HYPE the user wants to spend.
6. FerryIn must never revert on swap failure (try/catch -> send `msg.value` to `recipient`), because a revert strands `l2CallValue` in the ticket escrow for 7 days; keeper calls `ArbRetryableTx.redeem(ticketId)` (`0xeda1122c`) for tickets whose auto-redeem failed (watch `TicketCreated`, `RedeemScheduled`).
7. **Keeper claim parsing** (Go): struct `{Calldata string json:"calldata"; OutboxAddressHash string json:"outbox_address_hash"; OutboxAddress string json:"outbox_address"}`; use whichever is non-empty. Treat HTTP 400 "already executed" as success/skip, 400 "unconfirmed yet" as retry later. Verify `calldata[:10]=="0x08635a95"` and decoded `to` equals the expected receiver before sending.
8. Rescue discovery: page `GET /arbitrum/messages/from-rollup?id=<cursor>` following `next_page_params.id` until `items` older than the horizon; filter `status=="confirmed" && completion_transaction_hash==null && now-origination_timestamp>=24h`; resolve the owner via `/messages/withdrawals/{origination_transaction_hash}` -> `destination_address_hash` (and `callvalue`), because `origination_address_hash` is the L2 caller, not the L1 recipient. Always simulate (`eth_call`/`executeTransactionSimulation`-like) first; claims by anyone pay `to`, so safe.
9. Exit tracking: map `ExitIntent` tx hash -> message id with `/messages/withdrawals/{txHash}` (match `destination_address_hash == receiver`); decode `L2ToL1Tx` (topic0 `0x3e7aafa7…74fc`, `position` = message id) from the receipt as a no-API fallback.
10. Fix INTERFACES.md section 4: field is `outbox_address_hash` (current Blockscout); cursor params are `id` (messages) and `number` (batches); messages carry `id, origination_address_hash, origination_transaction_hash, origination_timestamp, origination_transaction_block_number, completion_transaction_hash, status`.
11. HyperEVM deployment: contracts are small; if a deploy fails on gas, enable big blocks via `evmUserModify` first. HyperCore landing: plain `call{value}` or EOA transfer to `0x2222…2222`; verify with `spotClearinghouseState` on `https://api.hyperliquid-testnet.xyz/info`.
12. Do the kill tests listed in details.md early: (a) is Inbox allowlist on for the demo EOA, (b) ExitDesk contract `withdrawEth`, (c) WHYPE deposit and the real hookData behavior.
