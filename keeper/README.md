# Ferryman keeper

One Go binary that runs the backend for Ferryman on the Ascend testnet:

- **Indexer.** Scans Elysium for Ascend pools (CLPoolManager `Initialize` events whose hook is the AscendLaunchpad) and for ExitDesk `ExitIntent` events. Stores the token list, pool keys, launchers and exits.
- **Ferry Out claimer.** For each exit, finds the Outbox message, waits for confirmation, verifies the explorer's claim calldata, then runs `ExitFactory.claimAndSweep`. The user signs once, on Elysium.
- **Rescue.** Claims matured withdrawals that nobody claimed, and pays each one to its original destination.
- **Ferry In.** Quotes buys (an `eth_call` of `FerryIn.quote` on Elysium plus retryable gas parameters), tracks tickets from HyperEVM to Elysium, and retries failed redeems when a ticket id is known.
- **HTTP API.** The shapes in `docs/INTERFACES.md` section 3, served with `net/http`.

Everything runs in one process with graceful shutdown (SIGINT or SIGTERM). Every transaction is simulated with `eth_call` before it is signed and sent. The keeper is idempotent across restarts: processed message ids and `isSwept` checks prevent double work.

## Layout

```
cmd/ferryman/        flags, wiring, loops, graceful shutdown
internal/config/     env + deployments/testnet.json (zero addresses disable features with a warning)
internal/abis/       embedded minimal ABIs, event decoders, Outbox calldata check, ExitParams salt, BalanceDelta
internal/explorer/   Blockscout client: message pages, withdrawals by tx, claim calldata; typed ErrUnconfirmed / ErrExecuted / ErrNotFound
internal/chain/      RPC connections (Elysium, HyperEVM) and Sender: simulate, sign (EIP-1559, legacy fallback), send, wait, track gas
internal/store/      one JSON state file, written atomically (temp file, fsync, rename), behind a RWMutex
internal/model/      HTTP wire types (exact JSON of INTERFACES.md section 3)
internal/indexer/    token list, exit intents, FerryIn token registration
internal/quote/      Ferry In quotes and retryable gas parameters
internal/claimer/    Ferry Out state machine: intent, confirmed, claimed, delivered
internal/rescue/     Rescue loop, lookup, claim-for-me
internal/ferryin/    Ferry In ticket status, redeem of failed tickets (best effort)
internal/api/        HTTP handlers, CORS, JSON errors
```

## Requirements

- Go 1.24. `go-ethereum` is pinned to v1.17.2 because its `go.mod` targets Go 1.24; v1.17.8 requires Go 1.25.
- Module dependencies: `github.com/ethereum/go-ethereum` and what it pulls in. Nothing else.

## Configuration

Read from the environment. `.env.example` lists every variable. Flags override the matching variable: `-addr`, `-deployments`, `-data-dir`, `-dry-run`.

| Variable | Default | Meaning |
|---|---|---|
| `ELYSIUM_RPC` | `https://testnet-rpc.elysium.kinetiq.xyz` | Elysium JSON-RPC (chain 99801) |
| `HYPEREVM_RPC` | `https://rpc.hyperliquid-testnet.xyz/evm` | HyperEVM JSON-RPC (chain 998) |
| `EXPLORER_API` | `https://testnet-api.elysium.kinetiq.xyz/api/v2` | Blockscout API base |
| `KEEPER_PRIVATE_KEY` | empty | Hex key. Empty means read-only: API and indexing, no claims, registrations or redeems |
| `DRY_RUN` | `false` | Simulate every transaction, send none |
| `LISTEN_ADDR` | `:8787` | HTTP listen address |
| `DATA_DIR` | `./data` | Directory for `state.json` |
| `DEPLOYMENTS` | `../deployments/testnet.json` | Contract addresses. A missing file or a zero address disables the dependent feature with a warning |
| `LAUNCHPAD` | `0x6d01f6644bbf1b5074472D3e85aa1465B4d2bF3d` | AscendLaunchpad, also the pool hook |
| `CL_POOL_MANAGER` | none | CLPoolManager on Elysium. Required for token indexing (unverified, so no default) |
| `OUR_LAUNCHERS` | empty | Comma list of launcher addresses. Tokens they launched get `ourLaunch: true` |
| `QUOTE_CURRENCIES` | empty | Extra quote currencies. Canonical USDC and the mock USDC are always included |
| `INDEX_FROM_BLOCK` | `0` | First Elysium block to scan. Set it near the launchpad deployment to save time |
| `LOG_RANGE` | `5000` | Blocks per `eth_getLogs` call |
| `DEFAULT_CLAIM_TIP_WEI` | `500000000000000` | Reported as `defaultClaimTip` (0.0005 HYPE) |
| `RESCUE_ENABLED` | `true` | Enables Rescue (needs a key and an Outbox to run claims) |
| `RESCUE_MIN_AGE` | `24h` | Minimum age of a withdrawal before Rescue claims it |
| `RESCUE_LOOKBACK` | `720h` | How far back Rescue scans (30 days) |
| `RESCUE_MAX_PER_RUN` | `10` | Maximum claim attempts per Rescue pass |
| `POLL_INTERVAL` | `20s` | Interval of the indexer, claimer, Rescue and redeem loops |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `text` | `text` or `json` |

