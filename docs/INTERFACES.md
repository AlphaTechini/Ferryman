# Ferryman: component interfaces

This is the contract between the three parts of the repo. `details.md` is the product spec; this file pins the exact shapes so the contracts, the Go keeper and the Svelte app agree.

```
contracts/   Foundry. Solidity 0.8.28. Immutable, ownerless, no proxies.
keeper/      Go 1.24. One binary: indexer + quote API + claimer + Rescue.
web/         Svelte 5 + Vite + TypeScript + viem. pnpm.
deployments/testnet.json   Addresses written after deploy; read by keeper and web.
```

## 1. Chains

| | HyperEVM testnet | Elysium testnet |
|---|---|---|
| chainId | 998 | 99801 |
| RPC | `https://rpc.hyperliquid-testnet.xyz/evm` | `https://testnet-rpc.elysium.kinetiq.xyz` |
| Explorer | `https://testnet.purrsec.com` | `https://elysium.kinetiq.xyz/testnet-explorer` |
| Explorer API | n/a | `https://testnet-api.elysium.kinetiq.xyz/api/v2/` |
| Native | HYPE (18) | HYPE (18) |

Do not use viem's built-in `elysiumTestnet` (that is chain 1338). Define both chains with `defineChain`.

## 2. Contracts

### 2.1 Shared struct (`contracts/src/libraries/ExitParams.sol`)

```solidity
struct ExitParams {
    uint256 chainId;    // Elysium chain id (99801)
    address desk;       // ExitDesk that started the exit
    address sender;     // msg.sender of exitHYPE
    uint256 nonce;      // per-sender nonce on the desk (0,1,2,...)
    uint256 amount;     // HYPE sent to ArbSys.withdrawEth
    uint256 claimTip;   // paid to whoever runs claimAndSweep, <= amount
    address recipient;  // HyperEVM address that receives amount - claimTip
    bool toCore;        // reserved: UI offers HyperCore landing; contracts always pay recipient on HyperEVM
}
library ExitParamsLib {
    function salt(ExitParams memory p) internal pure returns (bytes32) { return keccak256(abi.encode(p)); }
    // receiver = address(uint160(uint256(keccak256(abi.encodePacked(bytes1(0xff), factory, salt(p), receiverInitHash)))))
}
```

`receiverInitHash = keccak256(abi.encodePacked(type(Receiver).creationCode, abi.encode(factory)))`. ExitDesk takes `(exitFactory, receiverInitHash)` as constructor immutables. ExitFactory exposes `receiverInitHash()` and `receiverFor(ExitParams)`. A Foundry test must assert parity between ExitDesk.previewReceiver and ExitFactory.receiverFor.

### 2.2 Elysium

```solidity
// ExitDesk
event ExitIntent(bytes32 indexed salt, address indexed receiver, address indexed recipient, ExitParams params);
function nonces(address sender) external view returns (uint256);
function previewReceiver(address sender, address recipient, uint256 amount, uint256 claimTip, bool toCore)
    external view returns (ExitParams memory params, address receiver);
function exitHYPE(address recipient, uint256 claimTip, bool toCore) external payable returns (address receiver);
// requires msg.value > 0, claimTip <= msg.value, recipient != 0.
// Calls ArbSys(0x64).withdrawEth{value: msg.value}(receiver).

// FerryIn
event Bought(address indexed token, address indexed recipient, uint256 hypeIn, uint256 fee, uint256 amountOut);
event Refunded(address indexed token, address indexed recipient, uint256 hypeIn, bytes reason);
function buy(address token, address recipient, uint256 minOut) external payable;
// fee = msg.value * 30 / 10_000 -> FeeJar. Rest swapped via adapter inside try/catch. On failure the
// full msg.value (fee included) is refunded to recipient. Never reverts on swap failure.
function quote(address token, uint256 hypeIn) external returns (uint256 amountOut); // for eth_call only

// FeeJar: receive() payable; immutable beneficiary (fee router). release() is permissionless and sends balance to beneficiary.
// InfinitySwapAdapter: see contracts/README.md (route = HYPE -> quote currency -> token through Infinity CL pools).
```

### 2.3 HyperEVM

```solidity
// ExitFactory(address outbox)
event Claimed(bytes32 indexed salt, address indexed receiver, address indexed recipient, uint256 paid, uint256 tip, address keeper);
function claimAndSweep(bytes calldata outboxCalldata, ExitParams calldata p) external returns (uint256 paid);
// 1. require outboxCalldata selector == 0x08635a95 (Outbox.executeTransaction) and its `to` arg == receiverFor(p).
// 2. Call outbox. If it reverts (already spent by someone else), continue only if receiver balance >= p.amount.
// 3. Deploy Receiver via CREATE2 if missing. 4. Mark salt swept (second call reverts AlreadySwept).
// 5. Receiver pays p.recipient (p.amount - p.claimTip) and msg.sender p.claimTip.
function sweep(ExitParams calldata p) external;  // after claim: sends any extra balance in the receiver to p.recipient
function receiverFor(ExitParams calldata p) external view returns (address);
function isSwept(bytes32 salt) external view returns (bool);

// Receiver: immutable factory; receive() external payable {}; pull(address to, uint256 amt) onlyFactory. No selfdestruct.
```

