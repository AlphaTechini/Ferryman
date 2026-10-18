package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDefaults(t *testing.T) {
	c, err := Load(env(map[string]string{"DEPLOYMENTS": filepath.Join(t.TempDir(), "missing.json")}))
	if err != nil {
		t.Fatal(err)
	}
	if c.ElysiumRPC != "https://testnet-rpc.elysium.kinetiq.xyz" || c.HyperEVMRPC != "https://rpc.hyperliquid-testnet.xyz/evm" {
		t.Fatalf("rpc defaults wrong: %s %s", c.ElysiumRPC, c.HyperEVMRPC)
	}
	if c.ExplorerAPI != "https://testnet-api.elysium.kinetiq.xyz/api/v2" || c.ListenAddr != ":8787" {
		t.Fatalf("explorer/listen defaults wrong")
	}
	if c.Launchpad.Hex() != "0x6d01f6644bbf1b5074472D3e85aa1465B4d2bF3d" {
		t.Fatalf("launchpad default wrong: %s", c.Launchpad.Hex())
	}
	if c.DefaultClaimTip.String() != "500000000000000" || c.LogRange != 5000 || c.PollInterval != 20*time.Second {
		t.Fatalf("numeric defaults wrong")
	}
	if c.RescueMinAge != 24*time.Hour || c.RescueLookback != 30*24*time.Hour || c.RescueMaxPerRun != 10 || !c.RescueEnabled {
		t.Fatal("rescue defaults wrong")
	}
	if len(c.QuoteCurrencies) != 2 {
		t.Fatalf("canonical and mock USDC must be quote currencies, got %d", len(c.QuoteCurrencies))
	}
	if c.CLPoolManager.Hex() != "0x0000000000000000000000000000000000000000" {
		t.Fatal("CL_POOL_MANAGER must have no default")
	}
	joined := strings.Join(c.Warnings, "|")
	for _, want := range []string{"KEEPER_PRIVATE_KEY", "CL_POOL_MANAGER", "deployments file", "elysium.ferryIn is zero"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing warning containing %q in %q", want, joined)
		}
	}
}

func TestInvalidValuesAreErrors(t *testing.T) {
	cases := []map[string]string{
		{"LAUNCHPAD": "0x1234"},
		{"CL_POOL_MANAGER": "not-an-address"},
		{"DRY_RUN": "maybe"},
		{"POLL_INTERVAL": "soon"},
		{"OUR_LAUNCHERS": "0xabc"},
		{"DEFAULT_CLAIM_TIP_WEI": "-1"},
		{"LOG_RANGE": "0"},
	}
	for _, m := range cases {
		m["DEPLOYMENTS"] = filepath.Join(t.TempDir(), "x.json")
		if _, err := Load(env(m)); err == nil {
			t.Errorf("expected error for %v", m)
		}
	}
}

func TestDeploymentsFileZeroAndRealAddresses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "testnet.json")
	body := `{ "elysium": { "chainId": 99801, "ferryIn": "0x0", "exitDesk": "0x00000000000000000000000000000000000000Ee" },
	  "hyperevm": { "chainId": 998, "exitFactory": "0x00000000000000000000000000000000000000Ff",
	                "inbox": "0x11178Df40CBf87a4343f4B3F8B09B366ceFA8Ec0", "outbox": "0x87391D602aaffB3Fe8051EcCDb75d6EEe8C31060" } }`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(env(map[string]string{"DEPLOYMENTS": path, "KEEPER_PRIVATE_KEY": "abc"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Contracts.FerryIn.Hex() != "0x0000000000000000000000000000000000000000" {
		t.Fatal("0x0 must be the zero address")
	}
	if c.Contracts.ExitDesk != common.HexToAddress("0x00000000000000000000000000000000000000Ee") {
		t.Fatalf("exitDesk = %s", c.Contracts.ExitDesk.Hex())
	}
	if c.Contracts.Outbox.Hex() != "0x87391D602aaffB3Fe8051EcCDb75d6EEe8C31060" {
		t.Fatal("outbox not parsed")
	}
	if !c.RescueActive() {
		t.Fatal("rescue must be active with a key and an outbox")
	}
	for _, w := range c.Warnings {
		if strings.Contains(w, "outbox is zero") {
			t.Fatal("outbox is set, must not warn")
		}
	}
}

func TestMalformedDeploymentsIsWarningNotCrash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte(`{"elysium": {"ferryIn": "0xzz"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(env(map[string]string{"DEPLOYMENTS": path}))
	if err != nil {
		t.Fatalf("bad deployments must not stop start-up: %v", err)
	}
	if c.Contracts.FerryIn.Hex() != "0x0000000000000000000000000000000000000000" {
		t.Fatal("malformed addresses must fall back to zero")
	}
}