Fixed constants: confirmations behind head = 3, protocol fee = 30 bps, default slippage = 150 bps.

## Run

```sh
make test                    # go test ./... and vet
make build                   # bin/ferryman

# read-only, against the zero-address testnet file (works offline; RPC failures are logged, not fatal)
DEPLOYMENTS=../deployments/testnet.json ./bin/ferryman

# dry run with a key: everything is simulated, nothing is sent
KEEPER_PRIVATE_KEY=0x... DRY_RUN=true CL_POOL_MANAGER=0x... ./bin/ferryman

curl -s localhost:8787/api/health
```

## Docker

```sh
make docker   # multi-stage build, distroless nonroot runtime, state in /data
docker run --rm -p 8787:8787 \
  -v ferryman-data:/data \
  -v "$PWD/../deployments/testnet.json:/etc/ferryman/testnet.json:ro" \
  -e KEEPER_PRIVATE_KEY -e CL_POOL_MANAGER -e OUR_LAUNCHERS \
  ferryman-keeper
```

The deployments file is mounted, not copied, so the image never contains addresses.

## HTTP API

All amounts are decimal strings in wei. Addresses are checksummed. Times are unix seconds. Errors are `{"error": "..."}`. CORS is `*`.

| Method | Path | Notes |
|---|---|---|
| GET | `/api/health` | `ok` is false when an RPC is unreachable; the body then has `error` and null block numbers |
| GET | `/api/config` | Chains, contracts (zero address when not deployed), fee, slippage, default tip |
| GET | `/api/tokens` | Ascend tokens, newest launch first |
| GET | `/api/quote?token&amountIn&slippageBps` | 400 bad input, 404 token not indexed, 503 FerryIn not deployed |
| GET | `/api/ferry-in/{hyperevmTxHash}` | 404 unknown or not a Ferry In ticket, 422 the HyperEVM transaction reverted |
| GET | `/api/exits?address=` | Exits where the address is sender or recipient. `address` is required |
| GET | `/api/exits/{receiver}` | 404 when unknown |
| GET | `/api/rescue/stats` | Counter and the 50 most recent claims |
| GET | `/api/rescue/lookup?address=` | Stuck withdrawals with that owner. 503 when Rescue is disabled |
| POST | `/api/rescue/claim` | Body `{"messageId": n}`. Returns `queued`, `claimed` or `not_claimable` |

`/api/health` adds `error` only on failure. `keeper` is null when there is no key.

## Rescue: why funds always reach the owner

Rescue never chooses a destination. For each candidate message it does the following.

1. It resolves the owner from the explorer's withdrawals for the origin transaction, matching the message id. The owner is `destination_address_hash`, not `origination_address_hash`, which is the L2 caller.
2. It fetches the claim calldata from the explorer and checks that the Outbox address equals the configured Outbox, that the selector is `Outbox.executeTransaction` (`0x08635a95`), that the payout target `to` equals the owner, and that the forwarded value equals the withdrawal's callvalue.
3. It sends that calldata unchanged to the Outbox, after an `eth_call` simulation. Any mismatch refuses the message and marks it processed.