## 3. Keeper HTTP API (Go, default `:8787`, JSON, CORS `*`)

All amounts are decimal strings in wei. Addresses are checksummed. Times are unix seconds.

| Method | Path | Response |
|---|---|---|
| GET | `/api/health` | `{ "ok": true, "elysiumBlock": 123, "hyperevmBlock": 456, "keeper": "0x..", "dryRun": false }` |
| GET | `/api/config` | `{ "elysium": {chainId, rpc, explorer}, "hyperevm": {...}, "contracts": {ferryIn, exitDesk, exitFactory, feeJar, inbox, outbox}, "feeBps": 30, "defaultSlippageBps": 150, "defaultClaimTip": "..." }` |
| GET | `/api/tokens` | `Token[]` |
| GET | `/api/quote?token=0x..&amountIn=<wei>&slippageBps=150` | `Quote` |
| GET | `/api/ferry-in/{hyperevmTxHash}` | `FerryInStatus` |
| GET | `/api/exits?address=0x..` | `Exit[]` (as sender or recipient) |
| GET | `/api/exits/{receiver}` | `Exit` |
| GET | `/api/rescue/stats` | `RescueStats` |
| GET | `/api/rescue/lookup?address=0x..` | `StuckWithdrawal[]` |
| POST | `/api/rescue/claim` body `{ "messageId": 573261 }` | `{ "status": "queued" \| "claimed" \| "not_claimable", "txHash": "0x.." \| null }` |

```ts
type Token = { address: string; name: string; symbol: string; decimals: number; launcher: string;
  launchTx: string; launchBlock: number; launchedAt: number; quoteCurrency: string; quoteSymbol: string;
  poolKey: { currency0: string; currency1: string; hooks: string; poolManager: string; fee: number; parameters: string };
  priceHype: string | null; /* token price in HYPE, decimal string, display only */ ourLaunch: boolean };

type Quote = { token: string; amountIn: string; fee: string; amountOut: string; minOut: string;
  slippageBps: number; priceImpactBps: number; route: string[]; simulatedAtBlock: number;
  retryable: { gasLimit: string; maxFeePerGas: string; maxSubmissionCost: string; totalValue: string } };

type FerryInStatus = { hyperevmTx: string; stage: 'submitted' | 'relayed' | 'redeemed' | 'bought' | 'refunded' | 'redeem_failed';
  messageId: number | null; elysiumTx: string | null; token: string | null; recipient: string;
  hypeIn: string; amountOut: string | null; updatedAt: number };

type Exit = { receiver: string; salt: string; params: { chainId: number; desk: string; sender: string; nonce: string;
  amount: string; claimTip: string; recipient: string; toCore: boolean };
  stage: 'intent' | 'confirmed' | 'claimed' | 'delivered'; elysiumTx: string; messageId: number | null;
  startedAt: number; maturesAt: number | null; claimTx: string | null; deliveredAt: number | null };

type RescueStats = { withdrawals: number; owners: number; hypeReturned: string; gasSpent: string;
  lastRunAt: number; recent: { messageId: number; owner: string; amount: string; claimTx: string; claimedAt: number }[] };

type StuckWithdrawal = { messageId: number; owner: string; amount: string; originTx: string;
  status: 'initiated' | 'sent' | 'confirmed' | 'relayed'; ageSeconds: number; claimable: boolean };
```

## 4. Explorer API (Blockscout, Arbitrum-flavoured) used by the keeper

- `GET /api/v2/arbitrum/messages/from-rollup?…` paged (`next_page_params`), items carry `id`, `status`, `origination_transaction_hash`, `origination_timestamp`, `completion_transaction_hash`, `origination_address`.
- `GET /api/v2/arbitrum/messages/to-rollup` for retryables.
- `GET /api/v2/arbitrum/messages/claim/{id}` returns ready-made Outbox calldata (`{ "calldata": "0x08635a95…", "outbox_address": "0x…" }`).
- Exact field names are recorded in `docs/RESEARCH.md`.

## 5. Deployments file

`deployments/testnet.json`
```json
{ "elysium": { "chainId": 99801, "ferryIn": "0x0", "exitDesk": "0x0", "feeJar": "0x0", "swapAdapter": "0x0", "whype": "0x0" },
  "hyperevm": { "chainId": 998, "exitFactory": "0x0", "inbox": "0x11178Df40CBf87a4343f4B3F8B09B366ceFA8Ec0", "outbox": "0x87391D602aaffB3Fe8051EcCDb75d6EEe8C31060" } }
```
