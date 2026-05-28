package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	defaultRPC     = "https://ethereum-rpc.publicnode.com"
	defaultTarget  = "0x23A5e45f9556Dc7ffB507DB8a3CFb2589bC8aDAD"
	defaultRsETH   = "0xA1290d69c65A6Fe4DF752f95823fae25cB99e5A7" // Kelp DAO rsETH
	rsETHDecimals  = 18
	defaultPollMS  = 4000
	rpcCallTimeout = 15 * time.Second
)

func main() {
	rpcURL := flag.String("rpc", envOr("ETH_RPC_URL", defaultRPC), "Ethereum RPC endpoint")
	targetStr := flag.String("target", envOr("TARGET_ADDRESS", defaultTarget), "address to monitor")
	tokenStr := flag.String("token", envOr("RSETH_ADDRESS", defaultRsETH), "rsETH token contract address")
	pollMS := flag.Int("poll-ms", defaultPollMS, "poll interval in milliseconds")
	flag.Parse()

	if !common.IsHexAddress(*targetStr) {
		log.Fatalf("invalid --target address: %s", *targetStr)
	}
	if !common.IsHexAddress(*tokenStr) {
		log.Fatalf("invalid --token address: %s", *tokenStr)
	}
	target := common.HexToAddress(*targetStr)
	token := common.HexToAddress(*tokenStr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cli, err := ethclient.DialContext(ctx, *rpcURL)
	if err != nil {
		log.Fatalf("dial rpc: %v", err)
	}
	defer cli.Close()

	selector := crypto.Keccak256([]byte("balanceOf(address)"))[:4]
	callData := append(selector, common.LeftPadBytes(target.Bytes(), 32)...)

	balanceAt := func(blockNum *big.Int) (*big.Int, error) {
		cctx, ccancel := context.WithTimeout(ctx, rpcCallTimeout)
		defer ccancel()
		out, err := cli.CallContract(cctx, ethereum.CallMsg{To: &token, Data: callData}, blockNum)
		if err != nil {
			return nil, err
		}
		return new(big.Int).SetBytes(out), nil
	}

	startCtx, startCancel := context.WithTimeout(ctx, rpcCallTimeout)
	startBlock, err := cli.BlockNumber(startCtx)
	startCancel()
	if err != nil {
		log.Fatalf("get latest block: %v", err)
	}
	initial, err := balanceAt(new(big.Int).SetUint64(startBlock))
	if err != nil {
		log.Fatalf("read initial balance: %v", err)
	}
	log.Printf("Monitoring target=%s token=%s", target.Hex(), token.Hex())
	log.Printf("Initial block=%d balance=%s rsETH (%s wei)", startBlock, formatRsETH(initial), initial.String())

	lastBlock := startBlock
	prev := initial
	ticker := time.NewTicker(time.Duration(*pollMS) * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		nctx, ncancel := context.WithTimeout(ctx, rpcCallTimeout)
		cur, err := cli.BlockNumber(nctx)
		ncancel()
		if err != nil {
			log.Printf("warn: get block number: %v", err)
			continue
		}
		if cur <= lastBlock {
			continue
		}
		for b := lastBlock + 1; b <= cur; b++ {
			bal, err := balanceAt(new(big.Int).SetUint64(b))
			if err != nil {
				log.Printf("warn: balanceOf at block %d: %v", b, err)
				break
			}
			if bal.Cmp(prev) != 0 {
				delta := new(big.Int).Sub(bal, prev)
				log.Printf("block %d — balance CHANGED %s → %s rsETH (delta=%s wei)",
					b, formatRsETH(prev), formatRsETH(bal), delta.String())
				return
			}
			log.Printf("block %d — balance unchanged (%s rsETH)", b, formatRsETH(bal))
			lastBlock = b
		}
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func formatRsETH(wei *big.Int) string {
	s := wei.String()
	if len(s) <= rsETHDecimals {
		s = strings.Repeat("0", rsETHDecimals-len(s)+1) + s
	}
	point := len(s) - rsETHDecimals
	whole := s[:point]
	frac := strings.TrimRight(s[point:], "0")
	if frac == "" {
		return whole
	}
	return fmt.Sprintf("%s.%s", whole, frac)
}