The Outbox pays the destination it was given, so a valid claim can only deliver to the owner. Anyone can call the Outbox with the same calldata, so the keeper is never a trusted party for destinations.

Rescue counts are persisted. Each successful claim updates `withdrawals`, `owners` (distinct destinations), `hypeReturned` (the sum of callvalues) and `gasSpent` (gas used times effective gas price, from the receipt).

## Ferry Out: no redirection

- The receiver is a CREATE2 address derived from the exit's params. It is emitted in `ExitIntent`, and the claim calldata must pay exactly that address.
- `ExitFactory.claimAndSweep` pays `amount - claimTip` to `params.recipient` and the tip to the caller. Both are fixed by the signed exit.
- The keeper checks the explorer calldata (selector, outbox, payout target) before simulating. A refused claim is never sent.
- Before claiming, it checks `isSwept(salt)`, so restarts and repeated passes are no-ops once delivered.

## Ferry In: limitations

- A failed buy refunds HYPE to the recipient on Elysium (the contracts do not revert on swap failure). Tickets that never auto-redeem are caught by the status endpoint as `redeem_failed` after 15 minutes.
- Redeeming a failed ticket (`ArbRetryableTx.redeem`, `0x…6E`) needs its ticket id. The to-rollup list in the documented explorer shape does not expose it, so the redeem loop does nothing until a ticket id is recorded for the ticket (`SetTicketID`, not exposed over HTTP). Until then, anyone can redeem the ticket manually; the caller pays the Elysium gas.
- `priceHype` is always null. It is display-only and needs a price source that is not wired.
- `priceImpactBps` compares the quote rate with a probe of `amountIn / 1000` (at least 1e12 wei). It is best effort and 0 when the probe fails.
- The buy gas estimate runs from the keeper address when one is configured, with the value attached. That needs a balance, so it often fails and the 1.5M floor applies. This is conservative and documented in the code.

## Known limits and assumptions

- The explorer shapes come from `docs/RESEARCH.md` section 4 and are unverified against the live API. The client accepts both `outbox_address_hash` and `outbox_address`, and tolerates numeric or string `callvalue`.
- Rescue scans at most 20 from-rollup pages per pass and stops at `RESCUE_LOOKBACK`. Lookups are capped at 200 candidates and cached for 60 seconds.
- If an exit's claim was already executed by someone else, the claimer scans up to 200,000 HyperEVM blocks for the `OutBoxTransactionExecuted` log and reuses that transaction's calldata. If that fails, the exit stays in `confirmed` for manual recovery.
- Simulation fails for "not claimable yet" as well as for real errors. Both are retried on the next pass and never marked as done.
- A reverted send returns an error and leaves the item to be retried. A claim that is pending for a long time may be resent. The contract reverts the second copy with `AlreadySwept`, so the cost is gas, not funds.
- State is one JSON file. It is fine for testnet volumes. Each change rewrites the file atomically.
- The API has no authentication. Put it behind a reverse proxy if the keeper is reachable from the internet, because `POST /api/rescue/claim` sends transactions.

## Tests

```sh
make test
```

Unit tests cover:

- ABI and calldata checks (selector rejection, `to` extraction, salt parity, BalanceDelta split, event decoding).
- Explorer parsing, pagination, error mapping and retries, against `httptest` servers.
- Sender behavior (simulation gate, dry run, signature recovery, gas accounting, reverted receipts, legacy fallback).
- Store atomicity and round trip; config defaults, warnings and invalid input.
- Indexer filtering (launchpad hook, known quote currency, look-alike pools), idempotency, exits and token registration.
- Quote numbers, price impact, retryable parameters and error mapping.
- Claimer state machine with fakes (happy path, unconfirmed, already swept, refused calldata, simulation failure, dry run, recovery).
- Rescue selection filter, owner resolution, refusals, idempotency, per-run cap, lookup caching and claim-for-me.
- API JSON key sets for every type in INTERFACES.md section 3, validation and status codes, CORS, JSON 404s.

No test touches the network.
